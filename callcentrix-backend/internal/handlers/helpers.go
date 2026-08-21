package handlers

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// operatorUserType is the users.user_type value for the Operator role
// (see migrations/001_init.sql: 0=SuperAdmin 1=TenantAdmin 2=Supervisor 3=Operator).
const operatorUserType = 3

// jwtTTLFor picks the token lifetime for a user: Operators get their own
// (longer) TTL since they work full shifts, everyone else gets the default.
func jwtTTLFor(userType, defaultMinutes, operatorMinutes int) int {
	if userType == operatorUserType {
		return operatorMinutes
	}
	return defaultMinutes
}
