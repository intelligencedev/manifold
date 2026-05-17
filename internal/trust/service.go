package trust

import "context"

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Init(ctx context.Context) error {
	if s == nil || s.store == nil { return nil }
	return s.store.Init(ctx)
}

func (s *Service) List(ctx context.Context) ([]Budget, error) {
	if s == nil || s.store == nil { return nil, nil }
	return s.store.List(ctx)
}

func (s *Service) Spend(ctx context.Context, name string, delta int) (Budget, error) {
	return s.store.Spend(ctx, name, delta)
}

func (s *Service) Refill(ctx context.Context, name string, quota int) (Budget, error) {
	return s.store.Refill(ctx, name, quota)
}

func (s *Service) Get(ctx context.Context, name string) (Budget, bool, error) {
	return s.store.Get(ctx, name)
}
