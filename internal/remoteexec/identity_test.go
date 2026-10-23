package remoteexec

import (
	"encoding/hex"
	"testing"

	"shell_ops/internal/sshmgr"
)

func TestHostIdentityBindsConnectionWithoutCredentials(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(*sshmgr.HostInput)
		changed bool
	}{
		{"address", func(h *sshmgr.HostInput) { h.Address = "10.0.0.2" }, true},
		{"port", func(h *sshmgr.HostInput) { h.Port = 2222 }, true},
		{"user", func(h *sshmgr.HostInput) { h.User = "other" }, true},
		{"name", func(h *sshmgr.HostInput) { h.Name = "renamed" }, false},
		{"password", func(h *sshmgr.HostInput) { h.Password = "rotated-secret" }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := sshmgr.NewManager(t.TempDir())
			input := sshmgr.HostInput{Name: "target", Address: "10.0.0.1", Port: 22, User: "ops", Auth: sshmgr.AuthPassword, Password: "private-secret"}
			host, err := manager.Create(input)
			if err != nil {
				t.Fatal(err)
			}
			executor := New(manager)
			before, err := executor.HostIdentity(host.ID)
			if err != nil {
				t.Fatal(err)
			}
			if value, err := hex.DecodeString(before); err != nil || len(value) != 32 {
				t.Fatalf("identity must only contain SHA256 digest: %q", before)
			}
			tt.change(&input)
			if _, err := manager.Update(host.ID, input); err != nil {
				t.Fatal(err)
			}
			after, err := executor.HostIdentity(host.ID)
			if err != nil {
				t.Fatal(err)
			}
			if (before != after) != tt.changed {
				t.Fatalf("identity change = %v, want %v", before != after, tt.changed)
			}
		})
	}
}
