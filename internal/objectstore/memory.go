package objectstore

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore implements ObjectStore using an in-memory map.
// Useful for testing without S3 dependencies.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string]*memObject
}

type memObject struct {
	data        []byte
	attrs       ObjectAttrs
	contentType string
	metadata    map[string]string
}

// NewMemoryStore creates an in-memory ObjectStore for testing.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects: make(map[string]*memObject),
	}
}

// Get retrieves an object by key.
func (m *MemoryStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectAttrs, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, ok := m.objects[key]
	if !ok {
		return nil, ObjectAttrs{}, ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(obj.data)), obj.attrs, nil
}

// Put stores an object with the given key.
func (m *MemoryStore) Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	etag := "\"" + key + "-etag\""
	m.objects[key] = &memObject{
		data: data,
		attrs: ObjectAttrs{
			Key:          key,
			Size:         int64(len(data)),
			ETag:         etag,
			LastModified: time.Now().UTC(),
			ContentType:  opts.ContentType,
		},
		contentType: opts.ContentType,
		metadata:    opts.Metadata,
	}

	return etag, nil
}

// Delete removes an object by key.
func (m *MemoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.objects, key)
	return nil
}

// List returns objects matching the given options.
func (m *MemoryStore) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var objects []ObjectAttrs
	prefixSet := make(map[string]bool)

	for key, obj := range m.objects {
		if opts.Prefix != "" && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}

		// Handle delimiter for pseudo-directories
		if opts.Delimiter != "" {
			suffix := strings.TrimPrefix(key, opts.Prefix)
			if idx := strings.Index(suffix, opts.Delimiter); idx >= 0 {
				prefix := opts.Prefix + suffix[:idx+1]
				prefixSet[prefix] = true
				continue
			}
		}

		objects = append(objects, obj.attrs)
	}

	// Sort objects by key for consistent ordering
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	// Convert prefix set to slice
	var prefixes []string
	for p := range prefixSet {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	// Apply MaxKeys limit
	if opts.MaxKeys > 0 && len(objects) > opts.MaxKeys {
		return ListResult{
			Objects:               objects[:opts.MaxKeys],
			CommonPrefixes:        prefixes,
			IsTruncated:           true,
			NextContinuationToken: objects[opts.MaxKeys].Key,
		}, nil
	}

	return ListResult{
		Objects:        objects,
		CommonPrefixes: prefixes,
	}, nil
}

// Head returns object metadata without downloading content.
func (m *MemoryStore) Head(ctx context.Context, key string) (ObjectAttrs, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, ok := m.objects[key]
	if !ok {
		return ObjectAttrs{}, ErrNotFound
	}

	return obj.attrs, nil
}

// Copy duplicates an object to a new key.
func (m *MemoryStore) Copy(ctx context.Context, srcKey, dstKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	src, ok := m.objects[srcKey]
	if !ok {
		return ErrNotFound
	}

	// Copy the data
	data := make([]byte, len(src.data))
	copy(data, src.data)

	m.objects[dstKey] = &memObject{
		data: data,
		attrs: ObjectAttrs{
			Key:          dstKey,
			Size:         src.attrs.Size,
			ETag:         "\"" + dstKey + "-etag\"",
			LastModified: time.Now().UTC(),
			ContentType:  src.contentType,
		},
		contentType: src.contentType,
		metadata:    src.metadata,
	}

	return nil
}

// Exists checks if an object exists at the given key.
func (m *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.objects[key]
	return ok, nil
}

// Ping always succeeds for memory store.
func (m *MemoryStore) Ping(ctx context.Context) error {
	return nil
}

// Ensure MemoryStore implements ObjectStore.
var _ ObjectStore = (*MemoryStore)(nil)
