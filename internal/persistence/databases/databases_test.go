package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"
	sqlitep "manifold/internal/persistence/sqlite"
)

func TestMemorySearch_IndexAndSearch(t *testing.T) {
	t.Parallel()
	s := NewMemorySearch()
	ctx := context.Background()
	_ = s.Index(ctx, "1", "The quick brown fox jumps over the lazy dog", map[string]string{"type": "doc"})
	_ = s.Index(ctx, "2", "Foxes are swift and quick", nil)
	_ = s.Index(ctx, "3", "Completely unrelated text", nil)
	hits, err := s.Search(ctx, "quick fox", 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit")
	}
	if hits[0].ID != "1" && hits[0].ID != "2" {
		t.Fatalf("unexpected top hit: %#v", hits[0])
	}
}

func TestMemoryVector_UpsertAndQuery(t *testing.T) {
	t.Parallel()
	v := NewMemoryVector()
	ctx := context.Background()
	// 2D vectors for simplicity
	_ = v.Upsert(ctx, "a", []float32{1, 0}, map[string]string{"label": "A"})
	_ = v.Upsert(ctx, "b", []float32{0, 1}, nil)
	_ = v.Upsert(ctx, "c", []float32{1, 1}, nil)
	q := []float32{0.9, 0.1}
	res, err := v.SimilaritySearch(ctx, q, 2, nil)
	if err != nil {
		t.Fatalf("sim search error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].ID != "a" {
		t.Fatalf("expected 'a' to be nearest, got %q", res[0].ID)
	}
}

func TestMemoryGraph_Basics(t *testing.T) {
	t.Parallel()
	g := NewMemoryGraph()
	ctx := context.Background()
	_ = g.UpsertNode(ctx, "n1", []string{"User"}, map[string]any{"name": "Alice"})
	_ = g.UpsertNode(ctx, "n2", []string{"User"}, map[string]any{"name": "Bob"})
	_ = g.UpsertEdge(ctx, "n1", "KNOWS", "n2", map[string]any{"since": 2020})
	neigh, err := g.Neighbors(ctx, "n1", "KNOWS")
	if err != nil {
		t.Fatalf("neighbors error: %v", err)
	}
	if len(neigh) != 1 || neigh[0] != "n2" {
		t.Fatalf("unexpected neighbors: %#v", neigh)
	}
	if n, ok := g.GetNode(ctx, "n1"); !ok || n.Props["name"] != "Alice" {
		t.Fatalf("unexpected node: %#v exists=%v", n, ok)
	}
}

func TestMemoryGraph_TypedEdges(t *testing.T) {
	t.Parallel()
	g := NewMemoryGraph()
	ctx := context.Background()
	if err := TypedUpsertEdge(ctx, g, TypedEdgeInput{
		Source:    "event:1",
		GraphType: "semantic",
		Rel:       "SIMILAR_TO",
		Target:    "event:2",
		Props:     map[string]any{"weight": 0.9},
	}); err != nil {
		t.Fatalf("TypedUpsertEdge() error = %v", err)
	}
	got, err := TypedNeighbors(ctx, g, "event:1", "semantic", "SIMILAR_TO")
	if err != nil {
		t.Fatalf("TypedNeighbors() error = %v", err)
	}
	if len(got) != 1 || got[0] != "event:2" {
		t.Fatalf("unexpected typed neighbors: %#v", got)
	}
}

func TestSQLiteSearch_IndexChunksAndSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestSQLite(t)
	search := NewSQLiteSearch(db)

	if err := search.Index(ctx, "doc:1", "The quick brown fox jumps over the lazy dog", map[string]string{"type": "doc"}); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	hits, err := search.Search(ctx, "quick fox", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "doc:1" {
		t.Fatalf("unexpected hits: %#v", hits)
	}
	if err := search.(*sqliteSearch).UpsertChunk(ctx, ChunkSearchRow{ID: "chunk:1", DocID: "doc:1", Index: 0, Text: "fast fox chunk", Metadata: map[string]string{"type": "chunk", "tenant": "t1"}, Lang: "english"}); err != nil {
		t.Fatalf("UpsertChunk() error = %v", err)
	}
	chunks, err := search.(*sqliteSearch).SearchChunks(ctx, "fox", "english", 5, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("SearchChunks() error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != "chunk:1" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestSQLiteSearch_SearchChunksUsesLanguageTolerantChunkFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	search := NewSQLiteSearch(openTestSQLite(t)).(*sqliteSearch)

	if err := search.UpsertChunk(ctx, ChunkSearchRow{
		ID:       "chunk:lang:1",
		DocID:    "doc:lang",
		Index:    0,
		Text:     "fast fox chunk",
		Metadata: map[string]string{"type": "chunk", "tenant": "t1"},
		Lang:     "english",
	}); err != nil {
		t.Fatalf("UpsertChunk() error = %v", err)
	}
	chunks, err := search.SearchChunks(ctx, "fox", "english", 5, map[string]string{"tenant": "t1", "lang": "english"})
	if err != nil {
		t.Fatalf("SearchChunks() error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != "chunk:lang:1" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if chunks[0].Metadata["lang"] != "english" {
		t.Fatalf("expected chunk metadata to include lang, got %#v", chunks[0].Metadata)
	}
}

func TestSQLiteSearch_SearchChunksFallsBackToDocumentsAndNormalizesQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	search := NewSQLiteSearch(openTestSQLite(t)).(*sqliteSearch)

	if err := search.Index(ctx, "doc:web:1", "Rare fetch keyword appears in this fetched page", map[string]string{"source": "web_fetch"}); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	hits, err := search.SearchChunks(ctx, "rare_fetch-key", "english", 5, map[string]string{"lang": "english"})
	if err != nil {
		t.Fatalf("SearchChunks() error = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "doc:web:1" {
		t.Fatalf("unexpected document fallback hits: %#v", hits)
	}
}

func TestSQLiteSearch_SearchRetriesWithPrefixNormalizedTerms(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	search := NewSQLiteSearch(openTestSQLite(t))

	if err := search.Index(ctx, "doc:prefix:1", "rarefetchkeyword keyphrase appears in this document", nil); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	hits, err := search.Search(ctx, "rarefetch-key", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "doc:prefix:1" {
		t.Fatalf("unexpected prefix hits: %#v", hits)
	}
}

func TestSQLiteVector_UpsertFilterAndQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestSQLite(t)
	store, err := NewSQLiteVector(db, config.VectorConfig{Dimensions: 2, Metric: "cosine"}, config.SQLiteVectorConfig{NProbe: 0.08})
	if err != nil {
		t.Fatalf("NewSQLiteVector() error = %v", err)
	}
	if err := store.Upsert(ctx, "a", []float32{1, 0}, map[string]string{"tenant": "t1", "type": "chunk"}); err != nil {
		t.Fatalf("Upsert(a) error = %v", err)
	}
	if err := store.Upsert(ctx, "b", []float32{0, 1}, map[string]string{"tenant": "t2", "type": "chunk"}); err != nil {
		t.Fatalf("Upsert(b) error = %v", err)
	}
	results, err := store.SimilaritySearch(ctx, []float32{0.9, 0.1}, 3, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("SimilaritySearch() error = %v", err)
	}
	if len(results) != 1 || results[0].ID != "a" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSQLiteVector_ANNRebuildState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestSQLite(t)
	store, err := NewSQLiteVector(db, config.VectorConfig{Dimensions: 2, Metric: "cosine"}, config.SQLiteVectorConfig{
		ANNEnabled:        true,
		ANNMinRows:        8,
		ANNRebuildChanges: 1,
		NProbe:            0.08,
	})
	if err != nil {
		t.Fatalf("NewSQLiteVector() error = %v", err)
	}
	vectorStore := store.(*sqliteVector)
	vectors := map[string][]float32{
		"a": {1, 0},
		"b": {0.9, 0.1},
		"c": {0.8, 0.2},
		"d": {0.7, 0.3},
		"e": {0.3, 0.7},
		"f": {0.2, 0.8},
		"g": {0.1, 0.9},
		"h": {0, 1},
	}
	for id, vector := range vectors {
		if err := store.Upsert(ctx, id, vector, map[string]string{"tenant": "t1"}); err != nil {
			t.Fatalf("Upsert(%s) error = %v", id, err)
		}
	}
	if err := vectorStore.rebuildANNIfNeeded(ctx); err != nil {
		t.Fatalf("rebuildANNIfNeeded: %v", err)
	}
	mode, err := vectorStore.vectorIndexMode(ctx)
	if err != nil {
		t.Fatalf("vectorIndexMode final: %v", err)
	}
	if mode != "ann" {
		t.Fatalf("expected ann index mode, got %q", mode)
	}
	results, err := store.SimilaritySearch(ctx, []float32{1, 0}, 2, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("SimilaritySearch() error = %v", err)
	}
	if len(results) == 0 || results[0].ID != "a" {
		t.Fatalf("unexpected results after ann rebuild: %#v", results)
	}
}

func TestSQLiteGraph_TypedMagmaMaintenance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	graph := NewSQLiteGraph(openTestSQLite(t))
	if err := graph.UpsertNode(ctx, "event:1", []string{"MagmaEvent"}, map[string]any{"tenant": "t1", "session": "s1", "text": "hello", "graphs": `["semantic"]`}); err != nil {
		t.Fatalf("UpsertNode() error = %v", err)
	}
	if err := TypedUpsertEdge(ctx, graph, TypedEdgeInput{Source: "event:1", GraphType: "semantic", Rel: "SIMILAR_TO", Target: "event:2", Props: map[string]any{"weight": 0.8}}); err != nil {
		t.Fatalf("TypedUpsertEdge() error = %v", err)
	}
	maintenance, ok := graph.(MagmaGraphMaintenanceDB)
	if !ok {
		t.Fatal("sqlite graph does not implement MagmaGraphMaintenanceDB")
	}
	events, err := maintenance.ListMagmaEvents(ctx)
	if err != nil {
		t.Fatalf("ListMagmaEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].ID != "event:1" {
		t.Fatalf("unexpected events: %#v", events)
	}
	edges, err := maintenance.ListMagmaEdges(ctx)
	if err != nil {
		t.Fatalf("ListMagmaEdges() error = %v", err)
	}
	if len(edges) != 1 || edges[0].Weight != 0.8 {
		t.Fatalf("unexpected edges: %#v", edges)
	}
}

func TestPostgresGraphJSONProp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{
			name:  "missing uses fallback",
			props: map[string]any{},
			want:  "[]",
		},
		{
			name:  "string json becomes raw json",
			props: map[string]any{"graphs": `["semantic","entity"]`},
			want:  `["semantic","entity"]`,
		},
		{
			name:  "empty string uses fallback",
			props: map[string]any{"graphs": "  "},
			want:  "[]",
		},
		{
			name:  "bytes json becomes raw json",
			props: map[string]any{"graphs": []byte(`["temporal"]`)},
			want:  `["temporal"]`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := jsonProp(tt.props, "graphs", "[]").(json.RawMessage)
			if !ok {
				t.Fatalf("jsonProp() type = %T, want json.RawMessage", got)
			}
			if string(got) != tt.want {
				t.Fatalf("jsonProp() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPostgresGraphFloatProp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		props map[string]any
		want  float64
		ok    bool
	}{
		{name: "float64", props: map[string]any{"weight": 0.75}, want: 0.75, ok: true},
		{name: "float32", props: map[string]any{"weight": float32(0.5)}, want: 0.5, ok: true},
		{name: "int", props: map[string]any{"weight": 2}, want: 2, ok: true},
		{name: "json number", props: map[string]any{"weight": json.Number("0.25")}, want: 0.25, ok: true},
		{name: "missing", props: map[string]any{}, ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := floatProp(tt.props, "weight")
			if !tt.ok {
				if got != nil {
					t.Fatalf("floatProp() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("floatProp() = nil, want value")
			}
			if *got != tt.want {
				t.Fatalf("floatProp() = %v, want %v", *got, tt.want)
			}
		})
	}
}

func TestPostgresMagmaEventDeleteStatementsArePreparedSafe(t *testing.T) {
	t.Parallel()

	if len(postgresMagmaEventDeleteStatements) != 4 {
		t.Fatalf("expected 4 delete statements, got %d", len(postgresMagmaEventDeleteStatements))
	}
	for _, stmt := range postgresMagmaEventDeleteStatements {
		if strings.Count(stmt, "DELETE FROM") != 1 {
			t.Fatalf("delete statement must contain one command, got %q", stmt)
		}
		if strings.Contains(stmt, ";") {
			t.Fatalf("delete statement must not contain semicolons, got %q", stmt)
		}
	}
}

func TestFactory_DefaultsAndNone(t *testing.T) {
	t.Setenv("MANIFOLD_SECRETS_KEY", testSecretsKey(t))
	ctx := context.Background()
	// Defaults should create SQLite-backed local backends.
	mgr, err := NewManager(ctx, config.DBConfig{SQLite: config.SQLiteConfig{Path: t.TempDir() + "/manifold.db"}})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if mgr.Search == nil || mgr.Vector == nil || mgr.Graph == nil || mgr.CommandPolicy == nil {
		t.Fatalf("expected non-nil backends by default")
	}
	// None should create no-op backends
	mgr, err = NewManager(ctx, config.DBConfig{Backend: "memory", Search: config.SearchConfig{Backend: "none"}, Vector: config.VectorConfig{Backend: "none"}, Graph: config.GraphConfig{Backend: "none"}})
	if err != nil {
		t.Fatalf("NewManager error (none): %v", err)
	}
	// Calls should not error
	_ = mgr.Search.Index(ctx, "x", "y", nil)
	_, _ = mgr.Search.Search(ctx, "z", 1)
	_ = mgr.Vector.Upsert(ctx, "x", []float32{1}, nil)
	_, _ = mgr.Vector.SimilaritySearch(ctx, []float32{1}, 1, nil)
	_ = mgr.Graph.UpsertNode(ctx, "n", nil, nil)
}

func TestFactory_DefaultSQLitePersistsSpecialistsAcrossManagers(t *testing.T) {
	t.Setenv("MANIFOLD_SECRETS_KEY", testSecretsKey(t))

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifold.db")
	cfg := config.DBConfig{SQLite: config.SQLiteConfig{Path: dbPath}}
	mgr, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if mgr.Specialists == nil || mgr.SpecialistTeams == nil {
		t.Fatalf("expected sqlite specialist stores")
	}
	_, err = mgr.Specialists.Upsert(ctx, 0, persistence.Specialist{
		Name:        "writer",
		Provider:    "openai",
		Model:       "gpt-5-mini",
		Description: "durable specialist",
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	mgr.Close()

	restarted, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager after restart error: %v", err)
	}
	defer restarted.Close()
	got, ok, err := restarted.Specialists.GetByName(ctx, 0, "writer")
	if err != nil {
		t.Fatalf("get specialist after restart: %v", err)
	}
	if !ok || got.Description != "durable specialist" || got.Model != "gpt-5-mini" {
		t.Fatalf("specialist did not persist across manager restart: ok=%v got=%+v", ok, got)
	}
}

func TestFactory_DefaultSQLitePersistsTeamsAcrossManagers(t *testing.T) {
	t.Setenv("MANIFOLD_SECRETS_KEY", testSecretsKey(t))

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifold.db")
	cfg := config.DBConfig{SQLite: config.SQLiteConfig{Path: dbPath}}
	mgr, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	_, err = mgr.Specialists.Upsert(ctx, 0, persistence.Specialist{
		Name:     "lead",
		Provider: "openai",
		Model:    "gpt-5-mini",
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = mgr.SpecialistTeams.Upsert(ctx, 0, persistence.SpecialistTeam{
		Name:             "ops",
		Description:      "durable team",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}
	mgr.Close()

	restarted, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager after restart error: %v", err)
	}
	defer restarted.Close()
	got, ok, err := restarted.SpecialistTeams.GetByName(ctx, 0, "ops")
	if err != nil {
		t.Fatalf("get team after restart: %v", err)
	}
	if !ok || got.Description != "durable team" || got.OrchestratorName != "lead" {
		t.Fatalf("team did not persist across manager restart: ok=%v got=%+v", ok, got)
	}
	if len(got.Members) != 1 || got.Members[0] != "lead" {
		t.Fatalf("team membership did not persist across manager restart: %+v", got.Members)
	}
	listCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	teams, err := restarted.SpecialistTeams.List(listCtx, 0)
	if err != nil {
		t.Fatalf("list teams after restart: %v", err)
	}
	if len(teams) != 1 || teams[0].Name != "ops" || len(teams[0].Members) != 1 || teams[0].Members[0] != "lead" {
		t.Fatalf("unexpected teams after restart: %+v", teams)
	}
}

func TestSQLiteTeamStoreListDoesNotDeadlockWithSingleConnection(t *testing.T) {
	t.Setenv("MANIFOLD_SECRETS_KEY", testSecretsKey(t))

	ctx := context.Background()
	mgr, err := NewManager(ctx, config.DBConfig{
		SQLite: config.SQLiteConfig{
			Path:         filepath.Join(t.TempDir(), "manifold.db"),
			MaxOpenConns: 1,
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	defer mgr.Close()
	_, err = mgr.SpecialistTeams.Upsert(ctx, 0, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	teams, err := mgr.SpecialistTeams.List(listCtx, 0)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 1 || teams[0].Name != "ops" || len(teams[0].Members) != 1 || teams[0].Members[0] != "lead" {
		t.Fatalf("unexpected teams: %+v", teams)
	}
}

func TestFactory_RejectsPostgresWithoutDSN(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, err := NewManager(ctx, config.DBConfig{
		Search: config.SearchConfig{Backend: "postgres"},
	})
	if err == nil || err.Error() != "search backend postgres requires DSN" {
		t.Fatalf("expected search DSN error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Vector: config.VectorConfig{Backend: "postgres"},
	})
	if err == nil || err.Error() != "vector backend postgres requires DSN" {
		t.Fatalf("expected vector DSN error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Graph: config.GraphConfig{Backend: "postgres"},
	})
	if err == nil || err.Error() != "graph backend postgres requires DSN" {
		t.Fatalf("expected graph DSN error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Chat: config.ChatConfig{Backend: "postgres"},
	})
	if err == nil || err.Error() != "chat backend postgres requires DSN" {
		t.Fatalf("expected chat DSN error, got %v", err)
	}
}

func TestFactory_RejectsUnsupportedBackends(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, err := NewManager(ctx, config.DBConfig{
		Search: config.SearchConfig{Backend: "bogus"},
	})
	if err == nil || err.Error() != "unsupported search backend: bogus" {
		t.Fatalf("expected unsupported search backend error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Vector: config.VectorConfig{Backend: "bogus"},
	})
	if err == nil || err.Error() != "unsupported vector backend: bogus" {
		t.Fatalf("expected unsupported vector backend error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Graph: config.GraphConfig{Backend: "bogus"},
	})
	if err == nil || err.Error() != "unsupported graph backend: bogus" {
		t.Fatalf("expected unsupported graph backend error, got %v", err)
	}

	_, err = NewManager(ctx, config.DBConfig{
		Chat: config.ChatConfig{Backend: "bogus"},
	})
	if err == nil || err.Error() != "unsupported chat backend: bogus" {
		t.Fatalf("expected unsupported chat backend error, got %v", err)
	}
}

func openTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlitep.Open(context.Background(), sqlitep.Config{
		Path:          filepath.Join(t.TempDir(), "manifold.db"),
		BusyTimeoutMs: 10000,
		WAL:           true,
		MaxOpenConns:  1,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCloseIfPossible_IgnoresTypedNilAndCallsClosers(t *testing.T) {
	t.Parallel()

	var nilPlayground *PlaygroundStore
	closeIfPossible(nilPlayground)

	called := 0
	closeIfPossible(testCloser{called: &called})
	if called != 1 {
		t.Fatalf("expected Close() closer to be called once, got %d", called)
	}

	closeIfPossible(testErrorCloser{called: &called})
	if called != 2 {
		t.Fatalf("expected Close() error closer to be called once, got %d", called)
	}
}

type testCloser struct {
	called *int
}

func (c testCloser) Close() {
	*c.called = *c.called + 1
}

type testErrorCloser struct {
	called *int
}

func (c testErrorCloser) Close() error {
	*c.called = *c.called + 1
	return errors.New("boom")
}
