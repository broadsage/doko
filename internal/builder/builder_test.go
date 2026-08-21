package builder

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/sourceresolver"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/util/apicaps"
	caps_pb "github.com/moby/buildkit/util/apicaps/pb"
	"github.com/opencontainers/go-digest"
)

type mockReference struct {
	client.Reference
	files map[string][]byte
}

func (m *mockReference) ReadFile(ctx context.Context, req client.ReadRequest) ([]byte, error) {
	fmt.Printf("Mock ReadFile requested: %q\n", req.Filename)
	content, ok := m.files[req.Filename]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", req.Filename)
	}
	return content, nil
}

type mockGatewayClient struct {
	client.Client
	opts      client.BuildOpts
	yamlData  []byte
	lockData  []byte
	solveErr  error
	indexData []byte
}

func (m *mockGatewayClient) BuildOpts() client.BuildOpts {
	return m.opts
}

func (m *mockGatewayClient) Inputs(ctx context.Context) (map[string]llb.State, error) {
	return map[string]llb.State{
		"context": llb.Scratch(),
	}, nil
}

func (m *mockGatewayClient) Solve(ctx context.Context, req client.SolveRequest) (*client.Result, error) {
	if m.solveErr != nil {
		return nil, m.solveErr
	}

	res := client.NewResult()
	ref := &mockReference{
		files: map[string][]byte{
			"doko.yaml":       m.yamlData,
			"doko.lock":       m.lockData,
			"APKINDEX.tar.gz": m.indexData,
			"custom-cert.crt": []byte("custom-cert-payload"),
		},
	}
	res.SetRef(ref)
	return res, nil
}

func (m *mockGatewayClient) ResolveImageConfig(ctx context.Context, ref string, opt sourceresolver.Opt) (string, digest.Digest, []byte, error) {
	// Return a minimal base image config so LLB generation doesn't fail
	cfg := map[string]any{
		"config": map[string]any{
			"Env": []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		},
	}
	data, _ := json.Marshal(cfg)
	return "", "", data, nil
}

func getGzippedIndex() []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(`P:curl
V:8.0.0
A:x86_64
L:MIT
T:URL transfer utility
S:12345
C:Q1abc

`))
	_ = gw.Close()
	return buf.Bytes()
}

func setupAPICaps() client.BuildOpts {
	var cl apicaps.CapList
	cl.Init(apicaps.Cap{
		ID:      apicaps.CapID("file.base"),
		Enabled: true,
	})
	serverCaps := []*caps_pb.APICap{
		{
			ID:      "file.base",
			Enabled: true,
		},
	}
	return client.BuildOpts{
		Opts: map[string]string{
			"filename": "doko.yaml",
		},
		LLBCaps: cl.CapSet(serverCaps),
		Caps:    cl.CapSet(serverCaps),
	}
}

func TestBuild_Success(t *testing.T) {
	ctx := context.Background()
	opts := setupAPICaps()

	yamlContent := []byte(`
name: test-app
image: ghcr.io/broadsage/doko/test-app
variant: runtime
platforms:
  - linux/amd64
annotations:
  org.opencontainers.image.title: "test-app"
  org.opencontainers.image.description: "test-app description"
accounts:
  run-as: nonroot
  users:
    - name: nonroot
      uid: 65532
      gid: 65532
contents:
  packages:
    - curl
`)

	lockContent := []byte(`
provider: apk
arch: amd64
packages:
  - name: curl
    version: 8.0.0
`)

	mc := &mockGatewayClient{
		opts:      opts,
		yamlData:  yamlContent,
		lockData:  lockContent,
		indexData: getGzippedIndex(),
	}

	result, err := Build(ctx, mc)
	if err != nil {
		t.Fatalf("expected build to succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
}

func TestBuild_LintFailure(t *testing.T) {
	ctx := context.Background()
	opts := setupAPICaps()

	// Missing security configurations (violates OPA rule for run-as root default)
	yamlContent := []byte(`
name: insecure-app
contents:
  packages:
    - curl
`)

	mc := &mockGatewayClient{
		opts:      opts,
		yamlData:  yamlContent,
		indexData: getGzippedIndex(),
	}

	_, err := Build(ctx, mc)
	if err == nil {
		t.Fatal("expected build to fail security lint check, but it succeeded")
	}
}

func TestBuild_InvalidYAML(t *testing.T) {
	ctx := context.Background()
	opts := setupAPICaps()

	yamlContent := []byte(`
invalid yaml content { {
`)

	mc := &mockGatewayClient{
		opts:      opts,
		yamlData:  yamlContent,
		indexData: getGzippedIndex(),
	}

	_, err := Build(ctx, mc)
	if err == nil {
		t.Fatal("expected build to fail parsing invalid YAML, but it succeeded")
	}
}

func TestBuild_MultiPlatform(t *testing.T) {
	ctx := context.Background()
	opts := setupAPICaps()

	yamlContent := []byte(`
name: test-multi
image: test-multi
variant: runtime
platforms:
  - linux/amd64
  - linux/arm64
annotations:
  org.opencontainers.image.title: "test-multi"
  org.opencontainers.image.description: "description"
accounts:
  run-as: nonroot
contents:
  packages:
    - curl
`)

	mc := &mockGatewayClient{
		opts:      opts,
		yamlData:  yamlContent,
		indexData: getGzippedIndex(),
	}

	result, err := Build(ctx, mc)
	if err != nil {
		t.Fatalf("expected multi-platform build to succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
}

func TestBuild_HTTPCertAndArguments(t *testing.T) {
	ctx := context.Background()
	opts := setupAPICaps()

	// Start a mock HTTP server to download the cert from
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("http-cert-data"))
	}))
	defer ts.Close()

	// Add build args and http cert paths
	opts.Opts["build-arg:SOURCE_DATE_EPOCH"] = "1719942000"
	opts.Opts["build-arg:VERSION"] = "1.2.3"

	yamlContent := fmt.Appendf(nil, `
name: cert-app
image: cert-app
variant: runtime
annotations:
  org.opencontainers.image.title: "cert-app"
  org.opencontainers.image.description: "desc"
accounts:
  run-as: nonroot
contents:
  ca-certificates:
    - "%s"
    - "custom-cert.crt"
  packages:
    - curl
`, ts.URL)

	mc := &mockGatewayClient{
		opts:      opts,
		yamlData:  yamlContent,
		indexData: getGzippedIndex(),
	}

	result, err := Build(ctx, mc)
	if err != nil {
		t.Fatalf("expected build with cert and args to succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
}
