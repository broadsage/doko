package config

import (
	"context"
	"slices"
	"testing"
)

func TestLint_RootUserChecks(t *testing.T) {
	// 1. Explicit accounts.root = true should produce an error
	spec1 := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  true,
			RunAs: "nonroot",
		},
	}
	res1, err := Lint(context.Background(), spec1)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	if len(res1.Errors) == 0 {
		t.Error("expected error when accounts.root is true, but got none")
	}

	// 2. Explicit run-as = "root" should produce an error
	spec2 := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "root",
		},
	}
	res2, err := Lint(context.Background(), spec2)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	if len(res2.Errors) == 0 {
		t.Error("expected error when accounts.run-as is 'root', but got none")
	}

	// 3. Implicit run-as (missing) should produce an error unless accounts.root is explicitly false
	spec3 := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			RunAs: "",
		},
	}
	res3, err := Lint(context.Background(), spec3)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	if len(res3.Errors) == 0 {
		t.Error("expected error when run-as is empty and root is not explicitly false")
	}

	// 4. Safe run-as = "nonroot" with root = false should not produce root errors
	spec4 := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
	}
	res4, err := Lint(context.Background(), spec4)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	for _, e := range res4.Errors {
		if e == "[root-execution] 'accounts.root' is set to true. Run container as a non-root user instead." ||
			e == "[root-execution] 'accounts.run-as' is set to 'root'. Avoid using root user in production." ||
			e == "[root-execution] 'accounts.run-as' is empty, defaults to root. Explicitly specify a non-root user." {
			t.Errorf("unexpected root execution error: %s", e)
		}
	}
}

func TestLint_ForbiddenPackages(t *testing.T) {
	spec := &Spec{
		Name:    "test-app",
		Variant: "runtime",
		Contents: ContentsConfig{
			Packages: []string{"gcc", "curl"},
		},
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	foundWarning := slices.Contains(res.Warnings, "[dev-tools-in-production] Development tool 'gcc' is present in a runtime image. Exclude compiling/development tools in production variants.")
	if !foundWarning {
		t.Error("expected warning about forbidden package 'gcc' in runtime variant, but got none")
	}
}

func TestLint_MissingAnnotations(t *testing.T) {
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}

	foundTitle := slices.Contains(res.Warnings, "[missing-annotations] Standard OCI annotation 'org.opencontainers.image.title' is missing under 'annotations'.")
	foundDesc := slices.Contains(res.Warnings, "[missing-annotations] Standard OCI annotation 'org.opencontainers.image.description' is missing under 'annotations'.")

	if !foundTitle {
		t.Error("expected warning for missing title annotation")
	}
	if !foundDesc {
		t.Error("expected warning for missing description annotation")
	}
}

func TestLint_PermissivePermissions(t *testing.T) {
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
		Contents: ContentsConfig{
			Paths: []PathConfig{
				{
					Path: "/var/log/app",
					Mode: "0777",
				},
			},
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}

	foundPermissionWarning := slices.Contains(res.Warnings, "[permissive-permissions] Path '/var/log/app' is configured with permissive permissions (mode: '0777'). Use restricted permissions (e.g. '0755' or '0750').")

	if !foundPermissionWarning {
		t.Error("expected warning for permissive write permissions (0777)")
	}
}

func TestLint_IgnoreRules(t *testing.T) {
	// A spec that violates root-execution and missing-annotations,
	// but ignores root-execution. It should only produce the missing-annotations warning.
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root: true,
		},
		IgnoreRules: []string{"root-execution"},
	}

	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}

	if len(res.Errors) > 0 {
		t.Errorf("expected 0 errors due to root-execution rule bypass, got %d: %v", len(res.Errors), res.Errors)
	}

	foundAnnotationsWarning := slices.Contains(res.Warnings, "[missing-annotations] Standard OCI annotation 'org.opencontainers.image.title' is missing under 'annotations'.")
	if !foundAnnotationsWarning {
		t.Error("expected missing-annotations warning, but got none")
	}
}

func TestLint_NoShellsInProduction(t *testing.T) {
	spec := &Spec{
		Name:    "test-app",
		Variant: "production",
		Contents: ContentsConfig{
			Packages: []string{"bash", "ca-certificates-bundle"},
		},
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	found := slices.Contains(res.Warnings, "[no-shells-in-production] Development tool/shell 'bash' is present in a production variant. Enforce distroless environments in production.")
	if !found {
		t.Error("expected warning for shell 'bash' in production variant, but got none")
	}
}

func TestLint_SecretsExposure(t *testing.T) {
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
		Environment: map[string]string{
			"DB_PASSWORD": "supersecretpassword",
		},
		Vars: map[string]string{
			"GITHUB_TOKEN": "token123",
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}

	foundEnv := slices.Contains(res.Errors, "[secrets-exposure] Potential credential exposure: Key 'DB_PASSWORD' appears to contain sensitive information in 'environment'.")
	foundVars := slices.Contains(res.Errors, "[secrets-exposure] Potential credential exposure: Key 'GITHUB_TOKEN' appears to contain sensitive information in 'vars'.")

	if !foundEnv {
		t.Error("expected secrets-exposure error for environment key 'DB_PASSWORD'")
	}
	if !foundVars {
		t.Error("expected secrets-exposure error for vars key 'GITHUB_TOKEN'")
	}
}

func TestLint_DeprecatedPackages(t *testing.T) {
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
		Contents: ContentsConfig{
			Packages: []string{"telnet", "curl"},
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	found := slices.Contains(res.Warnings, "[deprecated-packages] Obsolete package 'telnet' is deprecated and has known security alternatives. Replace with secure packages.")
	if !found {
		t.Error("expected warning about deprecated package 'telnet', but got none")
	}
}

func TestLint_PrivilegedStages(t *testing.T) {
	spec := &Spec{
		Name: "test-app",
		Accounts: AccountsConfig{
			Root:  false,
			RunAs: "nonroot",
		},
		Builds: []BuildSpec{
			{
				Name:       "compiler",
				Privileged: true,
			},
		},
	}
	res, err := Lint(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error during lint: %v", err)
	}
	found := slices.Contains(res.Warnings, "[privileged-stages] Build stage 'compiler' is configured with 'privileged: true'. Restrict elevated compiler privileges only to verified steps.")
	if !found {
		t.Error("expected warning about privileged build stage, but got none")
	}
}
