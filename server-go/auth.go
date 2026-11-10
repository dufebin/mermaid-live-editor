package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

const (
	sessionCookie = "mermaid_session"
	sessionTTL    = 7 * 24 * time.Hour
	scryptN       = 32768
	scryptR       = 8
	scryptP       = 1
	scryptKeyLen  = 32
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	readRandom(salt)
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("scrypt$%d$%d$%d$%s$%s", scryptN, scryptR, scryptP,
		base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false
	}
	var n, r, p int
	if _, err := fmt.Sscanf(strings.Join(parts[1:4], "$"), "%d$%d$%d", &n, &r, &p); err != nil {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got, err := scrypt.Key([]byte(password), salt, n, r, p, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func base64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Sub string `json:"sub"`
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func signJWT(userID string, issued time.Time, secret []byte) string {
	header := base64url(mustJSON(jwtHeader{Alg: "HS256", Typ: "JWT"}))
	claims := base64url(mustJSON(jwtClaims{Exp: issued.Add(sessionTTL).Unix(), Iat: issued.Unix(), Sub: userID}))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(header + "." + claims))
	return header + "." + claims + "." + base64url(mac.Sum(nil))
}

func verifyJWT(token string, secret []byte, now time.Time) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	var header jwtHeader
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerRaw, &header) != nil || header.Alg != "HS256" {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac.Sum(nil), want) {
		return "", false
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims jwtClaims
	if err != nil || json.Unmarshal(claimsRaw, &claims) != nil {
		return "", false
	}
	if now.Unix() >= claims.Exp || claims.Sub == "" {
		return "", false
	}
	return claims.Sub, true
}

func validateEmail(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(email, "@")
	if at <= 0 || len(email)-at <= 1 {
		return "", false
	}
	domain := email[at+1:]
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 || dot == len(domain)-1 || strings.ContainsAny(email, " \t") {
		return "", false
	}
	return email, true
}

func validatePassword(password string) bool {
	return len(password) >= 8 && len(password) <= 128
}
