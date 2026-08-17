package handlers

import (
	"database/sql"
	"net/http"

	"callcentrix/internal/jwt"
	mw "callcentrix/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB         *sql.DB
	JWTSecret  string
	JWTMinutes int
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

	token, err := jwt.Generate(h.JWTSecret, h.JWTMinutes, id, req.Username, tenantID, userType, role)
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
