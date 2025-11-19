package ingest

import "context"

// DocumentLookup provides the minimal capability to check for existing docs by hash.
type DocumentLookup interface {
	// LookupByHash returns (docID, version, ok, err)
	LookupByHash(ctx context.Context, hash string, tenant string) (string, int, bool, error)
}

// IdempotencyDecision indicates what action to take based on policy and lookup.
type IdempotencyDecision struct {
	Action  string // "skip", "overwrite", "new_version", "create"
	DocID   string
	Version int
}

// ResolveIdempotency applies policy to an existing-or-not doc state.
func ResolveIdempotency(ctx context.Context, lookup DocumentLookup, tenant string, req IngestRequest, pre PreprocessedDoc) (IdempotencyDecision, error) {
	if lookup == nil {
		// default: always create when we cannot check
		return IdempotencyDecision{Action: "create", DocID: req.ID, Version: req.Options.Version}, nil
	}
	docID, ver, ok, err := lookup.LookupByHash(ctx, pre.Hash, tenant)
	if err != nil {
		return IdempotencyDecision{}, err
	}
	pol := req.Options.ReingestPolicy
	switch pol {
	case ReingestSkipIfUnchanged:
		if ok {
			return IdempotencyDecision{Action: "skip", DocID: docID, Version: ver}, nil
		}
		return IdempotencyDecision{Action: "create", DocID: req.ID, Version: req.Options.Version}, nil
	case ReingestOverwrite:
		if ok {
			return IdempotencyDecision{Action: "overwrite", DocID: docID, Version: ver}, nil
		}
		return IdempotencyDecision{Action: "create", DocID: req.ID, Version: req.Options.Version}, nil
	case ReingestNewVersion:
		if ok {
			return IdempotencyDecision{Action: "new_version", DocID: docID, Version: ver + 1}, nil
		}
		// first version if new
		v := req.Options.Version
		if v == 0 {
			v = 1
		}
		return IdempotencyDecision{Action: "create", DocID: req.ID, Version: v}, nil
	default:
		// default to skip-if-unchanged semantics
		if ok {
			return IdempotencyDecision{Action: "skip", DocID: docID, Version: ver}, nil
		}
		return IdempotencyDecision{Action: "create", DocID: req.ID, Version: req.Options.Version}, nil
	}
}
