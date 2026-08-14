package llb

import (
	"context"
	"testing"

	"github.com/broadsage/doko/internal/config"
	_ "github.com/broadsage/doko/internal/providers/apk"
)

func TestGenerate_WithPipelineAndKeyring(t *testing.T) {
	spec := &config.Spec{
		Name:     "test-generator",
		Provider: "apk",
		Base:     "alpine-3.23",
		Contents: config.ContentsConfig{
			Packages: []string{"curl"},
			Keyring:  []string{"https://example.com/key.rsa.pub"},
			Pipeline: []config.PipelineStep{
				{
					Name: "setup-logs",
					Runs: "ln -sf /dev/stdout /var/log/nginx/access.log",
				},
			},
		},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition: %v", err)
	}
}

func TestGenerate_MultiStageBuild(t *testing.T) {
	spec := &config.Spec{
		Name:     "multi-stage-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		Builds: []config.BuildSpec{
			{
				Name:    "builder-stage",
				WorkDir: "/go/src/app",
				Contents: config.ContentsConfig{
					Packages: []string{"go"},
					Pipeline: []config.PipelineStep{
						{Name: "build-binary", Runs: "go build -o app main.go"},
					},
				},
				Outputs: []config.Output{
					{
						Source: "/app",
						Target: "/usr/local/bin/app",
					},
				},
			},
		},
		Contents: config.ContentsConfig{},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition for multi-stage build: %v", err)
	}
}

func TestGenerate_WithAccounts(t *testing.T) {
	spec := &config.Spec{
		Name:     "accounts-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		Accounts: config.AccountsConfig{
			RunAs: "nonroot",
			Users: []config.User{
				{
					Name: "nonroot",
					UID:  65532,
					GID:  65532,
				},
			},
			Groups: []config.Group{
				{
					Name:    "nonroot",
					GID:     65532,
					Members: []string{"nonroot"},
				},
			},
		},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition with accounts: %v", err)
	}
}

func TestGenerate_WithWorkDir(t *testing.T) {
	spec := &config.Spec{
		Name:     "workdir-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		WorkDir:  "/var/app/custom",
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition with work-dir: %v", err)
	}
}

func TestGenerate_WithPrivileged(t *testing.T) {
	spec := &config.Spec{
		Name:     "privileged-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		Builds: []config.BuildSpec{
			{
				Name:       "builder",
				Privileged: true,
				Contents: config.ContentsConfig{
					Pipeline: []config.PipelineStep{
						{Name: "setup", Runs: "echo 1"},
					},
				},
			},
		},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition with privileged settings: %v", err)
	}
}

func TestGenerate_NginxConfig(t *testing.T) {
	spec, err := config.ParseFile("../../examples/nginx/doko.yaml")
	if err != nil {
		t.Fatalf("failed to parse nginx config file: %v", err)
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err = g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition for nginx config: %v", err)
	}
}

func TestGenerate_SubBuildWithCACertificates(t *testing.T) {
	spec := &config.Spec{
		Name:     "subbuild-ca-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		Builds: []config.BuildSpec{
			{
				Name:     "builder-stage",
				Provider: "apk",
				Base:     "alpine-3.23",
				Contents: config.ContentsConfig{
					Packages:       []string{"curl"},
					CACertificates: []string{"https://example.com/custom-ca.crt"},
				},
			},
		},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition with subbuild CA certificates: %v", err)
	}
}

func TestGenerate_WithSecretsAndNetwork(t *testing.T) {
	spec := &config.Spec{
		Name:     "secrets-network-app",
		Provider: "apk",
		Base:     "alpine-3.23",
		Contents: config.ContentsConfig{
			Pipeline: []config.PipelineStep{
				{
					Name: "test-pipeline",
					Runs: "echo hello",
					Secrets: []config.PipelineSecret{
						{
							ID:     "mysecret",
							Target: "/run/secrets/mysecret",
						},
					},
					Network: "none",
				},
			},
		},
	}
	g := NewGenerator(spec, nil, nil)
	ctx := context.Background()
	_, err := g.Generate(ctx)
	if err != nil {
		t.Fatalf("failed to generate LLB definition with secrets and network: %v", err)
	}
}
