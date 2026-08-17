package builder

import (
	"context"
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
	fmt.Printf("ReadFile requested: %q\n", req.Filename)
	content, ok := m.files[req.Filename]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", req.Filename)
	}
	return content, nil
}

type mockGatewayClient struct {
	client.Client
	opts client.BuildOpts
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
	res := client.NewResult()
	ref := &mockReference{
		files: map[string][]byte{
			"doko.yaml": []byte(`
name: test-app
base: alpine
contents:
  packages:
    - curl
`),
			"doko.lock": []byte(`
provider: apk
packages:
  - name: curl
    version: 8.0.0
`),
		},
	}
	res.SetRef(ref)
	return res, nil
}

func (m *mockGatewayClient) ResolveImageConfig(ctx context.Context, ref string, opt sourceresolver.Opt) (string, digest.Digest, []byte, error) {
	return "", "", nil, nil
}

func TestBuild_SuccessAndErrors(t *testing.T) {
	ctx := context.Background()
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

	mc := &mockGatewayClient{
		opts: client.BuildOpts{
			Opts: map[string]string{
				"build-arg:FOO": "BAR",
				"filename":      "doko.yaml",
			},
			LLBCaps: cl.CapSet(serverCaps),
			Caps:    cl.CapSet(serverCaps),
		},
	}

	_, err := Build(ctx, mc)
	t.Logf("Build returned error: %v", err)
	if err == nil {
		t.Error("expected build to fail eventually during package resolution, but got success")
	}
}

func TestFetchCACert(t *testing.T) {
	ctx := context.Background()
	hc := &http.Client{}

	// 1. Success case
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock-cert-data"))
	}))
	defer ts.Close()

	data, err := fetchCACert(ctx, hc, ts.URL)
	if err != nil {
		t.Fatalf("fetchCACert failed: %v", err)
	}
	if string(data) != "mock-cert-data" {
		t.Errorf("expected 'mock-cert-data', got %q", string(data))
	}

	// 2. Status error case
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tsErr.Close()

	_, err = fetchCACert(ctx, hc, tsErr.URL)
	if err == nil {
		t.Fatal("expected error from non-OK status, got nil")
	}

	// 3. Network error / invalid URL case
	_, err = fetchCACert(ctx, hc, "http://invalid.local.url/does-not-exist")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	// 4. Invalid Request case (bad URL format)
	_, err = fetchCACert(ctx, hc, "%%")
	if err == nil {
		t.Fatal("expected request creation error, got nil")
	}
}
