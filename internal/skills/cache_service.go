package skills

// CacheService initializes the process-local skills cache used at runtime.
type CacheService struct{}

type CacheServiceConfig struct {
}

// NewCacheService creates a skills cache service.
func NewCacheService(cfg CacheServiceConfig) (*CacheService, error) {
	return &CacheService{}, nil
}

// InitCacheService initializes the global cache service during startup.
func InitCacheService(cfg CacheServiceConfig) error {
	_, err := NewCacheService(cfg)
	if err != nil {
		return err
	}
	return nil
}
