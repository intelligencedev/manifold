package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (a *app) handleDevMockChat(w http.ResponseWriter, r *http.Request, prompt string) bool {
	prun := a.runs.create(prompt)
	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, _ := w.(http.Flusher)
		if b, err := json.Marshal("(dev) mock response: " + prompt); err == nil {
			fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
		} else {
			fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
		}
		if fl != nil {
			fl.Flush()
		}
		a.runs.updateStatus(prun.ID, "completed", 0)
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + prompt})
	a.runs.updateStatus(prun.ID, "completed", 0)
	return true
}
