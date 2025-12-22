package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AnalyseDeCircuit/opskernel/internal/network"
)

// SSHStatsHandler returns SSH stats. Use ?force=1 to bypass TTL caches.
func SSHStatsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	force := false
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("force"))) {
	case "1", "true", "yes", "y", "on":
		force = true
	}

	stats := network.GetSSHStatsWithOptions(force)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ssh_stats": stats,
	})
}
