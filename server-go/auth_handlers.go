package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// loadOrCreateSecret: MERMAID_JWT_SECRET wins; else persist 32 random bytes to <dataDir>/jwt-secret.
func loadOrCreateSecret(dataDir string) ([]byte, error) {
	if secret := os.Getenv("MERMAID_JWT_SECRET"); secret != "" {
		return []byte(secret), nil
	}
	path := filepath.Join(dataDir, "jwt-secret")
	if secret, err := os.ReadFile(path); err == nil && len(secret) >= 32 {
		return secret, nil
	}
	secret := make([]byte, 32)
	readRandom(secret)
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		MaxAge:   int(sessionTTL.Seconds()),
		Name:     sessionCookie,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Value:    token,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		MaxAge:   -1,
		Name:     sessionCookie,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeAuthBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "Invalid JSON body")
		return false
	}
	return true
}

func (a *api) userIDFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return verifyJWT(cookie.Value, a.secret, time.Now())
}

func (a *api) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		a.notFound(w, r)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	email, ok := validateEmail(body.Email)
	if !ok {
		writeError(w, 400, "email must be a valid address")
		return
	}
	if !validatePassword(body.Password) {
		writeError(w, 400, "password must be 8-128 characters")
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if a.s.getUserByEmail(email) != nil {
		writeError(w, 409, "email already registered")
		return
	}
	user, err := a.s.createUser(email, hash)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if a.s.countUsers() == 1 {
		if err := a.s.claimUnowned(user.ID); err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}
	setSessionCookie(w, signJWT(user.ID, time.Now(), a.secret))
	writeJSON(w, 201, user)
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		a.notFound(w, r)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	email, ok := validateEmail(body.Email)
	if !ok {
		writeError(w, 401, "invalid email or password")
		return
	}
	hash := a.s.getPasswordHash(email)
	if hash == "" || !verifyPassword(body.Password, hash) {
		writeError(w, 401, "invalid email or password")
		return
	}
	user := a.s.getUserByEmail(email)
	if user == nil {
		writeError(w, 401, "invalid email or password")
		return
	}
	setSessionCookie(w, signJWT(user.ID, time.Now(), a.secret))
	writeJSON(w, 200, user)
}

func (a *api) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		a.notFound(w, r)
		return
	}
	clearSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *api) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		a.notFound(w, r)
		return
	}
	userID, ok := a.userIDFromRequest(r)
	if !ok {
		writeError(w, 401, "unauthorized")
		return
	}
	user := a.s.getUser(userID)
	if user == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, user)
}
