package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/asterisk"
	mw "callcentrix/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

type UsersHandler struct {
	DB           *sql.DB
	SIPTransport string
}

type User struct {
	ID         int     `json:"id"`
	TenantID   *int    `json:"tenantId"`
	Username   string  `json:"username"`
	FirstName  string  `json:"firstName"`
	LastName   string  `json:"lastName"`
	UserType   int     `json:"userType"`
	Role       int     `json:"role"`
	SipNo      string  `json:"sipNo"`
	Active     bool    `json:"active"`
	CreatedAt  string  `json:"createdAt"`
	ServerName *string `json:"serverName"` // which Asterisk server this agent is assigned to, if any
}

func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var rows *sql.Rows
	var err error

	if c.UserType == 0 {
		rows, err = h.DB.QueryContext(r.Context(),
			`SELECT u.id, u.tenant_id, u.username, u.first_name, u.last_name, u.user_type, u.role, u.sip_no, u.active, u.created_at, s.name
			 FROM users u LEFT JOIN asterisk_servers s ON s.id = u.server_id
			 WHERE u.phone_verified = TRUE ORDER BY u.id`)
	} else {
		rows, err = h.DB.QueryContext(r.Context(),
			`SELECT u.id, u.tenant_id, u.username, u.first_name, u.last_name, u.user_type, u.role, u.sip_no, u.active, u.created_at, s.name
			 FROM users u LEFT JOIN asterisk_servers s ON s.id = u.server_id
			 WHERE u.tenant_id = $1 AND u.phone_verified = TRUE ORDER BY u.id`, c.TenantID)
	}
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	defer rows.Close()

	result := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Username, &u.FirstName, &u.LastName,
			&u.UserType, &u.Role, &u.SipNo, &u.Active, &u.CreatedAt, &u.ServerName); err != nil {
			continue
		}
		result = append(result, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var u User
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, tenant_id, username, first_name, last_name, user_type, role, sip_no, active, created_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.TenantID, &u.Username, &u.FirstName, &u.LastName,
		&u.UserType, &u.Role, &u.SipNo, &u.Active, &u.CreatedAt)
	if err == sql.ErrNoRows { writeError(w, http.StatusNotFound, "not found"); return }
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	writeJSON(w, http.StatusOK, u)
}

func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		UserType  int    `json:"userType"`
		Role      int    `json:"role"`
		SipNo     string `json:"sipNo"`
		TenantID  *int   `json:"tenantId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if c.UserType != 0 {
		body.TenantID = c.TenantID
	}

	sipNo := body.SipNo
	if sipNo == "" {
		sipNo = body.Username
	}

	if code := checkUsernameAvailable(r.Context(), h.DB, body.Username, sipNo); code != "" {
		writeError(w, http.StatusConflict, code)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil { writeError(w, http.StatusInternalServerError, "hash error"); return }

	serverID := pickServerID(h.DB)

	var id int
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO users
			(tenant_id, username, password_hash, sip_password, first_name, last_name, user_type, role, sip_no, active, server_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,FALSE,$10) RETURNING id`,
		body.TenantID, body.Username, string(hash),
		body.Password,
		body.FirstName, body.LastName, body.UserType, body.Role, sipNo, serverID,
	).Scan(&id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Create SIP account immediately so the phone is ready before activation
	if body.Password != "" {
		ctx := "default"
		if body.TenantID != nil {
			ctx = asterisk.TenantContext(*body.TenantID)
		}
		if err := asterisk.CreateSIPAccount(h.DB, body.Username, body.Password, ctx, h.SIPTransport); err != nil {
			log.Printf("[Users] SIP create warning for %s: %v", body.Username, err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		UserType  int    `json:"userType"`
		Role      int    `json:"role"`
		SipNo     string `json:"sipNo"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	sipNo := body.SipNo
	if sipNo == "" {
		sipNo = body.Username
	}

	if body.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil { writeError(w, http.StatusInternalServerError, "hash error"); return }
		_, err = h.DB.ExecContext(r.Context(),
			`UPDATE users SET username=$1, password_hash=$2, sip_password=$3,
			 first_name=$4, last_name=$5, user_type=$6, role=$7, sip_no=$8, updated_at=NOW()
			 WHERE id=$9`,
			body.Username, string(hash), body.Password,
			body.FirstName, body.LastName, body.UserType, body.Role, sipNo, id)
		if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

		// Update SIP password in Asterisk if account exists
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE ast_ps_auths SET password=$1 WHERE id=$2`, body.Password, body.Username)
	} else {
		_, err := h.DB.ExecContext(r.Context(),
			`UPDATE users SET username=$1, first_name=$2, last_name=$3,
			 user_type=$4, role=$5, sip_no=$6, updated_at=NOW() WHERE id=$7`,
			body.Username, body.FirstName, body.LastName,
			body.UserType, body.Role, sipNo, id)
		if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Get username before deleting
	var username string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT username FROM users WHERE id=$1`, id).Scan(&username)

	_, err := h.DB.ExecContext(r.Context(), `DELETE FROM users WHERE id=$1`, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Remove SIP account
	if username != "" {
		_ = asterisk.DeleteSIPAccount(h.DB, username)
	}
	w.WriteHeader(http.StatusNoContent)
}

// pickServerID auto-assigns a newly created user to the least-loaded active
// Asterisk server (see asterisk.PickLeastLoadedServer), or nil if none are
// configured — the user then falls back to the single default AMI/WS server,
// keeping single-box deployments working unchanged. Errors are logged and
// treated the same as "no servers configured" rather than failing user
// creation over what's a best-effort load-balancing hint.
func pickServerID(db *sql.DB) *int {
	id, err := asterisk.PickLeastLoadedServer(db)
	if err != nil {
		log.Printf("[Users] pick least loaded server: %v", err)
		return nil
	}
	if id == 0 {
		return nil
	}
	return &id
}

func (h *UsersHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Get user info needed for SIP creation
	var username, sipPassword string
	var tenantID *int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT username, sip_password, tenant_id FROM users WHERE id=$1`, id,
	).Scan(&username, &sipPassword, &tenantID)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Determine dialplan context
	ctx := "default"
	if tenantID != nil {
		ctx = asterisk.TenantContext(*tenantID)
	}

	// Create SIP account in Asterisk
	if sipPassword != "" {
		if err := asterisk.CreateSIPAccount(h.DB, username, sipPassword, ctx, h.SIPTransport); err != nil {
			writeError(w, http.StatusInternalServerError, "SIP: "+err.Error())
			return
		}
	}

	_, err = h.DB.ExecContext(r.Context(),
		`UPDATE users SET active=TRUE, updated_at=NOW() WHERE id=$1`, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var username string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT username FROM users WHERE id=$1`, id).Scan(&username)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET active=FALSE, updated_at=NOW() WHERE id=$1`, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Remove SIP account from Asterisk
	if username != "" {
		_ = asterisk.DeleteSIPAccount(h.DB, username)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil { writeError(w, http.StatusInternalServerError, "hash error"); return }

	var username string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT username FROM users WHERE id=$1`, id).Scan(&username)

	_, err = h.DB.ExecContext(r.Context(),
		`UPDATE users SET password_hash=$1, sip_password=$2, updated_at=NOW() WHERE id=$3`,
		string(hash), body.Password, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Sync to Asterisk
	if username != "" {
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE ast_ps_auths SET password=$1 WHERE id=$2`, body.Password, username)
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkUsernameAvailable checks that username/sipNo aren't already taken, by
// another user or by an Asterisk endpoint. Returns "username_exists" or
// "sip_exists" if taken, "" if free. Shared by admin-created users (Create)
// and public self-registration (RegistrationHandler.Register).
func checkUsernameAvailable(ctx context.Context, db *sql.DB, username, sipNo string) string {
	var cnt int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE username = $1`, username,
	).Scan(&cnt); err == nil && cnt > 0 {
		return "username_exists"
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE sip_no = $1`, sipNo,
	).Scan(&cnt); err == nil && cnt > 0 {
		return "sip_exists"
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ast_ps_auths WHERE id = $1`, sipNo,
	).Scan(&cnt); err == nil && cnt > 0 {
		return "sip_exists"
	}
	return ""
}

// authorizeUser marks a self-registered user's phone as verified, activates
// the account, and creates its SIP endpoint in Asterisk. Used both when the
// user enters the correct SMS code themselves (RegistrationHandler.VerifyCode)
// and when SuperAdmin manually authorizes them from the unauthorized-users list.
func authorizeUser(ctx context.Context, db *sql.DB, sipTransport string, id int) error {
	var username, sipPassword string
	var tenantID *int
	err := db.QueryRowContext(ctx,
		`SELECT username, sip_password, tenant_id FROM users WHERE id=$1`, id,
	).Scan(&username, &sipPassword, &tenantID)
	if err != nil {
		return err
	}

	dialCtx := "default"
	if tenantID != nil {
		dialCtx = asterisk.TenantContext(*tenantID)
	}
	if sipPassword != "" {
		if err := asterisk.CreateSIPAccount(db, username, sipPassword, dialCtx, sipTransport); err != nil {
			return fmt.Errorf("SIP: %w", err)
		}
	}

	_, err = db.ExecContext(ctx,
		`UPDATE users SET active=TRUE, phone_verified=TRUE, auth_code='', updated_at=NOW() WHERE id=$1`, id)
	return err
}

type unauthorizedUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	SipNo     string `json:"sipNo"`
	AuthCode  string `json:"authCode"`
	CreatedAt string `json:"createdAt"`
}

// ListUnauthorized returns self-registered users who haven't yet confirmed
// their SMS code — including the code itself, so SuperAdmin can relay it
// manually if delivery failed. SuperAdmin only.
func (h *UsersHandler) ListUnauthorized(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, username, first_name, last_name, sip_no, auth_code, created_at
		 FROM users WHERE phone_verified = FALSE ORDER BY created_at DESC`)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	defer rows.Close()

	result := []unauthorizedUser{}
	for rows.Next() {
		var u unauthorizedUser
		if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.SipNo, &u.AuthCode, &u.CreatedAt); err != nil {
			continue
		}
		result = append(result, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

// Authorize manually confirms a self-registered user without requiring their
// SMS code — the fallback for when delivery failed. SuperAdmin only.
func (h *UsersHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := authorizeUser(r.Context(), h.DB, h.SIPTransport, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
