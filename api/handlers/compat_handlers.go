package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AnalyseDeCircuit/opskernel/internal/monitoring"
	"github.com/AnalyseDeCircuit/opskernel/internal/power"
	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
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

func PowerProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, _, ok := requireAuth(w, r); !ok {
			return
		}

		info, err := power.GetPowerProfiles()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to read power profiles: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, info)
		return
	}

	if r.Method == http.MethodPost {
		if _, ok := requireAdmin(w, r); !ok {
			return
		}

		var req struct {
			Profile string `json:"profile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if err := power.SetPowerProfile(req.Profile); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
}
