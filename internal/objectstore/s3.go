package objectstore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"manifold/internal/config"
)

// S3Store implements ObjectStore using AWS SDK Go v2.
// It supports AWS S3 and S3-compatible services like MinIO.
type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
	sse    config.S3SSEConfig
}

// S3Option configures S3Store creation.
type S3Option func(*s3Options)

type s3Options struct {
	httpClient *http.Client
}

// WithHTTPClient sets a custom HTTP client for S3 requests.
func WithHTTPClient(c *http.Client) S3Option {
	return func(o *s3Options) {
		o.httpClient = c
	}
}

// NewS3Store creates an S3Store from configuration.
func NewS3Store(ctx context.Context, cfg config.S3Config, opts ...S3Option) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}

	o := &s3Options{}
	for _, opt := range opts {
		opt(o)
	}

	// Build AWS config options
	awsOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	// Use static credentials if provided
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		awsOpts = append(awsOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	// Configure custom HTTP client if needed (for TLS settings)
	if cfg.TLSInsecureSkipVerify || o.httpClient != nil {
		httpClient := o.httpClient
		if httpClient == nil {
			httpClient = &http.Client{}
		}
		if cfg.TLSInsecureSkipVerify {
			transport := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			httpClient = &http.Client{Transport: transport}
		}
		awsOpts = append(awsOpts, awsconfig.WithHTTPClient(httpClient))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	// Build S3 client options
	s3Opts := []func(*s3.Options){}

	// Custom endpoint for MinIO or other S3-compatible services
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	// Path-style addressing (required for MinIO)
	if cfg.UsePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.TrimSuffix(cfg.Prefix, "/"),
		sse:    cfg.SSE,
	}, nil
}

// fullKey prepends the configured prefix to a key.
func (s *S3Store) fullKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

// stripPrefix removes the configured prefix from a key.
func (s *S3Store) stripPrefix(key string) string {
	if s.prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, s.prefix+"/")
}

// Get retrieves an object by key.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, ObjectAttrs, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		if isNotFoundError(err) {
			return nil, ObjectAttrs{}, ErrNotFound
		}
		if isAccessDeniedError(err) {
			return nil, ObjectAttrs{}, ErrAccessDenied
		}
		return nil, ObjectAttrs{}, fmt.Errorf("s3 get: %w", err)
	}

	attrs := ObjectAttrs{
		Key:          key,
		Size:         aws.ToInt64(result.ContentLength),
		ETag:         aws.ToString(result.ETag),
		LastModified: aws.ToTime(result.LastModified),
		ContentType:  aws.ToString(result.ContentType),
	}

	return result.Body, attrs, nil
}

// Put stores an object with the given key.
func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (string, error) {
	// Read all content since S3 SDK requires content length or seekable body
	// For large files, consider using multipart upload
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
		Body:   strings.NewReader(string(data)),
	}

	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}

	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}

	// Apply server-side encryption
	switch s.sse.Mode {
	case "sse-s3":
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	case "sse-kms":
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		if s.sse.KMSKeyID != "" {
			input.SSEKMSKeyId = aws.String(s.sse.KMSKeyID)
		}
	}

	result, err := s.client.PutObject(ctx, input)
	if err != nil {
		if isAccessDeniedError(err) {
			return "", ErrAccessDenied
		}
		return "", fmt.Errorf("s3 put: %w", err)
	}

	return aws.ToString(result.ETag), nil
}

// Delete removes an object by key.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	}

	_, err := s.client.DeleteObject(ctx, input)
	if err != nil {
		if isNotFoundError(err) {
			return nil // DeleteObject is idempotent
		}
		if isAccessDeniedError(err) {
			return ErrAccessDenied
		}
		return fmt.Errorf("s3 delete: %w", err)
	}

	return nil
}

// List returns objects matching the given options.
func (s *S3Store) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	prefix := opts.Prefix
	if s.prefix != "" {
		if prefix != "" {
			prefix = s.prefix + "/" + prefix
		} else {
			prefix = s.prefix + "/"
		}
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	}

	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if opts.Delimiter != "" {
		input.Delimiter = aws.String(opts.Delimiter)
	}
	if opts.MaxKeys > 0 {
		input.MaxKeys = aws.Int32(int32(opts.MaxKeys))
	}
	if opts.ContinuationToken != "" {
		input.ContinuationToken = aws.String(opts.ContinuationToken)
	}

	result, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		if isAccessDeniedError(err) {
			return ListResult{}, ErrAccessDenied
		}
		return ListResult{}, fmt.Errorf("s3 list: %w", err)
	}

	objects := make([]ObjectAttrs, 0, len(result.Contents))
	for _, obj := range result.Contents {
		key := s.stripPrefix(aws.ToString(obj.Key))
		objects = append(objects, ObjectAttrs{
			Key:          key,
			Size:         aws.ToInt64(obj.Size),
			ETag:         aws.ToString(obj.ETag),
			LastModified: aws.ToTime(obj.LastModified),
		})
	}

	prefixes := make([]string, 0, len(result.CommonPrefixes))
	for _, p := range result.CommonPrefixes {
		prefixes = append(prefixes, s.stripPrefix(aws.ToString(p.Prefix)))
	}

	return ListResult{
		Objects:               objects,
		CommonPrefixes:        prefixes,
		IsTruncated:           aws.ToBool(result.IsTruncated),
		NextContinuationToken: aws.ToString(result.NextContinuationToken),
	}, nil
}

// Head returns object metadata without downloading content.
func (s *S3Store) Head(ctx context.Context, key string) (ObjectAttrs, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	}

	result, err := s.client.HeadObject(ctx, input)
	if err != nil {
		if isNotFoundError(err) {
			return ObjectAttrs{}, ErrNotFound
		}
		if isAccessDeniedError(err) {
			return ObjectAttrs{}, ErrAccessDenied
		}
		return ObjectAttrs{}, fmt.Errorf("s3 head: %w", err)
	}

	return ObjectAttrs{
		Key:          key,
		Size:         aws.ToInt64(result.ContentLength),
		ETag:         aws.ToString(result.ETag),
		LastModified: aws.ToTime(result.LastModified),
		ContentType:  aws.ToString(result.ContentType),
	}, nil
}

// Copy duplicates an object to a new key.
func (s *S3Store) Copy(ctx context.Context, srcKey, dstKey string) error {
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(path.Join(s.bucket, s.fullKey(srcKey))),
		Key:        aws.String(s.fullKey(dstKey)),
	}

	// Apply server-side encryption to the copy
	switch s.sse.Mode {
	case "sse-s3":
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	case "sse-kms":
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		if s.sse.KMSKeyID != "" {
			input.SSEKMSKeyId = aws.String(s.sse.KMSKeyID)
		}
	}

	_, err := s.client.CopyObject(ctx, input)
	if err != nil {
		if isNotFoundError(err) {
			return ErrNotFound
		}
		if isAccessDeniedError(err) {
			return ErrAccessDenied
		}
		return fmt.Errorf("s3 copy: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the given key.
func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Head(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Ping verifies connectivity to the S3 bucket.
func (s *S3Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		if isNotFoundError(err) {
			return ErrBucketMissing
		}
		if isAccessDeniedError(err) {
			return ErrAccessDenied
		}
		return fmt.Errorf("s3 ping: %w", err)
	}
	return nil
}

// isNotFoundError checks if the error indicates a missing object.
func isNotFoundError(err error) bool {
	var notFound *s3types.NotFound
	var noSuchKey *s3types.NoSuchKey
	var noSuchBucket *s3types.NoSuchBucket
	return errors.As(err, &notFound) ||
		errors.As(err, &noSuchKey) ||
		errors.As(err, &noSuchBucket) ||
		strings.Contains(err.Error(), "NotFound") ||
		strings.Contains(err.Error(), "NoSuchKey")
}

// isAccessDeniedError checks if the error indicates permission issues.
func isAccessDeniedError(err error) bool {
	return strings.Contains(err.Error(), "AccessDenied") ||
		strings.Contains(err.Error(), "Forbidden")
}
