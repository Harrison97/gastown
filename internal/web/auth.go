package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "gt_session"
	sessionDuration   = 24 * time.Hour
	csrfTokenLength   = 32
)

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	PasswordHash string `json:"password_hash"`
	Enabled      bool   `json:"enabled"`
}

// Session represents an authenticated session.
type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	CSRFToken string    `json:"csrf_token"`
}

// SessionStore manages active sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a new session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// Create creates a new session and returns it.
func (s *SessionStore) Create() (*Session, error) {
	id, err := generateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("generating session ID: %w", err)
	}

	csrfToken, err := generateSecureToken(csrfTokenLength)
	if err != nil {
		return nil, fmt.Errorf("generating CSRF token: %w", err)
	}

	session := &Session{
		ID:        id,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sessionDuration),
		CSRFToken: csrfToken,
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()

	return session, nil
}

// Get retrieves a session by ID, returning nil if not found or expired.
func (s *SessionStore) Get(id string) *Session {
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(session.ExpiresAt) {
		s.Delete(id)
		return nil
	}

	return session
}

// Delete removes a session.
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// CleanExpired removes all expired sessions.
func (s *SessionStore) CleanExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

// AuthHandler wraps protected handlers with authentication.
type AuthHandler struct {
	config       *AuthConfig
	configPath   string
	sessions     *SessionStore
	loginTmpl    string
	protectedMux *http.ServeMux
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(townRoot string) (*AuthHandler, error) {
	configPath := filepath.Join(townRoot, "settings", "auth.json")

	ah := &AuthHandler{
		configPath:   configPath,
		sessions:     NewSessionStore(),
		protectedMux: http.NewServeMux(),
	}

	// Load or create auth config
	if err := ah.loadConfig(); err != nil {
		return nil, err
	}

	// Start cleanup goroutine
	go ah.cleanupLoop()

	return ah, nil
}

// loadConfig loads auth configuration from file.
func (ah *AuthHandler) loadConfig() error {
	data, err := os.ReadFile(ah.configPath)
	if os.IsNotExist(err) {
		// No config yet - auth is disabled until password is set
		ah.config = &AuthConfig{Enabled: false}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading auth config: %w", err)
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing auth config: %w", err)
	}

	ah.config = &config
	return nil
}

// saveConfig saves auth configuration to file.
func (ah *AuthHandler) saveConfig() error {
	// Ensure settings directory exists
	if err := os.MkdirAll(filepath.Dir(ah.configPath), 0750); err != nil {
		return fmt.Errorf("creating settings dir: %w", err)
	}

	data, err := json.MarshalIndent(ah.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling auth config: %w", err)
	}

	if err := os.WriteFile(ah.configPath, data, 0600); err != nil {
		return fmt.Errorf("writing auth config: %w", err)
	}

	return nil
}

// SetPassword sets a new password and enables authentication.
func (ah *AuthHandler) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	ah.config.PasswordHash = string(hash)
	ah.config.Enabled = true

	return ah.saveConfig()
}

// CheckPassword verifies a password against the stored hash.
func (ah *AuthHandler) CheckPassword(password string) bool {
	if !ah.config.Enabled || ah.config.PasswordHash == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(ah.config.PasswordHash), []byte(password))
	return err == nil
}

// IsEnabled returns whether authentication is enabled.
func (ah *AuthHandler) IsEnabled() bool {
	return ah.config != nil && ah.config.Enabled
}

// cleanupLoop periodically cleans expired sessions.
func (ah *AuthHandler) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ah.sessions.CleanExpired()
	}
}

// RegisterProtected registers a handler for a protected route.
func (ah *AuthHandler) RegisterProtected(pattern string, handler http.Handler) {
	ah.protectedMux.Handle(pattern, handler)
}

// ServeHTTP implements http.Handler.
func (ah *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle login/logout routes
	switch r.URL.Path {
	case "/login":
		ah.handleLogin(w, r)
		return
	case "/logout":
		ah.handleLogout(w, r)
		return
	case "/setup":
		ah.handleSetup(w, r)
		return
	}

	// If auth is not enabled, serve protected content directly
	if !ah.IsEnabled() {
		// Redirect to setup if accessing root
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		ah.protectedMux.ServeHTTP(w, r)
		return
	}

	// Check for valid session
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || ah.sessions.Get(cookie.Value) == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Valid session - serve protected content
	ah.protectedMux.ServeHTTP(w, r)
}

// handleLogin handles GET/POST /login.
func (ah *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		ah.renderLoginPage(w, "")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		ah.renderLoginPage(w, "Invalid form data")
		return
	}

	password := r.FormValue("password")

	// Check password
	if !ah.CheckPassword(password) {
		ah.renderLoginPage(w, "Invalid password")
		return
	}

	// Create session
	session, err := ah.sessions.Create()
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout handles POST /logout.
func (ah *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get and delete session
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		ah.sessions.Delete(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleSetup handles GET/POST /setup for initial password configuration.
func (ah *AuthHandler) handleSetup(w http.ResponseWriter, r *http.Request) {
	// If auth is already enabled, redirect to login
	if ah.IsEnabled() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		ah.renderSetupPage(w, "")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		ah.renderSetupPage(w, "Invalid form data")
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	// Validate password
	if len(password) < 8 {
		ah.renderSetupPage(w, "Password must be at least 8 characters")
		return
	}

	if password != confirm {
		ah.renderSetupPage(w, "Passwords do not match")
		return
	}

	// Set password
	if err := ah.SetPassword(password); err != nil {
		ah.renderSetupPage(w, "Failed to save password")
		return
	}

	// Create session and log in
	session, err := ah.sessions.Create()
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderLoginPage renders the login page.
func (ah *AuthHandler) renderLoginPage(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := struct {
		Error string
	}{
		Error: errorMsg,
	}

	if err := loginTemplate.Execute(w, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// renderSetupPage renders the initial password setup page.
func (ah *AuthHandler) renderSetupPage(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := struct {
		Error string
	}{
		Error: errorMsg,
	}

	if err := setupTemplate.Execute(w, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateCSRFToken generates a CSRF token for forms.
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// validateCSRFToken validates a CSRF token using constant-time comparison.
func validateCSRFToken(expected, actual string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}
