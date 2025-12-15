package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// AlertsHandler provides legacy-compatible alert config endpoints.
// GET  /api/alerts  -> return current alert config
// POST /api/alerts  -> update alert config (admin-only)
func AlertsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, _, ok := requireAuth(w, r); !ok {
			return
		}
		cfg := monitoring.GetAlertConfig()
		writeJSON(w, http.StatusOK, cfg)
		return
	}

	if r.Method == http.MethodPost {
		if _, ok := requireAdmin(w, r); !ok {
			return
		}
		var cfg types.AlertConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := monitoring.UpdateAlertConfig(cfg); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to save alerts: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

// PowerProfileHandler provides a legacy-compatible stub.
// The new backend uses /api/power/action for power operations; profile is not universally supported.
func PowerProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, _, ok := requireAuth(w, r); !ok {
			return
		}
		// Frontend hides the card if available is empty or error is set.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": []string{},
			"current":   "",
			"error":     "not supported",
		})
		return
	}

	if r.Method == http.MethodPost {
		if _, ok := requireAdmin(w, r); !ok {
			return
		}
		writeJSONError(w, http.StatusNotImplemented, "Power profile not supported in this build")
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
}
