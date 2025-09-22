package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStoreSchemaAndUser(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	st := NewStore(pool, 1)
	if err := st.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := st.EnsureDefaultRoles(ctx); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	u := &User{Email: "test@example.com", Name: "Test", Provider: "oidc", Subject: "sub123"}
	if _, err := st.UpsertUser(ctx, u); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := st.AddRole(ctx, u.ID, "user"); err != nil {
		t.Fatalf("add role: %v", err)
	}
	ok, err := st.HasRole(ctx, u.ID, "user")
	if err != nil || !ok {
		t.Fatalf("has role: %v ok=%v", err, ok)
	}
	sess, err := st.CreateSession(ctx, u.ID)
	if err != nil || sess == nil {
		t.Fatalf("session: %v", err)
	}
	if _, _, err := st.GetSession(ctx, sess.ID); err != nil {
		t.Fatalf("get session: %v", err)
	}
}
