package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AnalyseDeCircuit/opskernel/internal/power"
)

// GUIStatusHandler returns display/session manager state (auth required).
// GET /api/gui/status
func GUIStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	status, err := power.GetGUIStatus()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to read GUI status: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// GUIActionHandler toggles display/session manager (admin only).
// POST /api/gui/action {"action":"start"|"stop"}
func GUIActionHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	enable := false
	switch req.Action {
	case "start", "enable", "on":
		enable = true
	case "stop", "disable", "off":
		enable = false
	default:
		writeJSONError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	if err := power.SetGUIRunning(enable); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
