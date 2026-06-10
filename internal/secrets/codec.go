package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	EnvKeyName = "MANIFOLD_SECRETS_KEY"

	envelopePrefix = "mfdsec:v1:xchacha20poly1305:"
	keySize        = chacha20poly1305.KeySize
)

// Codec encrypts and decrypts persisted secret strings.
type Codec interface {
	SealString(plaintext, aad string) (string, error)
	OpenString(value, aad string) (string, error)
	IsSealed(value string) bool
}

type xchachaCodec struct {
	key [keySize]byte
}

// NewCodec creates a secret codec from 32 raw key bytes.
func NewCodec(key []byte) (Codec, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secrets key must be %d bytes, got %d", keySize, len(key))
	}
	c := &xchachaCodec{}
	copy(c.key[:], key)
	return c, nil
}

// NewCodecFromEnv creates a codec from MANIFOLD_SECRETS_KEY.
func NewCodecFromEnv() (Codec, error) {
	value := strings.TrimSpace(os.Getenv(EnvKeyName))
	if value == "" {
		return nil, fmt.Errorf("%s is required for database-backed secret storage; generate one with: openssl rand -base64 32", EnvKeyName)
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(value)
	}
	if err != nil {
		key, err = base64.URLEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded 32 raw bytes: %w", EnvKeyName, err)
	}
	codec, err := NewCodec(key)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded 32 raw bytes: %w", EnvKeyName, err)
	}
	return codec, nil
}

func (c *xchachaCodec) SealString(plaintext, aad string) (string, error) {
	if plaintext == "" || c.IsSealed(plaintext) {
		return plaintext, nil
	}
	aead, err := chacha20poly1305.NewX(c.key[:])
	if err != nil {
		return "", fmt.Errorf("initialize secret cipher: %w", err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), []byte(aad))
	return envelopePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *xchachaCodec) OpenString(value, aad string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !c.IsSealed(value) {
		return value, nil
	}
	encoded := strings.TrimPrefix(value, envelopePrefix)
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret envelope: %w", err)
	}
	if len(payload) < chacha20poly1305.NonceSizeX {
		return "", errors.New("secret envelope payload is too short")
	}
	aead, err := chacha20poly1305.NewX(c.key[:])
	if err != nil {
		return "", fmt.Errorf("initialize secret cipher: %w", err)
	}
	nonce := payload[:chacha20poly1305.NonceSizeX]
	ciphertext := payload[chacha20poly1305.NonceSizeX:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("open secret envelope: %w", err)
	}
	return string(plaintext), nil
}

func (c *xchachaCodec) IsSealed(value string) bool {
	return strings.HasPrefix(value, envelopePrefix)
}
