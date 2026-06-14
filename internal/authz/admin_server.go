// Package authz — admin HTTP server for Gate management (Task 11).
// Exposes GET /health and POST /internal/authz/update (Bearer token auth).
// Only started when AuthzAdminAddr is configured (non-empty).
package authz

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/logger"
)

// AdminServer wraps the Gate and exposes a minimal HTTP API for the panel.
type AdminServer struct {
	gate  *Gate
	token string
	addr  string
	srv   *http.Server
	mu    sync.Mutex
}

// NewAdminServer creates an AdminServer for gate g on listenAddr.
// token is the expected Bearer token for write endpoints.
func NewAdminServer(g *Gate, listenAddr, token string) *AdminServer {
	return &AdminServer{gate: g, addr: listenAddr, token: token}
}

// Health returns a snapshot of the gate's current state (no locks leaked to caller).
func (g *Gate) Health() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	stale := false
	if !g.lkgValidAt.IsZero() && g.cfg.LkgMaxAge > 0 {
		stale = now.Sub(g.lkgValidAt) > g.cfg.LkgMaxAge
	}
	lkgStr := ""
	if !g.lkgValidAt.IsZero() {
		lkgStr = g.lkgValidAt.UTC().Format(time.RFC3339)
	}
	allowCount := len(g.lkgAllow)
	if g.loaded && g.allow != nil {
		allowCount = len(g.allow)
	}
	return map[string]any{
		"mode":            g.cfg.Mode,
		"fail_mode":       g.cfg.FailMode,
		"applied_version": g.lastVersion,
		"lkg_valid_at":    lkgStr,
		"load_errors":     g.loadErrors,
		"allow_count":     allowCount,
		"stale":           stale,
	}
}

// WriteAuthz atomically writes data to the Gate's device file and forces a refresh.
// Returns an error if the token doesn't match (caller should check before calling).
func (g *Gate) WriteAuthz(data []byte) error {
	path := g.cfg.DeviceFile
	if path == "" {
		return fmt.Errorf("authz: DeviceFile not configured")
	}
	tmp := path + ".admin.tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("authz: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("authz: rename: %w", err)
	}
	// Force refresh by clearing mtime cache
	g.mu.Lock()
	g.loaded = false
	g.loadedAt = time.Time{}
	g.mu.Unlock()
	// Trigger an immediate load to populate LKG
	_ = g.Allowed("__probe__")
	logger.Infof("authz: admin WriteAuthz: updated %s", path)
	return nil
}

// Start starts the HTTP admin server in a goroutine. Returns immediately.
func (a *AdminServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/internal/authz/update", a.handleUpdate)

	a.mu.Lock()
	a.srv = &http.Server{
		Addr:         a.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	a.mu.Unlock()

	go func() {
		logger.Infof("authz: admin HTTP server listening on %s", a.addr)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warnf("authz: admin HTTP server error: %v", err)
		}
	}()
}

func (a *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := a.gate.Health()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h)
}

func (a *AdminServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Bearer token check
	authHdr := r.Header.Get("Authorization")
	expected := "Bearer " + a.token
	if a.token == "" || !strings.EqualFold(strings.TrimSpace(authHdr), strings.TrimSpace(expected)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if err := a.gate.WriteAuthz(body); err != nil {
		logger.Warnf("authz: admin update error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
