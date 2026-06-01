package auth

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleAudience           = "https://appleid.apple.com"
	defaultAppleSecretTTL   = 180 * 24 * time.Hour
	maxAppleClientSecretTTL = 180 * 24 * time.Hour
)

// AppleClientSecretOptions describes the values required to generate a Sign in
// with Apple client_secret JWT.
type AppleClientSecretOptions struct {
	TeamID         string
	KeyID          string
	ClientID       string
	PrivateKey     string
	PrivateKeyPath string
	TTL            time.Duration
}

// GenerateAppleClientSecret creates the ES256 JWT Apple expects as client_secret.
func GenerateAppleClientSecret(opts AppleClientSecretOptions) (string, error) {
	teamID := strings.TrimSpace(opts.TeamID)
	keyID := strings.TrimSpace(opts.KeyID)
	clientID := strings.TrimSpace(opts.ClientID)
	if teamID == "" || keyID == "" || clientID == "" {
		return "", errors.New("apple teamID, keyID, and clientID are required")
	}
	keyPEM, err := applePrivateKeyPEM(opts)
	if err != nil {
		return "", err
	}
	key, err := parseApplePrivateKey(keyPEM)
	if err != nil {
		return "", err
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultAppleSecretTTL
	}
	if ttl > maxAppleClientSecretTTL {
		ttl = maxAppleClientSecretTTL
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": teamID,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"aud": appleAudience,
		"sub": clientID,
	})
	token.Header["kid"] = keyID
	return token.SignedString(key)
}

func applePrivateKeyPEM(opts AppleClientSecretOptions) (string, error) {
	if strings.TrimSpace(opts.PrivateKey) != "" {
		return opts.PrivateKey, nil
	}
	path := strings.TrimSpace(opts.PrivateKeyPath)
	if path == "" {
		return "", errors.New("apple privateKey or privateKeyPath is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read apple private key: %w", err)
	}
	return string(data), nil
}

func parseApplePrivateKey(raw string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("apple private key PEM block not found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		ecKey, ecErr := x509.ParseECPrivateKey(block.Bytes)
		if ecErr != nil {
			return nil, fmt.Errorf("parse apple private key: %w", err)
		}
		return ecKey, nil
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apple private key must be ECDSA")
	}
	return ecKey, nil
}
