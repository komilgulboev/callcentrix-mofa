package handlers

import (
	"database/sql"
	"net/http"

	"callcentrix/internal/jwt"
	mw "callcentrix/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB                 *sql.DB
	JWTSecret          string
	JWTMinutes         int
	JWTOperatorMinutes int
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	var (
		id           int
		passwordHash string
		tenantID     *int
		userType     int
		role         int
		active       bool
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash, tenant_id, user_type, role, active FROM users WHERE username = $1`,
		req.Username,
	).Scan(&id, &passwordHash, &tenantID, &userType, &role, &active)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if !active {
		writeError(w, http.StatusForbidden, "account inactive")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	ttl := jwtTTLFor(userType, h.JWTMinutes, h.JWTOperatorMinutes)
	token, err := jwt.Generate(h.JWTSecret, ttl, id, req.Username, tenantID, userType, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := mw.GetClaims(r)
	writeJSON(w, http.StatusOK, claims)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Stateless JWT — client just discards the token
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ChangePassword lets any authenticated user change their own password
// (unlike UsersHandler.ResetPassword, which is an admin-only reset by user
// id with no current-password check). The target is always claims.Sub —
// never trust an id from the request for a self-service endpoint.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := mw.GetClaims(r)

	var req changePasswordRequest
	if err := decode(r, &req); err != nil || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(req.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "password_too_short")
		return
	}

	var (
		passwordHash string
		username     string
	)
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT password_hash, username FROM users WHERE id = $1`, claims.Sub,
	).Scan(&passwordHash, &username); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	// 400, not 401: the frontend's fetch wrapper treats any 401 as "session
	// expired" and force-logs the user out, which a wrong-current-password
	// typo shouldn't trigger.
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_current_password")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash error")
		return
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET password_hash=$1, sip_password=$2, updated_at=NOW() WHERE id=$3`,
		string(hash), req.NewPassword, claims.Sub,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Keep the Asterisk SIP account's auth password in sync, same as
	// UsersHandler.ResetPassword — otherwise the softphone stops registering.
	if username != "" {
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE ast_ps_auths SET password=$1 WHERE id=$2`, req.NewPassword, username)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password updated"})
}
