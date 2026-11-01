package readiness

import "testing"

func TestCheckKarpenterCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		version string
		target  string
		wantSev Severity
	}{
		{"compatible version", "1.1.0", "1.32", SeveritySafe},
		{"incompatible older version", "0.36.0", "1.31", SeverityBlocking},
		{"unknown target version", "1.0.0", "1.99", SeverityWarning},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := CheckKarpenterCompatibility(tc.version, tc.target)
			if f.Severity != tc.wantSev {
				t.Fatalf("got severity %s, want %s (finding: %+v)", f.Severity, tc.wantSev, f)
			}
		})
	}
}

func TestCheckAddonCompatibility_AlwaysRecommendsVerification(t *testing.T) {
	f := CheckAddonCompatibility("vpc-cni", "v1.19.0", "1.31")
	if f.Severity != SeverityWarning {
		t.Fatalf("expected WARNING severity, got %s", f.Severity)
	}
	if f.Recommendation == "" {
		t.Fatal("expected a non-empty recommendation")
	}
}
