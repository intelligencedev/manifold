package retrieve

import (
	"context"
	"math"
	"strings"
)

// Maximum number of allowed filter keys to avoid excessive allocation or overflow
const maxFilterEntries = 1000

// QueryPlan is the normalized retrieval plan derived from input query and options.
type QueryPlan struct {
	Query   string
	Lang    string
	FtK     int
	VecK    int
	Filters map[string]string
	Tenant  string
}

// BuildQueryPlan normalizes the query, detects language (best-effort),
// splits candidate budgets between FTS and vector using Alpha, and builds
// metadata filters (tenant, lang, plus any provided Filter entries).
func BuildQueryPlan(ctx context.Context, q string, opt RetrieveOptions) QueryPlan { // ctx reserved for future pluggable detectors
	_ = ctx
	nq := normalizeQuery(q)
	lang := detectLang(nq)

	k := opt.K
	if k <= 0 {
		k = 10
	}
	if k > 1000 {
		k = 1000 // sanity cap to avoid runaway allocations
	}
	ftK, vecK := splitBudgets(k, opt)
	// Defensive: only allow up to maxFilterEntries nonempty entries in the filters map,
	// regardless of the size of opt.Filter, to prevent excessive allocation or overflow.
	entriesAdded := 0
	filters := make(map[string]string, maxFilterEntries+2)
	for k, v := range opt.Filter {
		if entriesAdded >= maxFilterEntries {
			break
		}
		if v != "" {
			filters[k] = v
			entriesAdded++
		}
	}
	if opt.Tenant != "" {
		filters["tenant"] = opt.Tenant
	}
	if lang != "" {
		filters["lang"] = lang
	}

	return QueryPlan{Query: nq, Lang: lang, FtK: ftK, VecK: vecK, Filters: filters, Tenant: opt.Tenant}
}

func normalizeQuery(q string) string {
	// Collapse whitespace and trim. Keep case for display but search is case-insensitive in backends.
	s := strings.TrimSpace(q)
	// Replace multiple spaces with single
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func detectLang(_ string) string {
	// Placeholder: default to english until a detector is plugged in
	return "english"
}

func splitBudgets(k int, opt RetrieveOptions) (int, int) {
	// If explicit FtK/VecK provided, honor them but cap by k and ensure non-negative.
	if opt.FtK > 0 || opt.VecK > 0 {
		ft := opt.FtK
		vc := opt.VecK
		if ft < 0 {
			ft = 0
		}
		if vc < 0 {
			vc = 0
		}
		if ft+vc == 0 {
			ft = k
		}
		if ft > k {
			ft = k
		}
		if vc > k {
			vc = k
		}
		return ft, vc
	}
	// Derive from Alpha where Alpha is the weight on FTS.
	a := opt.Alpha
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	ft := int(math.Ceil(float64(k) * a))
	vc := k - ft
	if ft == 0 && k > 0 {
		ft = 1
		vc = k - 1
	}
	if vc == 0 && k > 0 && k > 1 { // ensure both sides represented for k>1
		vc = 1
		ft = k - 1
	}
	return ft, vc
}
