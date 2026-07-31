package builder

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"

	"github.com/broadsage/doko/internal/config"
	"github.com/broadsage/doko/internal/pipeline"
)

//go:embed pipelines/*.yaml pipelines/*/*.yaml
var pipelinesFS embed.FS

func init() {
	pipeline.RegisterTemplateResolver(func(name string) ([]byte, error) {
		return pipelinesFS.ReadFile("pipelines/" + name + ".yaml")
	})
}

// ResolvedStep is a single resolved pipeline step ready to be emitted as an LLB exec op.
type ResolvedStep struct {
	Name    string
	Script  string
	SSH     bool
	Secrets []config.PipelineSecret
	Network string
}

// collectPipelinePackages returns a deduplicated list of packages required by pipeline steps (from each pipeline's needs.packages).
func collectPipelinePackages(s *config.BuildSpec) ([]string, error) {
	seen := make(map[string]struct{})
	for _, step := range s.Pipeline {
		if step.Uses == "" {
			continue
		}
		def, err := pipeline.GetPipeline(step.Uses)
		if err != nil {
			return nil, fmt.Errorf("get pipeline %q: %w", step.Uses, err)
		}
		for _, pkg := range def.Needs.Packages {
			seen[pkg] = struct{}{}
		}
	}
	list := make([]string, 0, len(seen))
	for pkg := range seen {
		list = append(list, pkg)
	}
	sort.Strings(list)
	return list, nil
}

// buildInstallCommand returns a shell script that configures apk repos (if any) and installs packages from the spec plus all packages needed by pipelines (deduplicated).
func buildInstallCommand(s *config.BuildSpec) (string, error) {
	pipelinePkgs, err := collectPipelinePackages(s)
	if err != nil {
		return "", err
	}
	seen := make(map[string]struct{})
	var all []string
	for _, p := range s.Contents.Packages {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			all = append(all, p)
		}
	}
	for _, p := range pipelinePkgs {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			all = append(all, p)
		}
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, repo := range s.Contents.Repositories {
		fmt.Fprintf(&b, "echo %q >> /etc/apk/repositories\n", repo)
	}
	if len(all) > 0 {
		b.WriteString("apk add --no-cache ")
		b.WriteString(strings.Join(all, " "))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// validatePipelineStep checks that step.With conforms to the pipeline's input schema.
func validatePipelineStep(def *pipeline.PipelineDef, step *config.PipelineStep, stepIndex int) error {
	for key := range step.With {
		if _, ok := def.Inputs[key]; !ok {
			return fmt.Errorf("pipeline step %d (%s): unknown input %q (allowed: %s)",
				stepIndex+1, step.Uses, key, sortedInputNames(def))
		}
	}
	for name, input := range def.Inputs {
		if !input.Required {
			continue
		}
		raw, ok := step.With[name]
		if !ok {
			return fmt.Errorf("pipeline step %d (%s): required input %q is missing", stepIndex+1, step.Uses, name)
		}
		var s string
		switch v := raw.(type) {
		case string:
			s = v
		case bool:
			s = strconv.FormatBool(v)
		default:
			s = fmt.Sprint(v)
		}
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("pipeline step %d (%s): required input %q must not be empty", stepIndex+1, step.Uses, name)
		}
	}
	return nil
}

func sortedInputNames(def *pipeline.PipelineDef) string {
	names := make([]string, 0, len(def.Inputs))
	for k := range def.Inputs {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// resolveInputs returns the full substitution map (package/targets/context + inputs) with recursive substitution applied.
func resolveInputs(def *pipeline.PipelineDef, with map[string]any, s *config.BuildSpec) (map[string]string, error) {
	sm, err := pipeline.NewSubstitutionMap(s)
	if err != nil {
		return nil, fmt.Errorf("create substitution map: %w", err)
	}
	withMap := make(map[string]string)
	for k, v := range def.Inputs {
		withMap[k] = pipeline.Substitute(v.Default, sm.Substitutions)
	}
	for k, v := range with {
		var val string
		switch x := v.(type) {
		case string:
			val = x
		case int:
			val = strconv.Itoa(x)
		case bool:
			val = strconv.FormatBool(x)
		default:
			val = fmt.Sprint(x)
		}
		withMap[k] = pipeline.Substitute(val, sm.Substitutions)
	}
	resolved, err := sm.MutateWith(withMap)
	if err != nil {
		return nil, fmt.Errorf("apply input substitutions: %w", err)
	}
	return resolved, nil
}

// reInputPlaceholder matches any ${{inputs.xxx}} left after known substitution (avoids bad shell substitution).
var reInputPlaceholder = regexp.MustCompile(`\$\{\{inputs\.[^}]+\}\}`)

// substituteScript replaces all Melange-style variables in script using the full substitution map.
func substituteScript(script string, inputs map[string]string) string {
	script = pipeline.Substitute(script, inputs)
	// Replace any remaining ${{inputs.xxx}} with empty string so shell never sees ${{ (bad substitution)
	script = reInputPlaceholder.ReplaceAllString(script, "")
	return script
}

// ResolvePipelineSteps turns spec.Pipeline into a slice of ResolvedStep structures.
func ResolvePipelineSteps(s *config.BuildSpec) ([]ResolvedStep, error) {
	if len(s.Pipeline) == 0 {
		return nil, errors.New("pipeline is required and must not be empty")
	}
	var steps []ResolvedStep
	for i, step := range s.Pipeline {
		hasRun := strings.TrimSpace(step.Runs) != ""
		hasUses := step.Uses != ""
		if hasRun && hasUses {
			return nil, fmt.Errorf("pipeline step %d: cannot set both 'uses' and 'run'", i+1)
		}
		if !hasRun && !hasUses {
			return nil, fmt.Errorf("pipeline step %d: must set either 'uses' or 'run'", i+1)
		}
		if hasRun {
			name := step.Name
			if name == "" {
				name = fmt.Sprintf("pipeline step %d", i+1)
			}
			sm, err := pipeline.NewSubstitutionMap(s)
			if err != nil {
				return nil, fmt.Errorf("create substitution map for step %d: %w", i+1, err)
			}
			resolved := substituteScript(step.Runs, sm.Substitutions)
			steps = append(steps, ResolvedStep{
				Name:    name,
				Script:  resolved,
				SSH:     step.SSH,
				Secrets: step.Secrets,
				Network: step.Network,
			})
			continue
		}
		def, err := pipeline.GetPipeline(step.Uses)
		if err != nil {
			return nil, fmt.Errorf("pipeline step %d: %w", i+1, err)
		}
		if err := validatePipelineStep(def, &step, i); err != nil {
			return nil, err
		}
		inputs, err := resolveInputs(def, step.With, s)
		if err != nil {
			return nil, err
		}
		resolved := substituteScript(def.Runs, inputs)
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("pipeline step %d (%s)", i+1, step.Uses)
		}
		steps = append(steps, ResolvedStep{
			Name:    name,
			Script:  resolved,
			SSH:     step.SSH,
			Secrets: step.Secrets,
			Network: step.Network,
		})
	}
	return steps, nil
}

// BuildAPK produces an llb.State that contains built .apk package(s).
func BuildAPK(ctx context.Context, s *config.BuildSpec, sourceState llb.State, workerBaseImage string, resolver llb.ImageMetaResolver, opts ...llb.ConstraintsOpt) (llb.State, error) {
	if s.Name == "" {
		return llb.Scratch(), errors.New("spec name is required")
	}
	if s.Version == "" {
		return llb.Scratch(), errors.New("spec version is required")
	}
	if s.Description == "" {
		return llb.Scratch(), errors.New("spec description is required")
	}
	if s.URL == "" {
		return llb.Scratch(), errors.New("spec url is required")
	}
	if s.License == "" {
		return llb.Scratch(), errors.New("spec license is required")
	}

	// Worker: Alpine + environment packages from spec (repositories + packages) + pipeline needs (deduplicated)
	workerImage := llb.Image(workerBaseImage, llb.WithCustomName("apk worker base"))
	if resolver != nil {
		workerImage = llb.Image(workerBaseImage, llb.WithMetaResolver(resolver), llb.WithCustomName("apk worker base"))
	}
	installCmd, err := buildInstallCommand(s)
	if err != nil {
		return llb.Scratch(), err
	}
	workerRunOpts := []llb.RunOption{
		llb.Args([]string{"sh", "-c", installCmd}),
		llb.WithCustomName("install build deps"),
	}
	for _, o := range opts {
		workerRunOpts = append(workerRunOpts, o)
	}
	worker := workerImage.Run(workerRunOpts...).Root()

	// Mount source at /src
	workerWithSrc := worker.File(
		llb.Copy(sourceState, "/", "/src"),
		opts...,
	)

	steps, err := ResolvePipelineSteps(s)
	if err != nil {
		return llb.Scratch(), err
	}

	// Initialize /pkg directory once in the worker state
	state := workerWithSrc.File(
		llb.Mkdir("/pkg", 0o755, llb.WithParents(true)),
		opts...,
	)

	// Run pipeline steps sequentially as separate BuildKit run vertices
	for _, step := range steps {
		runOpts := []llb.RunOption{
			llb.Args([]string{"sh", "-c", "set -e\n" + step.Script}),
			llb.Dir("/"),
			llb.WithCustomName(step.Name),
		}
		if step.SSH {
			runOpts = append(runOpts, llb.AddSSHSocket(llb.SSHID("default"), llb.SSHSocketTarget("/run/ssh-agent.sock")))
			runOpts = append(runOpts, llb.AddEnv("SSH_AUTH_SOCK", "/run/ssh-agent.sock"))
		}
		for _, sec := range step.Secrets {
			runOpts = append(runOpts, llb.AddSecret(sec.Target, llb.SecretID(sec.ID)))
		}
		stepState := state
		if step.Network != "" {
			switch strings.ToLower(step.Network) {
			case "none":
				stepState = stepState.Network(pb.NetMode_NONE)
			case "host":
				stepState = stepState.Network(pb.NetMode_HOST)
			}
		}
		for _, o := range opts {
			runOpts = append(runOpts, o)
		}
		state = stepState.Run(runOpts...).Root()
	}

	return state, nil
}
