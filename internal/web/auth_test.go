package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	// Test Create
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if session.CSRFToken == "" {
		t.Error("CSRF token should not be empty")
	}

	// Test Get
	retrieved := store.Get(session.ID)
	if retrieved == nil {
		t.Fatal("Failed to retrieve session")
	}
	if retrieved.ID != session.ID {
		t.Errorf("Session ID mismatch: got %s, want %s", retrieved.ID, session.ID)
	}

	// Test Get with invalid ID
	invalid := store.Get("nonexistent")
	if invalid != nil {
		t.Error("Should return nil for nonexistent session")
	}

	// Test Delete
	store.Delete(session.ID)
	deleted := store.Get(session.ID)
	if deleted != nil {
		t.Error("Session should be deleted")
	}
}

func TestAuthHandler_PasswordHashing(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	handler, err := NewAuthHandler(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create auth handler: %v", err)
	}

	// Initially auth should be disabled
	if handler.IsEnabled() {
		t.Error("Auth should be disabled initially")
	}

	// Set password
	password := "testpassword123"
	if err := handler.SetPassword(password); err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Auth should now be enabled
	if !handler.IsEnabled() {
		t.Error("Auth should be enabled after setting password")
	}

	// Check correct password
	if !handler.CheckPassword(password) {
		t.Error("CheckPassword should return true for correct password")
	}

	// Check incorrect password
	if handler.CheckPassword("wrongpassword") {
		t.Error("CheckPassword should return false for incorrect password")
	}

	// Verify config was persisted
	configPath := filepath.Join(tmpDir, "settings", "auth.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Auth config file should exist")
	}
}

func TestAuthHandler_LoginFlow(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	handler, err := NewAuthHandler(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create auth handler: %v", err)
	}

	// Set password
	password := "testpassword123"
	if err := handler.SetPassword(password); err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Test GET /login
	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /login: expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Password") {
		t.Error("Login page should contain Password field")
	}

	// Test POST /login with wrong password
	form := url.Values{}
	form.Add("password", "wrongpassword")
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /login with wrong password: expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid password") {
		t.Error("Should show error for wrong password")
	}

	// Test POST /login with correct password
	form = url.Values{}
	form.Add("password", password)
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /login with correct password: expected redirect (303), got %d", rec.Code)
	}

	// Check that session cookie was set
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("Session cookie should be set after successful login")
	}
}

func TestAuthHandler_SetupFlow(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	handler, err := NewAuthHandler(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create auth handler: %v", err)
	}

	// Test GET /setup
	req := httptest.NewRequest("GET", "/setup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /setup: expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Create Dashboard Password") {
		t.Error("Setup page should contain setup header")
	}

	// Test POST /setup with mismatched passwords
	form := url.Values{}
	form.Add("password", "testpassword123")
	form.Add("confirm", "differentpassword")
	req = httptest.NewRequest("POST", "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "Passwords do not match") {
		t.Error("Should show error for mismatched passwords")
	}

	// Test POST /setup with short password
	form = url.Values{}
	form.Add("password", "short")
	form.Add("confirm", "short")
	req = httptest.NewRequest("POST", "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "at least 8 characters") {
		t.Error("Should show error for short password")
	}

	// Test POST /setup with valid password
	form = url.Values{}
	form.Add("password", "testpassword123")
	form.Add("confirm", "testpassword123")
	req = httptest.NewRequest("POST", "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /setup with valid password: expected redirect (303), got %d", rec.Code)
	}

	// Auth should now be enabled
	if !handler.IsEnabled() {
		t.Error("Auth should be enabled after setup")
	}
}

func TestAuthHandler_ProtectedRoutes(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	handler, err := NewAuthHandler(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create auth handler: %v", err)
	}

	// Register a protected handler
	handler.RegisterProtected("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("protected content"))
	}))

	// Set password
	if err := handler.SetPassword("testpassword123"); err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Access protected route without session - should redirect
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("Protected route without session: expected redirect (303), got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/login" {
		t.Errorf("Should redirect to /login, got %s", rec.Header().Get("Location"))
	}

	// Login to get session
	form := url.Values{}
	form.Add("password", "testpassword123")
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Get session cookie
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}

	// Access protected route with session - should work
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Protected route with session: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "protected content") {
		t.Error("Should show protected content with valid session")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	token1, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	if len(token1) != 64 { // hex encoded 32 bytes = 64 chars
		t.Errorf("Token length should be 64, got %d", len(token1))
	}

	// Tokens should be unique
	token2, _ := generateSecureToken(32)
	if token1 == token2 {
		t.Error("Generated tokens should be unique")
	}
}
