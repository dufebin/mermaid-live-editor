package main

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundtrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery")
	if err != nil || !strings.HasPrefix(hash, "scrypt$32768$8$1$") {
		t.Fatalf("hash: %q err=%v", hash, err)
	}
	if !verifyPassword("correct horse battery", hash) {
		t.Fatal("correct password must verify")
	}
	if verifyPassword("wrong", hash) {
		t.Fatal("wrong password must not verify")
	}
	if verifyPassword("correct horse battery", "scrypt$32768$8$1$AAAA$BBBB") {
		t.Fatal("corrupt hash must not verify")
	}
}

func TestJWTRoundtrip(t *testing.T) {
	secret := []byte("s3cret")
	issued := time.Unix(1_700_000_000, 0)
	token := signJWT("user-1", issued, secret)
	uid, ok := verifyJWT(token, secret, issued.Add(time.Hour))
	if !ok || uid != "user-1" {
		t.Fatalf("roundtrip: %q %v", uid, ok)
	}
}

func TestJWTVerificationFailures(t *testing.T) {
	secret := []byte("s3cret")
	issued := time.Unix(1_700_000_000, 0)
	token := signJWT("user-1", issued, secret)
	now := issued.Add(time.Hour)

	if _, ok := verifyJWT(token, []byte("other"), now); ok {
		t.Fatal("wrong secret must fail")
	}
	if _, ok := verifyJWT(token+"x", secret, now); ok {
		t.Fatal("tampered signature must fail")
	}
	parts := strings.Split(token, ".")
	swapped := parts[0] + "." + base64url(mustJSON(jwtClaims{Exp: 9_999_999_999, Iat: 1, Sub: "attacker"})) + "." + parts[2]
	if _, ok := verifyJWT(swapped, secret, now); ok {
		t.Fatal("swapped payload must fail")
	}
	if _, ok := verifyJWT(token, secret, issued.Add(sessionTTL+time.Minute)); ok {
		t.Fatal("expired token must fail")
	}
	forged := base64url(mustJSON(jwtHeader{Alg: "HS512", Typ: "JWT"})) + "." + parts[1] + "." + parts[2]
	if _, ok := verifyJWT(forged, secret, now); ok {
		t.Fatal("non-HS256 alg must fail")
	}
	if _, ok := verifyJWT("not-a-jwt", secret, now); ok {
		t.Fatal("malformed token must fail")
	}
}

func TestValidation(t *testing.T) {
	if e, ok := validateEmail("User@Example.COM"); !ok || e != "user@example.com" {
		t.Fatalf("email normalize: %q %v", e, ok)
	}
	for _, bad := range []string{"nope", "a@b", "@b.com", "a@.com", ""} {
		if _, ok := validateEmail(bad); ok {
			t.Fatalf("bad email accepted: %q", bad)
		}
	}
	for _, bad := range []string{"", "short", strings.Repeat("x", 129)} {
		if validatePassword(bad) {
			t.Fatalf("bad password accepted: len=%d", len(bad))
		}
	}
	if !validatePassword("eightchars") {
		t.Fatal("8-char password must be valid")
	}
}
