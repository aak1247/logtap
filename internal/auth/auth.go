package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return "", errors.New("password too short")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash string, password string) bool {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

type Claims struct {
	UserID int64 `json:"uid"`
	Exp    int64 `json:"exp"`
}

func RandomSecret(n int) ([]byte, error) {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func EncodeSecretBase64(secret []byte) string {
	return base64.RawStdEncoding.EncodeToString(secret)
}

func DecodeSecretBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty secret")
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func SignToken(secret []byte, userID int64, expiresAt time.Time) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("missing secret")
	}
	c := Claims{UserID: userID, Exp: expiresAt.UTC().Unix()}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(payload)
	msg := "v1." + p
	sig := sign(secret, msg)
	return msg + "." + sig, nil
}

func VerifyToken(secret []byte, token string, now time.Time) (Claims, bool) {
	token = strings.TrimSpace(token)
	if token == "" || len(secret) == 0 {
		return Claims{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, false
	}
	if parts[0] != "v1" {
		return Claims{}, false
	}
	msg := parts[0] + "." + parts[1]
	want := sign(secret, msg)
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(want)) != 1 {
		return Claims{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, false
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, false
	}
	if c.UserID <= 0 {
		return Claims{}, false
	}
	if c.Exp <= 0 || now.UTC().Unix() > c.Exp {
		return Claims{}, false
	}
	return c, true
}

func sign(secret []byte, msg string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum)
}

