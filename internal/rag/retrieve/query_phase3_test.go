package retrieve

// Phase 3 tests for retrieve/query.go:
//   C1 – detectLang returns "" so no spurious lang filter is injected

import (
	"context"
	"testing"
)

func TestDetectLang_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	// detectLang is a stub until a real detector is plugged in.
	// It must return "" to avoid injecting a spurious lang filter.
	if got := detectLang("hello world"); got != "" {
		t.Fatalf("detectLang should return \"\", got %q", got)
	}
	if got := detectLang(""); got != "" {
		t.Fatalf("detectLang(\"\") should return \"\", got %q", got)
	}
}

func TestBuildQueryPlan_NoLangFilterInjected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plan := BuildQueryPlan(ctx, "hello world", RetrieveOptions{K: 10, Alpha: 0.5})

	if _, ok := plan.Filters["lang"]; ok {
		t.Fatalf("BuildQueryPlan should not inject lang filter when detectLang returns \"\"; got filters: %v", plan.Filters)
	}
	// Lang field on the plan itself should also be ""
	if plan.Lang != "" {
		t.Fatalf("plan.Lang should be \"\" when no detector is present; got %q", plan.Lang)
	}
}

func TestBuildQueryPlan_TenantFilterPreserved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plan := BuildQueryPlan(ctx, "hello world", RetrieveOptions{K: 5, Alpha: 0.5, Tenant: "acme"})

	if plan.Filters["tenant"] != "acme" {
		t.Fatalf("expected tenant=acme in filters; got %v", plan.Filters)
	}
	// Still no lang filter
	if _, ok := plan.Filters["lang"]; ok {
		t.Fatalf("unexpected lang filter: %v", plan.Filters)
	}
}
