package authz

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "authz.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestModeOffAdmitsEveryone(t *testing.T) {
	g := New(Config{Mode: "off"})
	if g.Enabled() {
		t.Fatal("mode off should be disabled")
	}
	if !g.Allowed("anything") {
		t.Fatal("mode off must admit any device")
	}
}

func TestAllowlist(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, `{"version":1,"allow":["dev1","dev2"]}`)
	g := New(Config{Mode: "allowlist", DeviceFile: p})
	if !g.Allowed("dev1") {
		t.Error("dev1 should be allowed")
	}
	if g.Allowed("dev3") {
		t.Error("dev3 must be rejected")
	}
}

func TestDenylist(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, `{"version":1,"deny":["bad1"]}`)
	g := New(Config{Mode: "denylist", DeviceFile: p})
	if g.Allowed("bad1") {
		t.Error("bad1 must be rejected")
	}
	if !g.Allowed("good1") {
		t.Error("good1 should be allowed in denylist mode")
	}
}

func TestMissingFileFailsOpen(t *testing.T) {
	g := New(Config{Mode: "allowlist", DeviceFile: "/nonexistent/authz.json"})
	if !g.Allowed("dev1") {
		t.Error("missing file must fail open (admit)")
	}
}

func TestReloadOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, `{"version":1,"allow":["dev1"]}`)
	g := New(Config{Mode: "allowlist", DeviceFile: p})
	if !g.Allowed("dev1") || g.Allowed("dev2") {
		t.Fatal("initial state wrong")
	}
	// Rewrite the file with a different allowlist and bump mtime → reload.
	if err := os.WriteFile(p, []byte(`{"version":1,"allow":["dev2"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if g.Allowed("dev1") || !g.Allowed("dev2") {
		t.Error("gate did not pick up file change")
	}
}
