package handlers

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return false
	}
	return true
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return "", false
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "Forbidden: Admin access required")
		return "", false
	}
	return username, true
}

func requireAuth(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return "", "", false
	}
	return username, role, true
}
