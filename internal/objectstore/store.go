// Package objectstore provides an abstraction layer for object storage backends.
// It keeps a narrow interface suitable for project file storage.
package objectstore

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common errors returned by ObjectStore implementations.
var (
	ErrNotFound      = errors.New("object not found")
	ErrAccessDenied  = errors.New("access denied")
	ErrInvalidKey    = errors.New("invalid object key")
	ErrBucketMissing = errors.New("bucket does not exist")
)

// ObjectAttrs contains metadata about a stored object.
type ObjectAttrs struct {
	// Key is the full object key (path) in the bucket.
	Key string
	// Size is the object size in bytes.
	Size int64
	// ETag is the object's entity tag (typically an MD5 hash).
	ETag string
	// LastModified is when the object was last updated.
	LastModified time.Time
	// ContentType is the MIME type if set.
	ContentType string
	// IsPrefix indicates this is a "directory" prefix, not a real object.
	IsPrefix bool
}

// ListOptions configures List operation behavior.
type ListOptions struct {
	// Prefix filters objects to those starting with this string.
	Prefix string
	// Delimiter groups keys by this character (typically "/").
	// When set, common prefixes are returned as pseudo-directories.
	Delimiter string
	// MaxKeys limits the number of objects returned per call.
	MaxKeys int
	// ContinuationToken resumes listing from a previous truncated response.
	ContinuationToken string
}

// ListResult contains the result of a List operation.
type ListResult struct {
	// Objects contains the object metadata.
	Objects []ObjectAttrs
	// CommonPrefixes contains "directory" prefixes when Delimiter is set.
	CommonPrefixes []string
	// IsTruncated indicates more results are available.
	IsTruncated bool
	// NextContinuationToken is used to continue a truncated listing.
	NextContinuationToken string
}

// PutOptions configures Put operation behavior.
type PutOptions struct {
	// ContentType sets the MIME type of the object.
	ContentType string
	// Metadata contains custom key-value pairs to store with the object.
	Metadata map[string]string
}

// ObjectStore defines the interface for object storage operations.
// Implementations must be safe for concurrent use.
type ObjectStore interface {
	// Get retrieves an object by key. The caller must close the returned reader.
	// Returns ErrNotFound if the object does not exist.
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectAttrs, error)

	// Put stores an object with the given key. The reader is fully consumed.
	// Returns the ETag of the stored object.
	Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (etag string, err error)

	// Delete removes an object by key. Does not error if the object doesn't exist.
	Delete(ctx context.Context, key string) error

	// List returns objects matching the given options.
	List(ctx context.Context, opts ListOptions) (ListResult, error)

	// Head returns object metadata without downloading the content.
	// Returns ErrNotFound if the object does not exist.
	Head(ctx context.Context, key string) (ObjectAttrs, error)

	// Copy duplicates an object to a new key within the same bucket.
	Copy(ctx context.Context, srcKey, dstKey string) error

	// Exists checks if an object exists at the given key.
	Exists(ctx context.Context, key string) (bool, error)
}

// BucketClient returns a reference to a specific bucket.
type BucketClient interface {
	Bucket(name string) ObjectStore
}
