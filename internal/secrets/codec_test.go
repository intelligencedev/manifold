package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testCodec(t *testing.T) Codec {
	t.Helper()
	key := make([]byte, keySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	codec, err := NewCodec(key)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return codec
}

func TestCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	sealed, err := codec.SealString("secret-value", "row/a")
	if err != nil {
		t.Fatalf("SealString: %v", err)
	}
	if !codec.IsSealed(sealed) {
		t.Fatalf("expected sealed envelope, got %q", sealed)
	}
	got, err := codec.OpenString(sealed, "row/a")
	if err != nil {
		t.Fatalf("OpenString: %v", err)
	}
	if got != "secret-value" {
		t.Fatalf("got %q, want secret-value", got)
	}
}

func TestCodecUsesRandomNonce(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	first, err := codec.SealString("same", "row/a")
	if err != nil {
		t.Fatalf("SealString first: %v", err)
	}
	second, err := codec.SealString("same", "row/a")
	if err != nil {
		t.Fatalf("SealString second: %v", err)
	}
	if first == second {
		t.Fatalf("expected different ciphertexts for same plaintext")
	}
}

func TestCodecAADMismatchFails(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	sealed, err := codec.SealString("secret-value", "row/a")
	if err != nil {
		t.Fatalf("SealString: %v", err)
	}
	if _, err := codec.OpenString(sealed, "row/b"); err == nil {
		t.Fatalf("expected AAD mismatch to fail")
	}
}

func TestCodecHandlesEmptyAndPlaintext(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	if got, err := codec.SealString("", "row/a"); err != nil || got != "" {
		t.Fatalf("SealString empty = %q, %v", got, err)
	}
	if got, err := codec.OpenString("plain", "row/a"); err != nil || got != "plain" {
		t.Fatalf("OpenString plaintext = %q, %v", got, err)
	}
}

func TestCodecMalformedEnvelopeFails(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	if _, err := codec.OpenString(envelopePrefix+"not-base64-!", "row/a"); err == nil {
		t.Fatalf("expected malformed base64 to fail")
	}
	shortPayload := envelopePrefix + base64.RawURLEncoding.EncodeToString([]byte("short"))
	if _, err := codec.OpenString(shortPayload, "row/a"); err == nil {
		t.Fatalf("expected short payload to fail")
	}
}

func TestNewCodecRejectsInvalidKeyLength(t *testing.T) {
	t.Parallel()

	if _, err := NewCodec(make([]byte, keySize-1)); err == nil {
		t.Fatalf("expected short key to fail")
	}
	if _, err := NewCodec(make([]byte, keySize+1)); err == nil {
		t.Fatalf("expected long key to fail")
	}
}

func TestNewCodecFromEnv(t *testing.T) {
	t.Setenv(EnvKeyName, base64.StdEncoding.EncodeToString(make([]byte, keySize)))
	codec, err := NewCodecFromEnv()
	if err != nil {
		t.Fatalf("NewCodecFromEnv: %v", err)
	}
	sealed, err := codec.SealString("secret", "row/a")
	if err != nil {
		t.Fatalf("SealString: %v", err)
	}
	if !strings.HasPrefix(sealed, envelopePrefix) {
		t.Fatalf("unexpected envelope %q", sealed)
	}
}
