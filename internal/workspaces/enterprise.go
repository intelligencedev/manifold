package workspaces

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/objectstore"
	"manifold/internal/projects"
)

// SkillsInvalidationFunc is called when skills need to be invalidated for a project.
// This callback pattern avoids import cycles between workspaces and skills packages.
type SkillsInvalidationFunc func(projectID string)

// globalSkillsInvalidator is set by the skills package during initialization.
var globalSkillsInvalidator SkillsInvalidationFunc

// EnterpriseWorkspaceManager wraps the ephemeral manager with coordination primitives
// for generation caching and commit event publishing. It intentionally reuses the
// ephemeral manager's behavior for hydration/commit while introducing hooks for
// Redis/Kafka.
type EnterpriseWorkspaceManager struct {
	ephem     *EphemeralWorkspaceManager
	genCache  *RedisGenerationCache
	publisher *KafkaCommitPublisher
	mode      string
	cacheDir  string
	tmpfsDir  string
	cache     *EncryptedCacheManager
	material  *TmpfsMaterializer
	subMu     sync.Mutex
	subs      map[string]context.CancelFunc
}

// SetSkillsInvalidator registers a callback for skills cache invalidation.
// This should be called during application initialization.
func SetSkillsInvalidator(fn SkillsInvalidationFunc) {
	globalSkillsInvalidator = fn
}

// NewEnterpriseManager constructs an enterprise workspace manager. When store is nil,
// it falls back to legacy behavior.
func NewEnterpriseManager(cfg *config.Config, store objectstore.ObjectStore) WorkspaceManager {
	if store == nil {
		return &LegacyWorkspaceManager{workdir: cfg.Workdir, mode: "legacy"}
	}

	ephem := NewEphemeralManager(store, cfg)

	genCache, err := NewRedisGenerationCache(cfg.Projects.Redis)
	if err != nil {
		log.Warn().Err(err).Msg("redis_generation_cache_disabled")
	}

	publisher, err := NewKafkaCommitPublisher(cfg.Projects.Events)
	if err != nil {
		log.Warn().Err(err).Msg("kafka_publisher_disabled")
	}

	cacheDir := cfg.Projects.Workspace.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(cfg.Workdir, "cache")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.Warn().Err(err).Str("cacheDir", cacheDir).Msg("enterprise_cache_dir_create_failed")
	}

	tmpfsDir := cfg.Projects.Workspace.TmpfsDir
	if tmpfsDir == "" {
		tmpfsDir = filepath.Join(cfg.Workdir, "tmpfs")
	}
	if err := os.MkdirAll(tmpfsDir, 0o755); err != nil {
		log.Warn().Err(err).Str("tmpfsDir", tmpfsDir).Msg("enterprise_tmpfs_dir_create_failed")
	}

	var cacheMgr *EncryptedCacheManager
	var materializer *TmpfsMaterializer
	if cfg.Projects.Encrypt {
		kp, err := projects.NewKeyProvider(cfg.Workdir, projects.KeyProviderConfig{
			Type: cfg.Projects.Encryption.Provider,
			File: projects.FileKeyProviderConfig{KeystorePath: cfg.Projects.Encryption.File.KeystorePath},
			Vault: projects.VaultKeyProviderConfig{
				Address:        cfg.Projects.Encryption.Vault.Address,
				Token:          cfg.Projects.Encryption.Vault.Token,
				KeyName:        cfg.Projects.Encryption.Vault.KeyName,
				MountPath:      cfg.Projects.Encryption.Vault.MountPath,
				Namespace:      cfg.Projects.Encryption.Vault.Namespace,
				TLSSkipVerify:  cfg.Projects.Encryption.Vault.TLSSkipVerify,
				TimeoutSeconds: cfg.Projects.Encryption.Vault.TimeoutSeconds,
			},
			AWSKMS: projects.AWSKMSKeyProviderConfig{
				KeyID:           cfg.Projects.Encryption.AWSKMS.KeyID,
				Region:          cfg.Projects.Encryption.AWSKMS.Region,
				AccessKeyID:     cfg.Projects.Encryption.AWSKMS.AccessKeyID,
				SecretAccessKey: cfg.Projects.Encryption.AWSKMS.SecretAccessKey,
				Endpoint:        cfg.Projects.Encryption.AWSKMS.Endpoint,
			},
		})
		if err != nil {
			log.Warn().Err(err).Msg("enterprise_key_provider_init_failed")
		} else {
			cacheMgr, err = NewEncryptedCacheManager(cacheDir, kp)
			if err != nil {
				log.Warn().Err(err).Msg("enterprise_cache_manager_init_failed")
			}
			materializer, err = NewTmpfsMaterializer(tmpfsDir, cacheMgr)
			if err != nil {
				log.Warn().Err(err).Msg("enterprise_materializer_init_failed")
			}
		}
	}

	return &EnterpriseWorkspaceManager{
		ephem:     ephem,
		genCache:  genCache,
		publisher: publisher,
		mode:      "enterprise",
		cacheDir:  cacheDir,
		tmpfsDir:  tmpfsDir,
		cache:     cacheMgr,
		material:  materializer,
		subs:      make(map[string]context.CancelFunc),
	}
}

// Mode returns "enterprise".
func (m *EnterpriseWorkspaceManager) Mode() string { return m.mode }

// SetDecrypter forwards the decrypter configuration to the underlying ephemeral manager.
func (m *EnterpriseWorkspaceManager) SetDecrypter(d FileDecrypter) {
	if m == nil || m.ephem == nil {
		return
	}
	m.ephem.SetDecrypter(d)
}

// Checkout reuses the ephemeral manager; primes Redis generation cache when available.
func (m *EnterpriseWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error) {
	ws := Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, Mode: m.mode}

	if projectID == "" {
		return ws, nil
	}

	cleanPID, err := ValidateProjectID(projectID)
	if err != nil {
		return Workspace{}, err
	}
	ws.ProjectID = cleanPID

	if sessionID == "" {
		sessionID = fmt.Sprintf("ses-%d", time.Now().UnixNano())
	}

	cleanSID, err := ValidateSessionID(sessionID)
	if err != nil {
		return Workspace{}, err
	}
	ws.SessionID = cleanSID

	tenant := fmt.Sprint(userID)

	var remoteGen, remoteSkills int64
	if m.genCache != nil {
		remoteGen, _ = m.genCache.GetGeneration(ctx, tenant, cleanPID)
		remoteSkills, _ = m.genCache.GetSkillsGeneration(ctx, tenant, cleanPID)
	}
	if remoteGen == 0 && remoteSkills == 0 {
		if meta, err := m.ephem.fetchProjectMeta(ctx, userID, cleanPID); err == nil {
			remoteGen = meta.Generation
			remoteSkills = meta.SkillsGeneration
		}
	}

	// If encrypted cache/materializer are configured, perform cache-backed materialization.
	if m.cache != nil && m.material != nil {
		wsPath, manifest, err := m.material.Materialize(ctx, tenant, cleanPID, cleanSID)
		if err != nil {
			log.Warn().Err(err).Msg("enterprise_materialize_failed")
		} else if manifest != nil {
			fresh := true
			if (remoteGen > 0 && manifest.Generation < remoteGen) || (remoteSkills > 0 && manifest.SkillsGeneration < remoteSkills) {
				fresh = false
			}
			if !fresh {
				if refreshed, errRefresh := m.refreshCache(ctx, userID, cleanPID); errRefresh != nil {
					log.Warn().Err(errRefresh).Str("projectID", cleanPID).Msg("enterprise_cache_refresh_failed")
				} else {
					manifest = refreshed
					wsPath, manifest, err = m.material.Materialize(ctx, tenant, cleanPID, cleanSID)
					if err != nil {
						log.Warn().Err(err).Msg("enterprise_materialize_after_refresh_failed")
					}
				}
			}

			if err == nil && manifest != nil {
				ws.BaseDir = wsPath
				m.attachSessionFromCache(ws, manifest)
				if m.genCache != nil {
					_ = m.genCache.SetGenerations(ctx, tenant, cleanPID, manifest.Generation, manifest.SkillsGeneration)
				}
				m.ensureInvalidationSubscription(ctx, tenant, cleanPID)
				// Notify MCP pool of workspace checkout
				notifyCheckout(ctx, ws)
				return ws, nil
			}
		}
	}

	// Fast-path reuse using Redis generation cache, with S3 verification to catch out-of-band changes.
	if m.genCache != nil && cleanPID != "" {
		gen, errGen := m.genCache.GetGeneration(ctx, tenant, cleanPID)
		skillsGen, errSkills := m.genCache.GetSkillsGeneration(ctx, tenant, cleanPID)
		if errGen == nil && errSkills == nil {
			if state, ok := m.ephem.activeState(userID, cleanPID, cleanSID); ok && state.manifest != nil {
				if state.manifest.Generation >= gen && state.manifest.SkillsGeneration >= skillsGen {
					if meta, err := m.ephem.fetchProjectMeta(ctx, userID, cleanPID); err == nil {
						gen = meta.Generation
						skillsGen = meta.SkillsGeneration
						_ = m.genCache.SetGenerations(ctx, tenant, cleanPID, gen, skillsGen)
					}
				}
				if state.manifest.Generation >= gen && state.manifest.SkillsGeneration >= skillsGen {
					return state.ws, nil
				}
			}
		}
	}

	ws, err = m.ephem.Checkout(ctx, userID, cleanPID, cleanSID)
	if err != nil {
		return ws, err
	}
	if m.genCache != nil && cleanPID != "" {
		if meta, err := m.ephem.fetchProjectMeta(ctx, userID, cleanPID); err == nil {
			_ = m.genCache.SetGenerations(ctx, tenant, cleanPID, meta.Generation, meta.SkillsGeneration)
			m.ensureInvalidationSubscription(ctx, tenant, cleanPID)
		}
	}
	// Notify MCP pool of workspace checkout
	notifyCheckout(ctx, ws)
	return ws, nil
}

// ensureInvalidationSubscription subscribes to Redis invalidations for the project and
// invalidates the skills cache when events arrive.
func (m *EnterpriseWorkspaceManager) ensureInvalidationSubscription(ctx context.Context, tenantID, projectID string) {
	if m.genCache == nil || projectID == "" {
		return
	}
	key := tenantID + ":" + projectID
	m.subMu.Lock()
	if _, exists := m.subs[key]; exists {
		m.subMu.Unlock()
		return
	}
	ch, cancel := m.genCache.SubscribeInvalidations(ctx, tenantID, projectID)
	m.subs[key] = cancel
	m.subMu.Unlock()

	go func() {
		for ev := range ch {
			if ev.SkillsGeneration > 0 || ev.Generation > 0 {
				if globalSkillsInvalidator != nil {
					globalSkillsInvalidator(projectID)
				}
			}
		}
	}()
}

// Commit delegates to the ephemeral manager, then updates generation cache and publishes events.
func (m *EnterpriseWorkspaceManager) Commit(ctx context.Context, ws Workspace) error {
	if m.cache != nil && m.material != nil {
		tenant := fmt.Sprint(ws.UserID)
		changed := m.ephem.LastChangedPaths(ws)
		if len(changed) > 0 {
			manifest, _ := m.cache.LoadManifest(ctx, tenant, ws.ProjectID)
			manifest, err := m.material.SyncBack(ctx, tenant, ws.ProjectID, ws.SessionID, manifest, changed)
			if err != nil {
				log.Warn().Err(err).Msg("enterprise_syncback_failed")
			} else {
				_ = m.cache.SaveManifest(ctx, tenant, ws.ProjectID, manifest)
			}
		}
	}

	if err := m.ephem.Commit(ctx, ws); err != nil {
		return err
	}

	if ws.ProjectID == "" {
		return nil
	}

	meta, err := m.ephem.fetchProjectMeta(ctx, ws.UserID, ws.ProjectID)
	if err != nil {
		log.Warn().Err(err).Msg("enterprise_fetch_meta_failed")
		return nil
	}

	changedPaths := m.ephem.LastChangedPaths(ws)

	tenant := fmt.Sprint(ws.UserID)
	if m.genCache != nil {
		if err := m.genCache.SetGenerations(ctx, tenant, ws.ProjectID, meta.Generation, meta.SkillsGeneration); err != nil {
			log.Warn().Err(err).Msg("redis_set_generation_failed")
		}
		ev := InvalidationEvent{Generation: meta.Generation, SkillsGeneration: meta.SkillsGeneration, ChangedPaths: changedPaths}
		if err := m.genCache.PublishInvalidation(ctx, tenant, ws.ProjectID, ev); err != nil {
			log.Warn().Err(err).Msg("redis_publish_invalidation_failed")
		}
		// Ensure we listen for future invalidations for this project
		m.ensureInvalidationSubscription(ctx, tenant, ws.ProjectID)
	}

	if m.publisher != nil {
		ev := ProjectCommitEvent{
			TenantID:         tenant,
			ProjectID:        ws.ProjectID,
			UserID:           ws.UserID,
			SessionID:        ws.SessionID,
			Generation:       meta.Generation,
			SkillsGeneration: meta.SkillsGeneration,
			ChangedPaths:     changedPaths,
			Timestamp:        time.Now().UTC(),
			CommitID:         uuid.NewString(),
		}
		if err := m.publisher.Publish(ctx, ev); err != nil {
			log.Warn().Err(err).Msg("kafka_publish_failed")
		}
	}

	return nil
}

// Cleanup delegates to the ephemeral manager.
func (m *EnterpriseWorkspaceManager) Cleanup(ctx context.Context, ws Workspace) error {
	if m.material != nil && ws.Mode == m.mode {
		tenant := fmt.Sprint(ws.UserID)
		if err := m.material.Unmaterialize(ctx, tenant, ws.ProjectID, ws.SessionID); err != nil {
			log.Warn().Err(err).Msg("enterprise_unmaterialize_failed")
		}
		m.dropSessionState(ws)
		return nil
	}
	return m.ephem.Cleanup(ctx, ws)
}

func (m *EnterpriseWorkspaceManager) refreshCache(ctx context.Context, userID int64, projectID string) (*SyncManifest, error) {
	if m.cache == nil {
		return nil, fmt.Errorf("cache not configured")
	}

	refreshRoot := filepath.Join(m.ephem.workdir, "cache-refresh")
	if err := os.MkdirAll(refreshRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create refresh root: %w", err)
	}

	tmpPath := filepath.Join(refreshRoot, fmt.Sprintf("%d-%s-%s", userID, projectID, uuid.NewString()))
	if err := os.MkdirAll(tmpPath, 0o755); err != nil {
		return nil, fmt.Errorf("create refresh workspace: %w", err)
	}
	defer os.RemoveAll(tmpPath)

	meta, err := m.ephem.fetchProjectMeta(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	manifest, err := m.ephem.hydrate(ctx, userID, projectID, tmpPath, meta)
	if err != nil {
		return nil, err
	}

	if err := m.cache.StoreWorkspace(ctx, fmt.Sprint(userID), projectID, tmpPath, manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

func (m *EnterpriseWorkspaceManager) attachSessionFromCache(ws Workspace, manifest *SyncManifest) {
	if manifest == nil {
		return
	}
	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)
	m.ephem.mu.Lock()
	m.ephem.active[key] = &workspaceState{
		ws:               ws,
		manifest:         manifest,
		generation:       manifest.Generation,
		skillsGeneration: manifest.SkillsGeneration,
	}
	m.ephem.mu.Unlock()
}

func (m *EnterpriseWorkspaceManager) dropSessionState(ws Workspace) {
	key := sessionKey(ws.UserID, ws.ProjectID, ws.SessionID)
	m.ephem.mu.Lock()
	delete(m.ephem.active, key)
	m.ephem.mu.Unlock()
}
