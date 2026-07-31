package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/broadsage/doko/internal/config"
	layerkitllb "github.com/broadsage/doko/internal/llb"
	"github.com/broadsage/doko/internal/metadata"
	"github.com/broadsage/doko/internal/providers"
	_ "github.com/broadsage/doko/internal/providers/apk"

	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/moby/buildkit/frontend/gateway/client"
)

const defaultFilename = "doko.yaml"

// Build is the main orchestrator called by BuildKit frontend.
func Build(ctx context.Context, c client.Client) (*client.Result, error) {
	// 1. Initialize the dockerui client to read the build definition file.
	bc, err := dockerui.NewClient(c)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build client: %w", err)
	}

	// 2. Read the doko.yaml from the build context.
	src, err := bc.ReadEntrypoint(ctx, defaultFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", defaultFilename, err)
	}

	// 3. Parse and validate the YAML configuration.
	spec, err := config.Parse(bytes.NewReader(src.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse layerkit config: %w", err)
	}

	// 3.5 Parse build arguments from client options and merge into spec.Vars
	buildOpts := c.BuildOpts()
	for k, v := range buildOpts.Opts {
		if argKey, cut := strings.CutPrefix(k, "build-arg:"); cut {
			if spec.Vars == nil {
				spec.Vars = make(map[string]string)
			}
			spec.Vars[argKey] = v
		}
	}

	var lockData []byte
	if srcLock, err := bc.ReadEntrypoint(ctx, "doko.lock"); err == nil {
		lockData = srcLock.Data
	}

	var lockedPkgs map[string]string
	if len(lockData) > 0 {
		var lf providers.Lockfile
		if err := yaml.Unmarshal(lockData, &lf); err == nil {
			lockedPkgs = make(map[string]string)
			for _, p := range lf.Packages {
				lockedPkgs[p.Name] = p.Version
			}
		}
	}

	var caCerts [][]byte
	timeoutDuration := time.Duration(spec.TimeoutSeconds) * time.Second
	caClient := &http.Client{Timeout: timeoutDuration}
	for _, certPath := range spec.Contents.CACertificates {
		if strings.HasPrefix(certPath, "http://") || strings.HasPrefix(certPath, "https://") {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, certPath, nil)
			if err == nil {
				resp, err := caClient.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					defer func() { _ = resp.Body.Close() }()
					if data, err := io.ReadAll(resp.Body); err == nil {
						caCerts = append(caCerts, data)
					}
				}
			}
		} else {
			if srcCert, err := bc.ReadEntrypoint(ctx, certPath); err == nil {
				caCerts = append(caCerts, srcCert.Data)
			}
		}
	}

	platforms := spec.Platforms
	if len(platforms) == 0 {
		// Fallback to single platform
		arch := spec.Arch
		if arch == "" {
			switch {
			case len(bc.TargetPlatforms) > 0 && bc.TargetPlatforms[0].Architecture != "":
				arch = bc.TargetPlatforms[0].Architecture
			case len(bc.BuildPlatforms) > 0 && bc.BuildPlatforms[0].Architecture != "":
				arch = bc.BuildPlatforms[0].Architecture
			default:
				arch = "amd64"
			}
		}
		platforms = []string{"linux/" + arch}
	}

	if len(platforms) == 1 {
		parts := strings.Split(platforms[0], "/")
		arch := parts[len(parts)-1]
		spec.Arch = arch
		return buildPlatformResult(ctx, c, spec, lockedPkgs, caCerts)
	}

	// Multi-platform manifest generation
	finalResult := client.NewResult()

	type Platform struct {
		ID       string `json:"id"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant,omitempty"`
		} `json:"platform"`
	}
	type Platforms struct {
		Platforms []Platform `json:"platforms"`
	}

	var platformList []Platform
	for _, p := range platforms {
		parts := strings.Split(p, "/")
		osVal := parts[0]
		archVal := parts[1]
		variant := ""
		if len(parts) > 2 {
			variant = parts[2]
		}

		plat := Platform{
			ID: p,
		}
		plat.Platform.OS = osVal
		plat.Platform.Architecture = archVal
		plat.Platform.Variant = variant
		platformList = append(platformList, plat)

		// Copy spec for this platform
		platformSpec := *spec
		platformSpec.Arch = archVal

		subRes, err := buildPlatformResult(ctx, c, &platformSpec, lockedPkgs, caCerts)
		if err != nil {
			return nil, fmt.Errorf("failed to build platform %s: %w", p, err)
		}

		finalResult.AddRef(p, subRes.Ref)
		for k, v := range subRes.Metadata {
			finalResult.AddMeta(k+"/"+p, v)
		}
	}

	platformData, err := json.Marshal(Platforms{Platforms: platformList})
	if err == nil {
		finalResult.AddMeta("refs.platforms", platformData)
	}

	return finalResult, nil
}

// buildPlatformResult compiles and solves the image filesystem and metadata for a given target architecture.
func buildPlatformResult(ctx context.Context, c client.Client, spec *config.Spec, lockedPkgs map[string]string, caCerts [][]byte) (*client.Result, error) {
	// 1. Construct a resolver for the specified package provider.
	timeoutDuration := time.Duration(spec.TimeoutSeconds) * time.Second

	res, err := providers.NewResolver(spec.Provider, providers.Options{
		Arch:           spec.Arch,
		Repositories:   spec.Contents.Repositories,
		LockedPackages: lockedPkgs,
		CACerts:        caCerts,
		Timeout:        timeoutDuration,
		OSVersion:      spec.Base,
		Client:         c,
	})
	if err != nil {
		return nil, err
	}

	// 2. Resolve packages using real index lookups.
	// Filter out negative package entries (e.g. "!telnet") — they are removals handled
	// by the LLB generator and have no entry in any package index.
	var resolvePkgs []string
	for _, pkg := range spec.Contents.Packages {
		if !strings.HasPrefix(pkg, "!") {
			resolvePkgs = append(resolvePkgs, pkg)
		}
	}
	resolvedPkgs, err := res.Resolve(ctx, resolvePkgs)
	if err != nil {
		return nil, fmt.Errorf("[doko] package resolution via %s failed: %w", res.Name(), err)
	}
	fmt.Fprintf(os.Stderr, "[doko] resolved %d packages via %s\n", len(resolvedPkgs), res.Name())

	// 4. Generate LLB definition.
	fmt.Fprintf(os.Stderr, "[doko] step 4: generating LLB definition\n")
	gen := layerkitllb.NewGenerator(spec, c)
	def, err := gen.Generate(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[doko] step 4: LLB definition generated successfully\n")

	// 5. Solve the LLB definition through BuildKit.
	fmt.Fprintf(os.Stderr, "[doko] step 5: calling c.Solve\n")
	result, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, fmt.Errorf("solve failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[doko] step 5: c.Solve succeeded\n")

	// 6. Generate and attach metadata and OCI image configs.
	fmt.Fprintf(os.Stderr, "[doko] step 6: attaching metadata and configurations\n")
	if err := metadata.AttachAll(ctx, spec, result); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[doko] step 6 finished\n")
	fmt.Fprintf(os.Stderr, "[doko] buildPlatformResult successfully returning result\n")

	return result, nil
}
