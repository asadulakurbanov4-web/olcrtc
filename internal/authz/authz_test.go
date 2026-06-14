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
	// Explicit "open" for legacy/diag semantics test.
	g := New(Config{Mode: "allowlist", DeviceFile: "/nonexistent/authz.json", FailMode: "open"})
	if !g.Allowed("dev1") {
		t.Error("missing file must fail open (admit) when FailMode=open")
	}
}

func TestLKGOnMissingOrBadFileDoesNotExpandAllow(t *testing.T) {
	// AZ-07: in fail_mode=lkg, bad/missing file MUST NOT admit devices beyond the last good allowlist.
	// This is the key regression test for paid LKG decision (analysis v2.1 §5.2, §5.1).
	dir := t.TempDir()
	p := writeFile(t, dir, `{"version":42,"allow":["good1","good2"]}`)

	g := New(Config{Mode: "allowlist", DeviceFile: p, FailMode: "lkg"})
	if !g.Allowed("good1") || !g.Allowed("good2") || g.Allowed("bad3") {
		t.Fatal("initial good state wrong")
	}

	// Now make file bad (corrupt JSON) — should freeze on LKG, not open to all.
	if err := os.WriteFile(p, []byte(`{"version":43,"allow":["good1" `), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if g.Allowed("bad3") || g.Allowed("bad4") {
		t.Error("AZ-07 FAIL: corrupt file in lkg mode expanded allow beyond last-good (bad3/bad4 admitted)")
	}
	if !g.Allowed("good1") || !g.Allowed("good2") {
		t.Error("LKG freeze lost previously good devices on corrupt file")
	}

	// Missing file: same, keep LKG
	os.Remove(p)
	if g.Allowed("badX") {
		t.Error("AZ-07 FAIL: missing file in lkg expanded allow")
	}
	if !g.Allowed("good2") {
		t.Error("LKG should still admit good devices after delete")
	}
}

func TestLKGVersionRollbackKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, `{"version":10,"allow":["v10"]}`)
	g := New(Config{Mode: "allowlist", DeviceFile: p, FailMode: "lkg"})
	if !g.Allowed("v10") {
		t.Fatal("init")
	}

	// Simulate rollback to older version (or stale writer) — must reject, keep LKG.
	if err := os.WriteFile(p, []byte(`{"version":5,"allow":["v5","v10"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fut := time.Now().Add(3 * time.Second)
	_ = os.Chtimes(p, fut, fut)

	if g.Allowed("v5") {
		t.Error("version rollback in lkg must not admit the rolled-back set (v5)")
	}
	if !g.Allowed("v10") {
		t.Error("LKG should preserve previous good on rollback")
	}
}

func TestColdStartLKGClosed(t *testing.T) {
	// No prior good file ever + lkg mode → cold start closed (deny), with error path.
	g := New(Config{Mode: "allowlist", DeviceFile: "/no/such/file/ever.json", FailMode: "lkg"})
	if g.Allowed("anything") {
		t.Error("cold-start lkg without any prior good must deny (closed behavior)")
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
