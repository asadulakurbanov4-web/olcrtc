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
	"strings"
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
	FailMode        string        // "lkg" (prod default for paid) | "open" (diag) | "closed"
	LkgMaxAge       time.Duration // if lkgValidAt older than this and no fresh success -> escalate (alert + optional closed); 0=disabled
}

// fileSchema mirrors what the panel writes (services/authz_writer.py).
type fileSchema struct {
	Version   int      `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny"`
}

// Gate evaluates deviceIDs against the allowlist file with an mtime cache.
// LKG (last-known-good): on read/parse/version error we freeze on the last *successfully validated*
// allow/deny instead of failing open (prod paid default). See OlcPanel v2.1 §5.2.
type Gate struct {
	cfg Config

	mu       sync.Mutex
	loadedAt time.Time // mtime of the file at last successful load
	allow    map[string]struct{}
	deny     map[string]struct{}
	loaded   bool

	// LKG cache (updated ONLY on successful parse+validate+version check)
	lkgAllow   map[string]struct{}
	lkgDeny    map[string]struct{}
	lkgValidAt time.Time
	loadErrors int // cumulative read/parse/schema/version errors since last good load

	lastVersion int // last successfully accepted version (for monotonic reject without resetting LKG)
}

// New builds a Gate. A nil-ish config (Mode off/"") yields an always-allow gate.
// FailMode defaults to "lkg" when using restrictive modes (defensive for paid); "open" only if explicitly set.
func New(cfg Config) *Gate {
	if cfg.FailMode == "" {
		if cfg.Mode == "allowlist" || cfg.Mode == "denylist" {
			cfg.FailMode = "lkg"
		} else {
			cfg.FailMode = "open"
		}
	}
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
// LKG semantics (prod default): errors (missing, unreadable, bad JSON, schema, version rollback)
// NEVER expand the allow set. We keep the last successfully validated lkgAllow/lkgDeny.
// Only success path updates LKG. Cold-start (no prior good ever) with fail_mode=lkg/closed → deny-all.
func (g *Gate) refresh() {
	fi, err := os.Stat(g.cfg.DeviceFile)
	if err != nil {
		// Missing/unreadable file → use LKG (never fail-open in lkg mode). Increment error.
		g.loadErrors++
		if g.loaded {
			logger.Warnf("authz: device file %s unreadable (%v); using LKG (errors=%d)", g.cfg.DeviceFile, err, g.loadErrors)
		}
		// do not clear loaded/lkg; Allowed will decide from LKG or policy
		g.allow, g.deny, g.loaded = nil, nil, false
		g.escalateIfStale()
		return
	}
	if g.loaded && fi.ModTime().Equal(g.loadedAt) {
		return // unchanged, fast path
	}
	data, err := os.ReadFile(g.cfg.DeviceFile)
	if err != nil {
		g.loadErrors++
		logger.Warnf("authz: read %s failed (%v); using LKG (errors=%d)", g.cfg.DeviceFile, err, g.loadErrors)
		g.allow, g.deny, g.loaded = nil, nil, false
		g.escalateIfStale()
		return
	}
	var fs fileSchema
	if err := json.Unmarshal(data, &fs); err != nil {
		g.loadErrors++
		logger.Warnf("authz: parse %s failed (%v); using LKG (errors=%d)", g.cfg.DeviceFile, err, g.loadErrors)
		g.allow, g.deny, g.loaded = nil, nil, false
		g.escalateIfStale()
		return
	}

	// Strict monotonic version check (core of LKG + version-reject per 4.5/5.2):
	// If we have previously accepted a version and the new one is <= it, treat as rollback/stale writer.
	// Do NOT update cached or LKG; inc errors; stay on previous good state.
	if g.loaded && fs.Version <= g.lastVersion {
		g.loadErrors++
		logger.Warnf("authz: version rollback or stale (%d <= last %d) on %s; using LKG (errors=%d)", fs.Version, g.lastVersion, g.cfg.DeviceFile, g.loadErrors)
		g.allow, g.deny, g.loaded = nil, nil, false
		g.escalateIfStale()
		return
	}
	if fs.Version <= 0 {
		// never accept non-positive as a new good version
		g.loadErrors++
		logger.Warnf("authz: non-positive version %d on %s; using LKG (errors=%d)", fs.Version, g.cfg.DeviceFile, g.loadErrors)
		g.allow, g.deny, g.loaded = nil, nil, false
		g.escalateIfStale()
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

	// Success path: accept, update cached + LKG + lastVersion (LKG only on good)
	g.allow, g.deny, g.loadedAt, g.loaded = allow, deny, fi.ModTime(), true
	g.lkgAllow, g.lkgDeny, g.lkgValidAt = copySet(allow), copySet(deny), time.Now()
	g.lastVersion = fs.Version
	g.loadErrors = 0 // reset on success
	logger.Infof("authz: loaded device file %s (version=%d allow=%d deny=%d)", g.cfg.DeviceFile, fs.Version, len(allow), len(deny))
}

func copySet(in map[string]struct{}) map[string]struct{} {
	if in == nil {
		return nil
	}
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func (g *Gate) escalateIfStale() {
	if g.cfg.LkgMaxAge <= 0 || g.lkgValidAt.IsZero() {
		return
	}
	if time.Since(g.lkgValidAt) > g.cfg.LkgMaxAge {
		logger.Warnf("authz: LKG stale (age>%s, errors=%d); escalate (consider closed or alert)", g.cfg.LkgMaxAge, g.loadErrors)
		// In full impl: increment critical counter / emit metric / optionally force closed behavior in Allowed.
		// For now: loud log is the signal (Telegram alert comes from panel health-poll on load_errors / lkg age).
	}
}

// Allowed reports whether a deviceID may connect under the current policy.
// With fail_mode=lkg (prod): missing/bad/rollback file → decide from last-good LKG (never expands allow).
// Cold-start (no LKG ever seen) under lkg/closed → deny (closed).
// fail_mode=open → admit on error (diag only).
// fail_mode=closed → deny on error (hard ceiling).
func (g *Gate) Allowed(deviceID string) bool {
	if !g.Enabled() {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.refresh()

	// Determine effective allow/deny set for decision
	effAllow := g.allow
	effDeny := g.deny
	if !g.loaded {
		// error path (missing, bad, rollback, etc.)
		fm := strings.ToLower(strings.TrimSpace(g.cfg.FailMode))
		switch fm {
		case "open":
			return true
		case "closed":
			return false
		case "lkg", "": // default to lkg for safety in paid
			if len(g.lkgAllow) > 0 || len(g.lkgDeny) > 0 {
				effAllow, effDeny = g.lkgAllow, g.lkgDeny
			} else {
				// cold-start: never had a good file → closed behavior (deny all)
				return false
			}
		default:
			// unknown → conservative closed
			return false
		}
	}

	switch g.cfg.Mode {
	case "denylist":
		_, banned := effDeny[deviceID]
		return !banned
	default: // allowlist
		_, ok := effAllow[deviceID]
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
