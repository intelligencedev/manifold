package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAppleClientSecret(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	secret, err := GenerateAppleClientSecret(AppleClientSecretOptions{
		TeamID:     "TEAM123456",
		KeyID:      "KEY1234567",
		ClientID:   "com.example.manifold",
		PrivateKey: keyPEM,
		TTL:        time.Hour,
	})
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	token, err := jwt.Parse(secret, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodES256 {
			t.Fatalf("unexpected signing method: %v", token.Method.Alg())
		}
		if token.Header["kid"] != "KEY1234567" {
			t.Fatalf("unexpected kid: %v", token.Header["kid"])
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse generated token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		t.Fatalf("expected valid map claims")
	}
	if claims["iss"] != "TEAM123456" || claims["sub"] != "com.example.manifold" || claims["aud"] != appleAudience {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestGenerateAppleClientSecretRequiresConfig(t *testing.T) {
	t.Parallel()
	if _, err := GenerateAppleClientSecret(AppleClientSecretOptions{}); err == nil {
		t.Fatalf("expected missing config error")
	}
}
