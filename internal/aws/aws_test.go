package aws

import "testing"

func TestControlPlaneLoggingEnabled(t *testing.T) {
	if ControlPlaneLoggingEnabled(nil) {
		t.Fatal("expected false for no enabled log types")
	}
	if !ControlPlaneLoggingEnabled([]string{"api"}) {
		t.Fatal("expected true when at least one log type is enabled")
	}
}
