package handlers

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"callcentrix/internal/jwt"
	"callcentrix/internal/smpp"
	"golang.org/x/crypto/bcrypt"
)

// RegistrationHandler implements public self-registration: a user submits
// their name, phone (which doubles as username/SIP number) and password, an
// SMS code is sent via the SMPP gateway, and entering it correctly activates
// the account and logs the user in. Self-registered users start with
// tenant_id=NULL — SuperAdmin later assigns them to a tenant from the
// existing "Назначение" (TenantUsers) page, same as any unassigned user.
type RegistrationHandler struct {
	DB           *sql.DB
	JWTSecret    string
	JWTMinutes   int
	SIPTransport string
}

// generateCode returns a random 6-digit numeric code.
func generateCode() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := make([]byte, 6)
	for i, v := range b {
		code[i] = '0' + v%10
	}
	return string(code), nil
}

// Register creates a pending (unverified) user account and sends an SMS
// confirmation code. Public — no auth.
func (h *RegistrationHandler) Register(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT registration_enabled FROM system_settings WHERE id=1`).Scan(&enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !enabled {
		writeError(w, http.StatusForbidden, "registration_disabled")
		return
	}

	var body struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Phone     string `json:"phone"`
		Password  string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Phone = strings.TrimSpace(body.Phone)
	if body.Phone == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "phone and password required")
		return
	}

	if code := checkUsernameAvailable(r.Context(), h.DB, body.Phone, body.Phone); code != "" {
		writeError(w, http.StatusConflict, code)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash error")
		return
	}

	authCode, err := generateCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "code error")
		return
	}

	serverID := pickServerID(h.DB)

	_, err = h.DB.ExecContext(r.Context(),
		`INSERT INTO users
			(tenant_id, username, password_hash, sip_password, first_name, last_name,
			 user_type, role, sip_no, active, phone_verified, auth_code, auth_code_sent_at, server_id)
		 VALUES (NULL,$1,$2,$3,$4,$5,3,0,$1,FALSE,FALSE,$6,NOW(),$7)`,
		body.Phone, string(hash), body.Password, body.FirstName, body.LastName, authCode, serverID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go h.sendCode(body.Phone, authCode)

	writeJSON(w, http.StatusCreated, map[string]string{"username": body.Phone})
}

// sendCode delivers the SMS in the background — best-effort. If the SMPP
// gateway is unreachable or misconfigured, the code stays visible to
// SuperAdmin in the unauthorized-users list as a fallback.
func (h *RegistrationHandler) sendCode(phone, code string) {
	var cfg smpp.Config
	err := h.DB.QueryRow(
		`SELECT host, port, system_id, password, sender_id FROM smpp_settings WHERE id=1`,
	).Scan(&cfg.Host, &cfg.Port, &cfg.SystemID, &cfg.Password, &cfg.SenderID)
	if err != nil {
		log.Printf("[Register] smpp settings load failed: %v", err)
		return
	}
	message := fmt.Sprintf("Ваш код подтверждения: %s", code)
	if err := smpp.SendSMS(cfg, phone, message); err != nil {
		log.Printf("[Register] SMS send to %s failed: %v", phone, err)
	}
}

// VerifyCode confirms a pending registration's SMS code, activates the
// account, and logs the user in. Public — no auth.
func (h *RegistrationHandler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Code     string `json:"code"`
	}
	if err := decode(r, &body); err != nil || body.Username == "" || body.Code == "" {
		writeError(w, http.StatusBadRequest, "username and code required")
		return
	}

	var (
		id       int
		tenantID *int
		userType int
		role     int
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, tenant_id, user_type, role FROM users
		 WHERE username=$1 AND phone_verified=FALSE AND auth_code=$2`,
		body.Username, body.Code,
	).Scan(&id, &tenantID, &userType, &role)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := authorizeUser(r.Context(), h.DB, h.SIPTransport, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	token, err := jwt.Generate(h.JWTSecret, h.JWTMinutes, id, body.Username, tenantID, userType, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}
