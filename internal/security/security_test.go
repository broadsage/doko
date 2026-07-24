package security

import (
	"encoding/json"
	"testing"
)

func TestGenerateSeccompProfile(t *testing.T) {
	profile, err := GenerateSeccompProfile([]string{"nginx", "curl"}, []int{8080})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("expected default action SCMP_ACT_ERRNO, got %s", profile.DefaultAction)
	}
	if len(profile.Architectures) != 2 {
		t.Errorf("expected 2 architectures, got %d", len(profile.Architectures))
	}
	if len(profile.Syscalls) == 0 {
		t.Error("expected at least one syscall rule")
	}
	if profile.Syscalls[0].Action != "SCMP_ACT_ALLOW" {
		t.Errorf("expected action SCMP_ACT_ALLOW, got %s", profile.Syscalls[0].Action)
	}
}

func TestGenerateLandlockPolicy(t *testing.T) {
	policy, err := GenerateLandlockPolicy([]string{"/var/www/html"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, p := range policy.ReadablePaths {
		if p == "/var/www/html" {
			found = true
		}
	}
	if !found {
		t.Error("expected /var/www/html in readable paths")
	}

	// When readOnly=true, the custom path should NOT be in writable paths
	for _, p := range policy.WritablePaths {
		if p == "/var/www/html" {
			t.Error("/var/www/html should not be writable when readOnly=true")
		}
	}
}

func TestMarshalSeccomp(t *testing.T) {
	profile, _ := GenerateSeccompProfile([]string{"test"}, nil)
	data, err := MarshalSeccomp(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed SeccompProfile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated JSON is not valid: %v", err)
	}
	if parsed.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("expected default action SCMP_ACT_ERRNO after round-trip, got %s", parsed.DefaultAction)
	}
}

func TestMarshalLandlock(t *testing.T) {
	policy, _ := GenerateLandlockPolicy([]string{"/app"}, false)
	data, err := MarshalLandlock(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed LandlockPolicy
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated JSON is not valid: %v", err)
	}
}
