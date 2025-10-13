package projects

import (
    "bytes"
    "encoding/json"
    "io"
    "os"
    "path/filepath"
    "testing"
)

func TestEncryptionUploadReadRotate(t *testing.T) {
    t.Parallel()
    tmp := t.TempDir()
    svc := NewService(tmp)
    if err := svc.EnableEncryption(true); err != nil {
        t.Fatalf("EnableEncryption error: %v", err)
    }
    userID := int64(42)
    p, err := svc.CreateProject(nil, userID, "Sec")
    if err != nil {
        t.Fatalf("CreateProject: %v", err)
    }

    // enc.json present
    encPath := filepath.Join(tmp, "users", "42", "projects", p.ID, ".meta", "enc.json")
    b, err := os.ReadFile(encPath)
    if err != nil {
        t.Fatalf("read enc.json: %v", err)
    }
    var env map[string]any
    if err := json.Unmarshal(b, &env); err != nil {
        t.Fatalf("parse enc.json: %v", err)
    }
    if env["alg"] != "AES-256-GCM" {
        t.Fatalf("unexpected alg: %v", env["alg"])
    }

    // upload encrypted
    msg := []byte("top secret")
    if err := svc.UploadFile(nil, userID, p.ID, "/", "s.txt", bytes.NewReader(msg)); err != nil {
        t.Fatalf("UploadFile: %v", err)
    }
    rawPath := filepath.Join(tmp, "users", "42", "projects", p.ID, "s.txt")
    raw, err := os.ReadFile(rawPath)
    if err != nil {
        t.Fatalf("read raw: %v", err)
    }
    // header: 'M','G','C','M', 1
    if len(raw) < 5 || raw[0] != 'M' || raw[1] != 'G' || raw[2] != 'C' || raw[3] != 'M' || raw[4] != 1 {
        t.Fatalf("missing encrypted header")
    }
    if bytes.Contains(raw, msg) {
        t.Fatalf("ciphertext leaked plaintext")
    }

    // read back (decrypt)
    rc, err := svc.ReadFile(nil, userID, p.ID, "/s.txt")
    if err != nil {
        t.Fatalf("ReadFile: %v", err)
    }
    got, err := io.ReadAll(rc)
    rc.Close()
    if err != nil {
        t.Fatalf("read decrypted: %v", err)
    }
    if !bytes.Equal(got, msg) {
        t.Fatalf("decrypted mismatch: %q != %q", string(got), string(msg))
    }

    // rotate DEK and verify still readable
    if err := svc.RotateProjectDEK(nil, userID, p.ID); err != nil {
        t.Fatalf("RotateProjectDEK: %v", err)
    }
    rc2, err := svc.ReadFile(nil, userID, p.ID, "/s.txt")
    if err != nil {
        t.Fatalf("ReadFile after rotate: %v", err)
    }
    got2, err := io.ReadAll(rc2)
    rc2.Close()
    if err != nil {
        t.Fatalf("read after rotate: %v", err)
    }
    if !bytes.Equal(got2, msg) {
        t.Fatalf("decrypted after rotate mismatch: %q != %q", string(got2), string(msg))
    }
    // enc.json finalized (no prev_wrapped_dek)
    b2, err := os.ReadFile(encPath)
    if err != nil {
        t.Fatalf("read enc.json2: %v", err)
    }
    var env2 map[string]any
    _ = json.Unmarshal(b2, &env2)
    if _, ok := env2["prev_wrapped_dek"]; ok {
        t.Fatalf("rotation not finalized: prev_wrapped_dek present")
    }
}

