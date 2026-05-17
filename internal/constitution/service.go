package constitution

import "context"

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }
func (s *Service) Init(ctx context.Context) error { if s == nil || s.store == nil { return nil }; return s.store.Init(ctx) }
func (s *Service) List(ctx context.Context) ([]Version, error) { return s.store.List(ctx) }
func (s *Service) Create(ctx context.Context, body string, createdBy int64) (Version, error) { return s.store.Create(ctx, body, createdBy) }
func (s *Service) Activate(ctx context.Context, id string) (Version, error) { return s.store.Activate(ctx, id) }
func (s *Service) GetActive(ctx context.Context) (Version, bool, error) { return s.store.GetActive(ctx) }
