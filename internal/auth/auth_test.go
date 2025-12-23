package auth

import (
	"bytes"
	"encoding/base64"
	"time"
)

import "testing"

func TestHashPassword_CheckPassword(t *testing.T) {
	t.Parallel()

	if _, err := HashPassword("short"); err == nil {
		t.Fatalf("expected error for short password")
	}

	hash, err := HashPassword("  12345678  ")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if CheckPassword("", "12345678") {
		t.Fatalf("expected CheckPassword to fail for empty hash")
	}
	if !CheckPassword(hash, "12345678") {
		t.Fatalf("expected CheckPassword to succeed")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatalf("expected CheckPassword to fail for wrong password")
	}
}

func TestRandomSecret_Base64(t *testing.T) {
	t.Parallel()

	sec, err := RandomSecret(0)
	if err != nil {
		t.Fatalf("RandomSecret: %v", err)
	}
	if len(sec) != 32 {
		t.Fatalf("expected default len=32, got %d", len(sec))
	}

	enc := EncodeSecretBase64(sec)
	dec, err := DecodeSecretBase64(enc)
	if err != nil {
		t.Fatalf("DecodeSecretBase64: %v", err)
	}
	if !bytes.Equal(sec, dec) {
		t.Fatalf("expected round-trip secret")
	}

	if _, err := DecodeSecretBase64(" "); err == nil {
		t.Fatalf("expected error for empty secret")
	}
}

func TestSignVerifyToken_RoundTrip_AndFailureModes(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := SignToken(nil, 1, now.Add(time.Hour)); err == nil {
		t.Fatalf("expected error for missing secret")
	}

	token, err := SignToken(secret, 42, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	claims, ok := VerifyToken(secret, token, now)
	if !ok {
		t.Fatalf("expected token to verify")
	}
	if claims.UserID != 42 {
		t.Fatalf("expected uid=42, got %d", claims.UserID)
	}

	if _, ok := VerifyToken(secret, token, now.Add(2*time.Hour)); ok {
		t.Fatalf("expected expired token to fail")
	}
	if _, ok := VerifyToken([]byte("different-secret-32-bytes-length!!"), token, now); ok {
		t.Fatalf("expected wrong secret to fail")
	}
	if _, ok := VerifyToken(secret, "", now); ok {
		t.Fatalf("expected empty token to fail")
	}
	if _, ok := VerifyToken(secret, "v2.x.y", now); ok {
		t.Fatalf("expected wrong version to fail")
	}
	if _, ok := VerifyToken(secret, "v1.onlytwo", now); ok {
		t.Fatalf("expected wrong parts count to fail")
	}

	negUID, err := SignToken(secret, -1, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, ok := VerifyToken(secret, negUID, now); ok {
		t.Fatalf("expected uid<=0 to fail")
	}

	exp0, err := SignToken(secret, 1, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, ok := VerifyToken(secret, exp0, now); ok {
		t.Fatalf("expected exp<=0 to fail")
	}
}

func TestVerifyToken_BadPayloadDecodeAndJSON(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Valid signature but payload part is not valid base64url.
	msg1 := "v1.###"
	token1 := msg1 + "." + sign(secret, msg1)
	if _, ok := VerifyToken(secret, token1, now); ok {
		t.Fatalf("expected invalid base64 payload to fail")
	}

	// Valid signature but payload decodes to non-JSON.
	raw := []byte("not-json")
	payload := base64.RawURLEncoding.EncodeToString(raw)
	msg2 := "v1." + payload
	token2 := msg2 + "." + sign(secret, msg2)
	if _, ok := VerifyToken(secret, token2, now); ok {
		t.Fatalf("expected invalid json payload to fail")
	}
}

