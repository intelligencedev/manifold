package warpp

import (
	"context"
	"fmt"
	"sync"
)

// runMapNode executes a control.map node: it runs the body subgraph once per
// item, gathering the body's single `result` output into a list (spec §6).
func (e *Engine) runMapNode(ctx context.Context, node *Node, in NodeInputs, scope *execScope, path string, defaults Policy) (map[string]Value, bool, error) {
	itemsVal, ok := in.Values["items"]
	if !ok {
		return nil, false, fmt.Errorf("map %s: items input missing", node.ID)
	}
	items, ok := itemsVal.Data.([]any)
	if !ok {
		return nil, false, fmt.Errorf("map %s: items is not a list", node.ID)
	}
	elem := Type{Kind: itemsVal.Type.Elem}
	if elem.Kind == "" || elem.Kind == KindVar {
		elem = Type{Kind: KindJSON}
	}
	concurrency := 4
	if c, ok := in.Values["concurrency"]; ok {
		if n, ok := asNumber(c.Data); ok && n >= 1 {
			concurrency = int(n)
		}
	}
	onItemError := "fail"
	if v, ok := in.Values["on_item_error"]; ok {
		if s, ok := v.Data.(string); ok && s != "" {
			onItemError = s
		}
	}

	type itemResult struct {
		value   Value
		fired   bool
		skipped bool
		err     error
	}
	results := make([]itemResult, len(items))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	iterCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-iterCtx.Done():
				results[i] = itemResult{err: iterCtx.Err()}
				return
			}

			itemScope := &execScope{
				parent:  scope,
				mu:      scope.mu,
				nodeSet: map[string]bool{ReservedItemNode: true},
				outputs: map[string]map[string]Value{
					ReservedItemNode: {
						"value": {Type: elem, Data: items[i]},
						"index": {Type: Type{Kind: KindNumber}, Data: float64(i)},
					},
				},
				terminal: map[string]bool{ReservedItemNode: true},
				skipped:  map[string]bool{},
			}
			for _, bn := range node.Body.Nodes {
				itemScope.nodeSet[bn.ID] = true
			}
			prefix := fmt.Sprintf("%s[%d].", path, i)
			_, err := e.runScope(iterCtx, node.Body.Nodes, itemScope, prefix, defaults, e.maxConc())
			if err != nil {
				results[i] = itemResult{err: err}
				if onItemError != "skip" {
					cancel()
				}
				return
			}
			binding := node.Body.Outputs["result"]
			if binding.HasValue {
				results[i] = itemResult{value: Value{Type: InferLiteral(binding.Value), Data: binding.Value}, fired: true}
				return
			}
			ref, refErr := ParseRef(binding.From)
			if refErr != nil {
				results[i] = itemResult{err: refErr}
				return
			}
			v, fired, _ := itemScope.lookup(ref)
			if !fired {
				results[i] = itemResult{skipped: true}
				return
			}
			results[i] = itemResult{value: v, fired: true}
		}(i)
	}
	wg.Wait()

	gathered := make([]any, 0, len(items))
	anySkipped := false
	for i, r := range results {
		if r.err != nil {
			if onItemError == "skip" {
				anySkipped = true
				continue
			}
			return nil, false, fmt.Errorf("item %d: %w", i, r.err)
		}
		if !r.fired {
			anySkipped = true
			continue
		}
		gathered = append(gathered, r.value.Data)
	}
	outType := InferLiteral(gathered)
	return map[string]Value{"results": {Type: outType, Data: gathered}}, anySkipped, nil
}
