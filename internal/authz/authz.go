// Package authz gates client sessions by deviceID against an allowlist file
// written by the control panel. It plugs into the server's handshake AuthFunc
// without touching the shared room/key transport.
//
// Design notes:
//   - The allowlist file is re-read on demand with an mtime cache, so the panel
//     can block/unblock clients live without restarting the server.
//   - Modes: "allowlist" (admit only listed devices), "denylist" (reject listed),
//     "off"/"" (admit everyone — preserves the original open behaviour).
//   - Fail-open: a missing or unparsable file admits everyone (with a warning).
//     Availability of paying clients beats best-effort revocation.
package authz

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openlibrecommunity/olcrtc/internal/handshake"
	"github.com/openlibrecommunity/olcrtc/internal/logger"
)

// Config configures a Gate. Zero value (Mode "") means the gate admits everyone.
type Config struct {
	Mode            string        // "allowlist" | "denylist" | "off"/""
	DeviceFile      string        // path to the JSON allowlist the panel writes
	EnforceInterval time.Duration // how often to sweep live sessions (0 = no sweep)
}

// fileSchema mirrors what the panel writes (services/authz_writer.py).
type fileSchema struct {
	Version   int      `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny"`
}

// Gate evaluates deviceIDs against the allowlist file with an mtime cache.
type Gate struct {
	cfg Config

	mu       sync.Mutex
	loadedAt time.Time // mtime of the file at last successful load
	allow    map[string]struct{}
	deny     map[string]struct{}
	loaded   bool
}

// New builds a Gate. A nil-ish config (Mode off/"") yields an always-allow gate.
func New(cfg Config) *Gate {
	return &Gate{cfg: cfg}
}

// Enabled reports whether the gate actually restricts anything.
func (g *Gate) Enabled() bool {
	switch g.cfg.Mode {
	case "allowlist", "denylist":
		return g.cfg.DeviceFile != ""
	default:
		return false
	}
}

// EnforceInterval exposes the configured sweep interval.
func (g *Gate) EnforceInterval() time.Duration { return g.cfg.EnforceInterval }

// refresh reloads the file if its mtime changed. Caller holds g.mu.
func (g *Gate) refresh() {
	fi, err := os.Stat(g.cfg.DeviceFile)
	if err != nil {
		// Missing file → fail-open (clear sets, treated as "no restriction" by Allowed).
		if g.loaded {
			logger.Warnf("authz: device file %s unreadable (%v); failing open", g.cfg.DeviceFile, err)
		}
		g.allow, g.deny, g.loaded = nil, nil, false
		return
	}
	if g.loaded && fi.ModTime().Equal(g.loadedAt) {
		return // unchanged
	}
	data, err := os.ReadFile(g.cfg.DeviceFile)
	if err != nil {
		logger.Warnf("authz: read %s failed (%v); failing open", g.cfg.DeviceFile, err)
		g.allow, g.deny, g.loaded = nil, nil, false
		return
	}
	var fs fileSchema
	if err := json.Unmarshal(data, &fs); err != nil {
		logger.Warnf("authz: parse %s failed (%v); failing open", g.cfg.DeviceFile, err)
		g.allow, g.deny, g.loaded = nil, nil, false
		return
	}
	allow := make(map[string]struct{}, len(fs.Allow))
	for _, d := range fs.Allow {
		allow[d] = struct{}{}
	}
	deny := make(map[string]struct{}, len(fs.Deny))
	for _, d := range fs.Deny {
		deny[d] = struct{}{}
	}
	g.allow, g.deny, g.loadedAt, g.loaded = allow, deny, fi.ModTime(), true
	logger.Infof("authz: loaded device file %s (allow=%d deny=%d)", g.cfg.DeviceFile, len(allow), len(deny))
}

// Allowed reports whether a deviceID may connect under the current policy.
// Fail-open: if the gate is disabled or the file is missing/unparsable, returns true.
func (g *Gate) Allowed(deviceID string) bool {
	if !g.Enabled() {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.refresh()
	if !g.loaded {
		return true // fail-open
	}
	switch g.cfg.Mode {
	case "denylist":
		_, banned := g.deny[deviceID]
		return !banned
	default: // allowlist
		_, ok := g.allow[deviceID]
		return ok
	}
}

// AuthFunc returns a handshake.AuthFunc that admits allowed devices (assigning a
// random session ID, like the server's defaultAuthHook) and rejects the rest.
func (g *Gate) AuthFunc() handshake.AuthFunc {
	return func(deviceID string, _ map[string]any) (string, error) {
		if g.Allowed(deviceID) {
			return uuid.NewString(), nil
		}
		logger.Infof("authz: rejecting device %q (not allowed)", deviceID)
		return "", errors.New("access revoked")
	}
}
