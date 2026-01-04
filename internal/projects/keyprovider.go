// Package projects provides project storage and encryption functionality.
package projects

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// KeyProvider defines the interface for wrapping and unwrapping Data Encryption Keys (DEKs).
// Implementations manage the Key Encryption Key (KEK) lifecycle and provide envelope
// encryption capabilities for project files.
//
// The design follows envelope encryption best practices:
//   - DEKs are generated locally for each project
//   - DEKs are wrapped (encrypted) by the KeyProvider before storage
//   - KEKs never leave the KeyProvider (KMS/Vault/file-based)
//
// Thread-safety: Implementations must be safe for concurrent use.
type KeyProvider interface {
	// WrapDEK encrypts a Data Encryption Key using the provider's Key Encryption Key.
	// The projectID is used for key derivation/scoping in multi-tenant scenarios.
	// Returns the wrapped (encrypted) DEK suitable for storage.
	WrapDEK(ctx context.Context, projectID string, dek []byte) ([]byte, error)

	// UnwrapDEK decrypts a wrapped Data Encryption Key.
	// Returns the original DEK bytes.
	UnwrapDEK(ctx context.Context, projectID string, wrapped []byte) ([]byte, error)

	// ProviderType returns a string identifier for the provider type (e.g., "file", "vault", "awskms").
	ProviderType() string

	// HealthCheck verifies the provider is operational and can perform key operations.
	HealthCheck(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// KeyProviderConfig contains configuration for creating a KeyProvider.
type KeyProviderConfig struct {
	// Type selects the provider: "file", "vault", or "awskms".
	Type string `yaml:"type" json:"type"`
	// File contains configuration for file-based key storage (dev/legacy mode).
	File FileKeyProviderConfig `yaml:"file" json:"file"`
	// Vault contains configuration for HashiCorp Vault Transit secrets engine.
	Vault VaultKeyProviderConfig `yaml:"vault" json:"vault"`
	// AWSKMS contains configuration for AWS KMS.
	AWSKMS AWSKMSKeyProviderConfig `yaml:"awskms" json:"awskms"`
}

// ----- File-based KeyProvider (legacy/dev) -----

// FileKeyProviderConfig configures the file-based key provider.
type FileKeyProviderConfig struct {
	// KeystorePath is the directory containing the master.key file.
	// Defaults to ${WORKDIR}/.keystore if empty.
	KeystorePath string `yaml:"keystorePath" json:"keystorePath"`
}

// FileKeyProvider implements KeyProvider using a local file-based master key.
// This is the legacy behavior and suitable for development environments.
// NOT recommended for production where infrastructure admins should not have access to keys.
type FileKeyProvider struct {
	mu          sync.RWMutex
	masterKey   []byte
	keystoreDir string
}

// NewFileKeyProvider creates a new file-based key provider.
// If keystorePath is empty, it defaults to ${workdir}/.keystore.
func NewFileKeyProvider(workdir, keystorePath string) (*FileKeyProvider, error) {
	dir := keystorePath
	if dir == "" {
		dir = filepath.Join(workdir, ".keystore")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create keystore dir: %w", err)
	}

	keyPath := filepath.Join(dir, "master.key")
	mk, err := loadOrCreateMasterKeyFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load master key: %w", err)
	}

	return &FileKeyProvider{
		masterKey:   mk,
		keystoreDir: dir,
	}, nil
}

func loadOrCreateMasterKeyFile(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("invalid master.key length: %d", len(b))
		}
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func (p *FileKeyProvider) ProviderType() string { return "file" }

func (p *FileKeyProvider) WrapDEK(_ context.Context, _ string, dek []byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.masterKey) == 0 {
		return nil, errors.New("master key not initialized")
	}

	block, err := aes.NewCipher(p.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := crand.Read(nonce); err != nil {
		return nil, err
	}
	// Output format: nonce || ciphertext
	ct := gcm.Seal(nonce, nonce, dek, nil)
	return ct, nil
}

func (p *FileKeyProvider) UnwrapDEK(_ context.Context, _ string, wrapped []byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.masterKey) == 0 {
		return nil, errors.New("master key not initialized")
	}

	block, err := aes.NewCipher(p.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(wrapped) < nonceSize {
		return nil, errors.New("wrapped key too short")
	}
	nonce := wrapped[:nonceSize]
	ct := wrapped[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap failed: %w", err)
	}
	return pt, nil
}

func (p *FileKeyProvider) HealthCheck(_ context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.masterKey) != 32 {
		return errors.New("master key not properly initialized")
	}
	return nil
}

func (p *FileKeyProvider) Close() error { return nil }

// ----- HashiCorp Vault KeyProvider -----

// VaultKeyProviderConfig configures the Vault Transit secrets engine provider.
type VaultKeyProviderConfig struct {
	// Address is the Vault server URL (e.g., "https://vault.example.com:8200").
	Address string `yaml:"address" json:"address"`
	// Token is the Vault authentication token. Prefer VAULT_TOKEN env var.
	Token string `yaml:"token" json:"token"`
	// KeyName is the name of the transit key in Vault (e.g., "manifold-kek").
	KeyName string `yaml:"keyName" json:"keyName"`
	// MountPath is the mount path for the transit engine (default: "transit").
	MountPath string `yaml:"mountPath" json:"mountPath"`
	// Namespace is the Vault namespace for enterprise deployments.
	Namespace string `yaml:"namespace" json:"namespace"`
	// TLSSkipVerify disables TLS certificate verification (dev only).
	TLSSkipVerify bool `yaml:"tlsSkipVerify" json:"tlsSkipVerify"`
	// TimeoutSeconds is the HTTP request timeout (default: 30).
	TimeoutSeconds int `yaml:"timeoutSeconds" json:"timeoutSeconds"`
}

// VaultKeyProvider implements KeyProvider using HashiCorp Vault's Transit secrets engine.
// This provides enterprise-grade key management where KEKs are managed by Vault and
// never exposed to application servers.
type VaultKeyProvider struct {
	client    *http.Client
	address   string
	token     string
	keyName   string
	mountPath string
	namespace string
}

// NewVaultKeyProvider creates a new Vault Transit key provider.
func NewVaultKeyProvider(cfg VaultKeyProviderConfig) (*VaultKeyProvider, error) {
	if cfg.Address == "" {
		return nil, errors.New("vault address required")
	}
	token := cfg.Token
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	if token == "" {
		return nil, errors.New("vault token required (config or VAULT_TOKEN env)")
	}
	if cfg.KeyName == "" {
		return nil, errors.New("vault key name required")
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "transit"
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSSkipVerify {
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	return &VaultKeyProvider{
		client:    &http.Client{Timeout: timeout, Transport: transport},
		address:   strings.TrimSuffix(cfg.Address, "/"),
		token:     token,
		keyName:   cfg.KeyName,
		mountPath: mountPath,
		namespace: cfg.Namespace,
	}, nil
}

func (p *VaultKeyProvider) ProviderType() string { return "vault" }

func (p *VaultKeyProvider) WrapDEK(ctx context.Context, projectID string, dek []byte) ([]byte, error) {
	// Vault Transit encrypt endpoint: POST /v1/{mount}/encrypt/{key}
	url := fmt.Sprintf("%s/v1/%s/encrypt/%s", p.address, p.mountPath, p.keyName)

	// Base64 encode the plaintext DEK as Vault expects
	payload := map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(dek),
		// Include project context for audit logging
		"context": base64.StdEncoding.EncodeToString([]byte(projectID)),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault encrypt failed: %s - %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse vault response: %w", err)
	}

	// Return the Vault ciphertext as bytes (it's already a vault-prefixed string like "vault:v1:...")
	return []byte(result.Data.Ciphertext), nil
}

func (p *VaultKeyProvider) UnwrapDEK(ctx context.Context, projectID string, wrapped []byte) ([]byte, error) {
	// Vault Transit decrypt endpoint: POST /v1/{mount}/decrypt/{key}
	url := fmt.Sprintf("%s/v1/%s/decrypt/%s", p.address, p.mountPath, p.keyName)

	payload := map[string]interface{}{
		"ciphertext": string(wrapped),
		"context":    base64.StdEncoding.EncodeToString([]byte(projectID)),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault decrypt failed: %s - %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse vault response: %w", err)
	}

	dek, err := base64.StdEncoding.DecodeString(result.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("decode plaintext: %w", err)
	}
	return dek, nil
}

func (p *VaultKeyProvider) HealthCheck(ctx context.Context) error {
	// Check Vault seal status: GET /v1/sys/seal-status
	url := fmt.Sprintf("%s/v1/sys/seal-status", p.address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", p.token)
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault health check returned %s", resp.Status)
	}

	var status struct {
		Sealed bool `json:"sealed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("parse seal status: %w", err)
	}
	if status.Sealed {
		return errors.New("vault is sealed")
	}
	return nil
}

func (p *VaultKeyProvider) Close() error { return nil }

// ----- AWS KMS KeyProvider -----

// AWSKMSKeyProviderConfig configures the AWS KMS provider.
type AWSKMSKeyProviderConfig struct {
	// KeyID is the AWS KMS key ID or ARN (e.g., "arn:aws:kms:us-east-1:123456789:key/12345678-1234-1234-1234-123456789012").
	KeyID string `yaml:"keyID" json:"keyID"`
	// Region is the AWS region (e.g., "us-east-1").
	Region string `yaml:"region" json:"region"`
	// AccessKeyID is the AWS access key (prefer IAM roles or env vars).
	AccessKeyID string `yaml:"accessKeyID" json:"accessKeyID"`
	// SecretAccessKey is the AWS secret key (prefer IAM roles or env vars).
	SecretAccessKey string `yaml:"secretAccessKey" json:"secretAccessKey"`
	// Endpoint is an optional custom endpoint for KMS (e.g., LocalStack).
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

// AWSKMSKeyProvider implements KeyProvider using AWS Key Management Service.
// This provides enterprise-grade key management where KEKs are managed by AWS KMS.
//
// Note: This implementation uses the AWS SDK v2 HTTP API directly to avoid
// heavy SDK dependencies. For production use, consider using the official AWS SDK.
type AWSKMSKeyProvider struct {
	client    *http.Client
	keyID     string
	region    string
	accessKey string
	secretKey string
	endpoint  string
}

// NewAWSKMSKeyProvider creates a new AWS KMS key provider.
func NewAWSKMSKeyProvider(cfg AWSKMSKeyProviderConfig) (*AWSKMSKeyProvider, error) {
	if cfg.KeyID == "" {
		return nil, errors.New("KMS key ID required")
	}
	region := cfg.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
	}
	if region == "" {
		return nil, errors.New("AWS region required")
	}

	accessKey := cfg.AccessKeyID
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	secretKey := cfg.SecretAccessKey
	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}

	// If no explicit credentials, rely on IAM roles (EC2 instance profile, ECS task role, etc.)
	// The actual signing will fail if no credentials are available

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://kms.%s.amazonaws.com", region)
	}

	return &AWSKMSKeyProvider{
		client:    &http.Client{Timeout: 30 * time.Second},
		keyID:     cfg.KeyID,
		region:    region,
		accessKey: accessKey,
		secretKey: secretKey,
		endpoint:  endpoint,
	}, nil
}

func (p *AWSKMSKeyProvider) ProviderType() string { return "awskms" }

// Note: The following implementation is a simplified version.
// For production use with IAM roles, use the official AWS SDK v2.

func (p *AWSKMSKeyProvider) WrapDEK(ctx context.Context, projectID string, dek []byte) ([]byte, error) {
	// AWS KMS Encrypt operation
	payload := map[string]interface{}{
		"KeyId":     p.keyID,
		"Plaintext": base64.StdEncoding.EncodeToString(dek),
		"EncryptionContext": map[string]string{
			"project_id": projectID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService.Encrypt")

	// Sign request with AWS SigV4 (simplified - production should use SDK)
	if err := p.signRequest(req, body); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("KMS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("KMS encrypt failed: %s - %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		CiphertextBlob string `json:"CiphertextBlob"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse KMS response: %w", err)
	}

	ct, err := base64.StdEncoding.DecodeString(result.CiphertextBlob)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	return ct, nil
}

func (p *AWSKMSKeyProvider) UnwrapDEK(ctx context.Context, projectID string, wrapped []byte) ([]byte, error) {
	// AWS KMS Decrypt operation
	payload := map[string]interface{}{
		"KeyId":          p.keyID,
		"CiphertextBlob": base64.StdEncoding.EncodeToString(wrapped),
		"EncryptionContext": map[string]string{
			"project_id": projectID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService.Decrypt")

	if err := p.signRequest(req, body); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("KMS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("KMS decrypt failed: %s - %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		Plaintext string `json:"Plaintext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse KMS response: %w", err)
	}

	dek, err := base64.StdEncoding.DecodeString(result.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("decode plaintext: %w", err)
	}
	return dek, nil
}

func (p *AWSKMSKeyProvider) HealthCheck(ctx context.Context) error {
	// Describe key to verify access
	payload := map[string]interface{}{
		"KeyId": p.keyID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService.DescribeKey")

	if err := p.signRequest(req, body); err != nil {
		return fmt.Errorf("sign request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("KMS health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("KMS health check returned %s: %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		KeyMetadata struct {
			Enabled  bool   `json:"Enabled"`
			KeyState string `json:"KeyState"`
		} `json:"KeyMetadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse KMS response: %w", err)
	}
	if !result.KeyMetadata.Enabled || result.KeyMetadata.KeyState != "Enabled" {
		return fmt.Errorf("KMS key not enabled (state: %s)", result.KeyMetadata.KeyState)
	}
	return nil
}

func (p *AWSKMSKeyProvider) Close() error { return nil }

// signRequest adds AWS SigV4 signature headers to the request.
// This is a simplified implementation. For production, use the AWS SDK.
func (p *AWSKMSKeyProvider) signRequest(req *http.Request, _ []byte) error {
	if p.accessKey == "" || p.secretKey == "" {
		// Skip signing - rely on IAM role (EC2/ECS metadata service)
		// This won't work without proper credential chain setup
		return errors.New("AWS credentials required for KMS operations; configure access keys or use AWS SDK with IAM roles")
	}

	// For a proper implementation, use AWS SDK v2's signer package.
	// This placeholder shows where signing would occur.
	// The actual SigV4 implementation is complex and best left to the SDK.
	return errors.New("AWS SigV4 signing not implemented; use AWS SDK v2 for production")
}

// ----- Factory -----

// NewKeyProvider creates a KeyProvider based on the configuration.
func NewKeyProvider(workdir string, cfg KeyProviderConfig) (KeyProvider, error) {
	switch strings.ToLower(cfg.Type) {
	case "", "file":
		return NewFileKeyProvider(workdir, cfg.File.KeystorePath)
	case "vault":
		return NewVaultKeyProvider(cfg.Vault)
	case "awskms", "kms":
		return NewAWSKMSKeyProvider(cfg.AWSKMS)
	default:
		return nil, fmt.Errorf("unknown key provider type: %q", cfg.Type)
	}
}
