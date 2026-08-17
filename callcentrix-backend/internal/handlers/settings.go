package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SettingsHandler manages the platform-wide branding shown on the login
// screen (logo, platform name, system info). It is a single global row —
// not per-tenant — since the login screen renders before the app knows
// which tenant a user belongs to.
type SettingsHandler struct {
	DB         *sql.DB
	UploadsDir string
}

type brandingSettings struct {
	PlatformName        string `json:"platformName"`
	SystemInfo          string `json:"systemInfo"`
	HasLogo             bool   `json:"hasLogo"`
	UpdatedAt           string `json:"updatedAt"`
	RegistrationEnabled bool   `json:"registrationEnabled"`
}

// GetBranding returns the platform branding. Public — no auth — because the
// login screen (pre-auth) needs it.
func (h *SettingsHandler) GetBranding(w http.ResponseWriter, r *http.Request) {
	var (
		platformName string
		systemInfo   string
		logoPath     string
		updatedAt    string
		regEnabled   bool
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT platform_name, system_info, logo_path, updated_at, registration_enabled FROM system_settings WHERE id=1`,
	).Scan(&platformName, &systemInfo, &logoPath, &updatedAt, &regEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, brandingSettings{
		PlatformName:        platformName,
		SystemInfo:          systemInfo,
		HasLogo:             logoPath != "",
		UpdatedAt:           updatedAt,
		RegistrationEnabled: regEnabled,
	})
}

// UpdateBranding saves the platform name, system info text, and whether
// public registration is offered on the login screen. SuperAdmin only.
func (h *SettingsHandler) UpdateBranding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlatformName        string `json:"platformName"`
		SystemInfo          string `json:"systemInfo"`
		RegistrationEnabled bool   `json:"registrationEnabled"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.PlatformName) == "" {
		writeError(w, http.StatusBadRequest, "platformName required")
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE system_settings SET platform_name=$1, system_info=$2, registration_enabled=$3, updated_at=NOW() WHERE id=1`,
		body.PlatformName, body.SystemInfo, body.RegistrationEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UploadLogo accepts an image file and stores it as the platform logo. SuperAdmin only.
func (h *SettingsHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(5 << 20) // 5 MB
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".svg", ".webp":
	default:
		writeError(w, http.StatusBadRequest, "supported formats: png, jpg, jpeg, svg, webp")
		return
	}

	dir := filepath.Join(h.UploadsDir, "branding")
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create upload dir")
		return
	}

	// Remove any previously uploaded logo (possibly a different extension)
	// so switching formats doesn't leave a stale file behind.
	if old, err := filepath.Glob(filepath.Join(dir, "logo.*")); err == nil {
		for _, f := range old {
			os.Remove(f)
		}
	}

	destPath := filepath.Join(dir, "logo"+ext)
	dst, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create file")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	logoPath := "branding/logo" + ext
	_, err = h.DB.ExecContext(r.Context(),
		`UPDATE system_settings SET logo_path=$1, updated_at=NOW() WHERE id=1`, logoPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"file": header.Filename})
}

// Logo serves the current platform logo file. Public — no auth — because the
// login screen (pre-auth) needs it.
func (h *SettingsHandler) Logo(w http.ResponseWriter, r *http.Request) {
	var logoPath string
	err := h.DB.QueryRowContext(r.Context(), `SELECT logo_path FROM system_settings WHERE id=1`).Scan(&logoPath)
	if err != nil || logoPath == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(h.UploadsDir, logoPath))
}

type smppSettingsResponse struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	SystemID    string `json:"systemId"`
	SenderID    string `json:"senderId"`
	HasPassword bool   `json:"hasPassword"`
}

// GetSMPPSettings returns the SMPP gateway config. SuperAdmin only — never
// includes the password itself, mirroring how SIP passwords are write-only.
func (h *SettingsHandler) GetSMPPSettings(w http.ResponseWriter, r *http.Request) {
	var (
		host, systemID, senderID, password string
		port                                int
	)
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT host, port, system_id, password, sender_id FROM smpp_settings WHERE id=1`,
	).Scan(&host, &port, &systemID, &password, &senderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, smppSettingsResponse{
		Host: host, Port: port, SystemID: systemID, SenderID: senderID,
		HasPassword: password != "",
	})
}

// UpdateSMPPSettings saves the SMPP gateway config. SuperAdmin only. An empty
// password leaves the previously saved password untouched.
func (h *SettingsHandler) UpdateSMPPSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		SystemID string `json:"systemId"`
		Password string `json:"password"`
		SenderID string `json:"senderId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Port == 0 {
		body.Port = 2775
	}

	var err error
	if body.Password == "" {
		_, err = h.DB.ExecContext(r.Context(),
			`UPDATE smpp_settings SET host=$1, port=$2, system_id=$3, sender_id=$4, updated_at=NOW() WHERE id=1`,
			body.Host, body.Port, body.SystemID, body.SenderID)
	} else {
		_, err = h.DB.ExecContext(r.Context(),
			`UPDATE smpp_settings SET host=$1, port=$2, system_id=$3, password=$4, sender_id=$5, updated_at=NOW() WHERE id=1`,
			body.Host, body.Port, body.SystemID, body.Password, body.SenderID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
