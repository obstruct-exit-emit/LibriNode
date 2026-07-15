package api

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/librinode/librinode/internal/config"
)

// Session cookie: HttpOnly, SameSite=Lax, 30-day expiry. Sessions live in
// memory — a restart logs everyone out (the README says to expect that).
const (
	sessionCookie = "librinode_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// Password hashing: PBKDF2-SHA256 (stdlib crypto/pbkdf2), format
// "pbkdf2-sha256$<iterations>$<salt hex>$<hash hex>".
const pbkdf2Iterations = 600_000

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		pbkdf2Iterations, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func verifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// sessionStore tracks login sessions in memory.
type sessionStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time // token -> expiry
}

func newSessionStore() *sessionStore {
	return &sessionStore{tokens: map[string]time.Time{}}
}

func (st *sessionStore) create() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	token := hex.EncodeToString(b)
	st.mu.Lock()
	// Prune expired sessions here — logins are rare, and expired tokens are
	// otherwise only deleted when presented again.
	now := time.Now()
	for t, expiry := range st.tokens {
		if now.After(expiry) {
			delete(st.tokens, t)
		}
	}
	st.tokens[token] = now.Add(sessionTTL)
	st.mu.Unlock()
	return token
}

func (st *sessionStore) valid(token string) bool {
	if token == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	expiry, ok := st.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(st.tokens, token)
		return false
	}
	return true
}

func (st *sessionStore) revoke(token string) {
	st.mu.Lock()
	delete(st.tokens, token)
	st.mu.Unlock()
}

// hasSession reports whether the request carries a valid session cookie.
func (s *server) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.sessions.valid(c.Value)
}

func (s *server) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleAuthStatus is unauthenticated: the UI needs it to decide between the
// login page, the API-key prompt, and going straight in.
func (s *server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authEnabled":   s.cfg.AuthSettings().Enabled(),
		"authenticated": s.hasSession(r),
	})
}

// handleLogin is unauthenticated by nature. Failed attempts are logged and
// slowed down a little.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	auth := s.cfg.AuthSettings()
	if !auth.Enabled() {
		writeError(w, http.StatusBadRequest, "authentication is not enabled")
		return
	}
	ok := false
	for i := range auth.Users {
		u := &auth.Users[i]
		if subtle.ConstantTimeCompare([]byte(req.Username), []byte(u.Username)) == 1 &&
			verifyPassword(u.PasswordHash, req.Password) {
			ok = true
			break
		}
	}
	if !ok {
		slog.Warn("failed login attempt", "username", req.Username, "remote", r.RemoteAddr)
		time.Sleep(500 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	s.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// handleSetCredentials creates or updates a login account, or disables
// authentication entirely (empty username removes every user). Kept for the
// setup wizard and scripts; the Settings UI manages users individually. The
// response sets a fresh session so the browser that just enabled auth stays
// signed in.
func (s *server) handleSetCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	if req.Username == "" {
		if err := s.cfg.SetAuth(config.AuthSettings{}); err != nil {
			writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"authEnabled": false})
		return
	}

	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing password: "+err.Error())
		return
	}
	// Upsert: change the existing user's password, or add a new account.
	if s.cfg.AuthSettings().Find(req.Username) != nil {
		err = s.cfg.SetUserPassword(req.Username, hash)
	} else {
		err = s.cfg.AddUser(req.Username, hash)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
		return
	}
	s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"authEnabled": true})
}

// --- User management (Settings → Security) ---

// handleListUsers returns the login accounts (never their hashes).
func (s *server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"users": s.cfg.AuthSettings().Users})
}

// handleAddUser creates an additional login account (the first one becomes
// the default).
func (s *server) handleAddUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing password: "+err.Error())
		return
	}
	if err := s.cfg.AddUser(req.Username, hash); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	slog.Info("user added", "username", req.Username)
	writeJSON(w, http.StatusCreated, map[string]any{"users": s.cfg.AuthSettings().Users})
}

// handleRemoveUser deletes a login account; the default user is refused.
func (s *server) handleRemoveUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := s.cfg.RemoveUser(username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Info("user removed", "username", username)
	writeJSON(w, http.StatusOK, map[string]any{"users": s.cfg.AuthSettings().Users})
}

// handleSetUserPassword changes one account's password.
func (s *server) handleSetUserPassword(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing password: "+err.Error())
		return
	}
	if err := s.cfg.SetUserPassword(username, hash); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Info("user password changed", "username", username)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMakeDefaultUser promotes an account to the protected default.
func (s *server) handleMakeDefaultUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := s.cfg.SetDefaultUser(username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Info("default user changed", "username", username)
	writeJSON(w, http.StatusOK, map[string]any{"users": s.cfg.AuthSettings().Users})
}

// setupNeeded reports whether this instance is claimable by its first visitor:
// no login account and nothing configured yet — a genuinely fresh install.
// A used instance (any root folder, indexer, or download client) is never
// claimable, so the open setup endpoint can't hijack an instance whose owner
// simply skipped creating an account and relies on the API key.
func (s *server) setupNeeded() bool {
	if s.cfg.AuthSettings().Enabled() {
		return false
	}
	if folders, err := s.store.ListRootFolders(); err != nil || len(folders) > 0 {
		return false
	}
	if indexers, err := s.indexers.Store().List(); err != nil || len(indexers) > 0 {
		return false
	}
	if clients, err := s.downloads.Store().List(); err != nil || len(clients) > 0 {
		return false
	}
	return true
}

// handleSetupStatus tells the web UI whether to open the first-run wizard
// instead of asking for the API key. Unauthenticated — it must answer before
// any credentials exist.
func (s *server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"needed": s.setupNeeded()})
}

// handleSetup claims a fresh instance: creates the login account and signs
// this browser in, in one step — the first-run wizard's entry point, no API
// key required. Refused (403) the moment the instance has an account or any
// configuration.
func (s *server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !s.setupNeeded() {
		writeError(w, http.StatusForbidden, "this instance is already set up")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing password: "+err.Error())
		return
	}
	if err := s.cfg.AddUser(req.Username, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
		return
	}
	slog.Info("instance claimed via first-run setup", "username", req.Username)
	s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRegenerateAPIKey mints a fresh API key; every integration using the
// old one (Prowlarr, scripts) must be updated.
func (s *server) handleRegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.cfg.RegenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
		return
	}
	slog.Info("API key regenerated")
	writeJSON(w, http.StatusOK, map[string]string{"apiKey": key})
}
