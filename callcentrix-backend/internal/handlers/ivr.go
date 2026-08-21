package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/asterisk"
	mw "callcentrix/internal/middleware"
)

// IVRHandler manages the greeting/menu/queue-settings/operators of a single
// KC number. Every method is scoped by the {id} URL param (kc_numbers.id);
// kcNumberID() resolves it and checks the caller is allowed to touch it.
type IVRHandler struct {
	DB         *sql.DB
	UploadsDir string
}

type IVRConfig struct {
	ID                 int    `json:"id"`
	TenantID           int    `json:"tenantId"`
	KCNumberID         int    `json:"kcNumberId"`
	GreetingFile       string `json:"greetingFile"`
	ClosedGreetingFile string `json:"closedGreetingFile"`
	MOHClass           string `json:"mohClass"`
	WaitTimeout        int    `json:"waitTimeout"`
	QueueTimeout       int    `json:"queueTimeout"`
	Strategy           string `json:"strategy"`
	MaxCallers         int    `json:"maxCallers"`
	WorkHoursEnabled   bool   `json:"workHoursEnabled"`
	WorkHoursStart     string `json:"workHoursStart"`
	WorkHoursEnd       string `json:"workHoursEnd"`
	WorkDays           string `json:"workDays"`
	WhitelistEnabled   bool   `json:"whitelistEnabled"`
	UpdatedAt          string `json:"updatedAt"`
}

type IVROption struct {
	ID         int    `json:"id"`
	KCNumberID int    `json:"kcNumberId"`
	Digit      string `json:"digit"`
	Label      string `json:"label"`
	Action     string `json:"action"`
	ActionData string `json:"actionData"`
	SortOrder  int    `json:"sortOrder"`
}

type IVRMember struct {
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Interface string `json:"interface"`
	Paused    bool   `json:"paused"`
}

// kcNumberID resolves the {id} URL param (a kc_numbers.id) and checks the
// caller's tenant owns it. Returns (kcNumberID, tenantID, error).
func (h *IVRHandler) kcNumberID(r *http.Request) (int, int, error) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id == 0 {
		return 0, 0, fmt.Errorf("invalid kc number id")
	}
	var tenantID int
	err = h.DB.QueryRowContext(r.Context(), `SELECT tenant_id FROM kc_numbers WHERE id=$1`, id).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return 0, 0, fmt.Errorf("kc number not found")
	}
	if err != nil {
		return 0, 0, err
	}

	c := mw.GetClaims(r)
	if c.UserType != 0 {
		if c.TenantID == nil || *c.TenantID != tenantID {
			return 0, 0, fmt.Errorf("forbidden")
		}
	}
	return id, tenantID, nil
}

// GetConfig returns the full IVR setup for a KC number: config + options + members.
func (h *IVRHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cfg IVRConfig
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT id, tenant_id, kc_number_id, greeting_file, closed_greeting_file, moh_class,
		        wait_timeout, queue_timeout, strategy, max_callers,
		        work_hours_enabled, work_hours_start, work_hours_end, work_days,
		        whitelist_enabled, updated_at
		 FROM ivr_configs WHERE kc_number_id=$1`, kcID,
	).Scan(&cfg.ID, &cfg.TenantID, &cfg.KCNumberID, &cfg.GreetingFile, &cfg.ClosedGreetingFile, &cfg.MOHClass,
		&cfg.WaitTimeout, &cfg.QueueTimeout, &cfg.Strategy, &cfg.MaxCallers,
		&cfg.WorkHoursEnabled, &cfg.WorkHoursStart, &cfg.WorkHoursEnd, &cfg.WorkDays,
		&cfg.WhitelistEnabled, &cfg.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	options, _ := h.listOptions(r, kcID)
	members, _ := h.listMembers(r, cfg.TenantID, kcID)

	writeJSON(w, http.StatusOK, map[string]any{
		"config":  cfg,
		"options": options,
		"members": members,
	})
}

// UpdateConfig saves queue strategy, timeouts, MOH class and the whitelist
// gate toggle for a KC number. Takes effect on the live dialplan only after
// Sync is called (same as greeting/menu/schedule changes).
func (h *IVRHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Strategy         string `json:"strategy"`
		WaitTimeout      int    `json:"waitTimeout"`
		QueueTimeout     int    `json:"queueTimeout"`
		MaxCallers       int    `json:"maxCallers"`
		MOHClass         string `json:"mohClass"`
		WhitelistEnabled bool   `json:"whitelistEnabled"`
		WorkHoursEnabled bool   `json:"workHoursEnabled"`
		WorkHoursStart   string `json:"workHoursStart"`
		WorkHoursEnd     string `json:"workHoursEnd"`
		WorkDays         string `json:"workDays"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.WaitTimeout == 0 {
		body.WaitTimeout = 5
	}
	if body.QueueTimeout == 0 {
		body.QueueTimeout = 300
	}
	if body.Strategy == "" {
		body.Strategy = "ringall"
	}
	if body.WorkHoursStart == "" {
		body.WorkHoursStart = "09:00"
	}
	if body.WorkHoursEnd == "" {
		body.WorkHoursEnd = "18:00"
	}
	if body.WorkDays == "" {
		body.WorkDays = "mon,tue,wed,thu,fri"
	}

	_, err = h.DB.ExecContext(r.Context(),
		`UPDATE ivr_configs SET strategy=$1, wait_timeout=$2, queue_timeout=$3,
		  max_callers=$4, moh_class=$5, whitelist_enabled=$6,
		  work_hours_enabled=$7, work_hours_start=$8, work_hours_end=$9, work_days=$10,
		  updated_at=NOW()
		 WHERE kc_number_id=$11`,
		body.Strategy, body.WaitTimeout, body.QueueTimeout,
		body.MaxCallers, body.MOHClass, body.WhitelistEnabled,
		body.WorkHoursEnabled, body.WorkHoursStart, body.WorkHoursEnd, body.WorkDays, kcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync queue strategy/limits/hold-music class in Asterisk
	if err := asterisk.UpsertKCQueue(h.DB, kcID, body.Strategy, 15, body.MaxCallers, body.MOHClass); err != nil {
		writeError(w, http.StatusInternalServerError, "queue: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validUploadExt checks an uploaded prompt/hold-music file's extension
// against what a browser or admin might reasonably send in — the actual
// on-disk format is normalized by saveConvertedUpload regardless.
func validUploadExt(ext string) bool {
	return ext == ".wav" || ext == ".gsm" || ext == ".mp3" || ext == ".ulaw"
}

// saveConvertedUpload writes an uploaded file under dir as baseName.wav,
// transcoding it to Asterisk's expected format on the way in (8kHz mono
// u-law — see asterisk.ConvertToAsteriskWAV) regardless of what format was
// actually uploaded, so admins never need to pre-convert files themselves.
// Also removes any other-extension leftovers from a previous upload under
// the same baseName (e.g. a pre-conversion-support greeting-5.mp3 sitting
// next to the new greeting-5.wav), so Asterisk's own extension search can't
// pick the stale, unconverted file instead.
func saveConvertedUpload(dir, baseName, ext string, file multipart.File) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create upload dir: %w", err)
	}

	tmpPath := filepath.Join(dir, baseName+".upload"+ext)
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("upload failed: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmpPath)

	destPath := filepath.Join(dir, baseName+".wav")
	if err := asterisk.ConvertToAsteriskWAV(tmpPath, destPath); err != nil {
		return err
	}

	if leftovers, err := filepath.Glob(filepath.Join(dir, baseName+".*")); err == nil {
		for _, f := range leftovers {
			if f != destPath {
				os.Remove(f)
			}
		}
	}
	return nil
}

// UploadGreeting accepts an audio file and stores it for Asterisk, converted
// to 8kHz mono u-law WAV (see saveConvertedUpload).
func (h *IVRHandler) UploadGreeting(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	r.ParseMultipartForm(20 << 20) // 20 MB
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !validUploadExt(ext) {
		writeError(w, http.StatusBadRequest, "supported formats: wav, gsm, mp3, ulaw")
		return
	}

	// Filename Asterisk will use: greeting-{kcNumberID} (without extension)
	dir := filepath.Join(h.UploadsDir, "ivr", fmt.Sprintf("kc-%d", kcID))
	asteriskName := fmt.Sprintf("greeting-%d", kcID)
	if err := saveConvertedUpload(dir, asteriskName, ext, file); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Store relative Asterisk path (without extension)
	asteriskPath := fmt.Sprintf("ivr/kc-%d/%s", kcID, asteriskName)
	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE ivr_configs SET greeting_file=$1, updated_at=NOW() WHERE kc_number_id=$2`,
		asteriskPath, kcID)

	writeJSON(w, http.StatusOK, map[string]string{
		"file":         header.Filename,
		"asteriskPath": asteriskPath,
	})
}

// UploadClosedGreeting accepts an audio file played instead of the regular
// greeting when the call arrives outside work hours (see work_hours_enabled
// and writeKCDialplan's closed-hours branch). Same storage/conversion
// pattern as UploadGreeting, just a distinct filename/column so both can
// coexist independently.
func (h *IVRHandler) UploadClosedGreeting(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	r.ParseMultipartForm(20 << 20) // 20 MB
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !validUploadExt(ext) {
		writeError(w, http.StatusBadRequest, "supported formats: wav, gsm, mp3, ulaw")
		return
	}

	dir := filepath.Join(h.UploadsDir, "ivr", fmt.Sprintf("kc-%d", kcID))
	asteriskName := fmt.Sprintf("closed-greeting-%d", kcID)
	if err := saveConvertedUpload(dir, asteriskName, ext, file); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	asteriskPath := fmt.Sprintf("ivr/kc-%d/%s", kcID, asteriskName)
	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE ivr_configs SET closed_greeting_file=$1, updated_at=NOW() WHERE kc_number_id=$2`,
		asteriskPath, kcID)

	writeJSON(w, http.StatusOK, map[string]string{
		"file":         header.Filename,
		"asteriskPath": asteriskPath,
	})
}

// UploadMOH accepts an audio file to use as this KC number's hold music: it
// creates/updates a dedicated realtime MOH class (see asterisk.UpsertMOHClass)
// named after the number, points ivr_configs.moh_class at it, and applies it
// to the live queue immediately (unlike greeting/menu changes, hold music
// doesn't touch the dialplan, so there's no reason to make the admin also
// click "Apply to Asterisk" just for this). Requires realtime MOH configured
// on the Asterisk server — see the ast_musiconhold comment in migration.sql;
// without that one-time step, this still stores the file and writes the DB
// rows, but Asterisk won't actually pick up the new class.
func (h *IVRHandler) UploadMOH(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	r.ParseMultipartForm(20 << 20) // 20 MB
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !validUploadExt(ext) {
		writeError(w, http.StatusBadRequest, "supported formats: wav, gsm, mp3, ulaw")
		return
	}

	className := asterisk.MOHClassName(kcID)
	relDir := fmt.Sprintf("moh/%s", className)
	dir := filepath.Join(h.UploadsDir, relDir)
	if err := saveConvertedUpload(dir, "hold", ext, file); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := asterisk.UpsertMOHClass(h.DB, className, relDir); err != nil {
		writeError(w, http.StatusInternalServerError, "moh class: "+err.Error())
		return
	}

	var strategy string
	var maxCallers int
	_ = h.DB.QueryRowContext(r.Context(),
		`UPDATE ivr_configs SET moh_class=$1, updated_at=NOW() WHERE kc_number_id=$2 RETURNING strategy, max_callers`,
		className, kcID).Scan(&strategy, &maxCallers)

	if err := asterisk.UpsertKCQueue(h.DB, kcID, strategy, 15, maxCallers, className); err != nil {
		writeError(w, http.StatusInternalServerError, "queue: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"file":     header.Filename,
		"mohClass": className,
	})
}

// listOptions returns IVR menu options for a KC number.
func (h *IVRHandler) listOptions(r *http.Request, kcID int) ([]IVROption, error) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, kc_number_id, digit, label, action, action_data, sort_order
		 FROM ivr_options WHERE kc_number_id=$1 ORDER BY sort_order, digit`, kcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []IVROption{}
	for rows.Next() {
		var o IVROption
		if err := rows.Scan(&o.ID, &o.KCNumberID, &o.Digit, &o.Label, &o.Action, &o.ActionData, &o.SortOrder); err != nil {
			continue
		}
		result = append(result, o)
	}
	return result, nil
}

// ListOptions returns IVR menu options.
func (h *IVRHandler) ListOptions(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts, err := h.listOptions(r, kcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": opts})
}

// SaveOption creates or updates an IVR option (upsert by digit).
func (h *IVRHandler) SaveOption(w http.ResponseWriter, r *http.Request) {
	kcID, tenantID, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Digit      string `json:"digit"`
		Label      string `json:"label"`
		Action     string `json:"action"`
		ActionData string `json:"actionData"`
		SortOrder  int    `json:"sortOrder"`
	}
	if err := decode(r, &body); err != nil || body.Digit == "" {
		writeError(w, http.StatusBadRequest, "digit required")
		return
	}

	var id int
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO ivr_options (tenant_id, kc_number_id, digit, label, action, action_data, sort_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (kc_number_id, digit) DO UPDATE
		   SET label=EXCLUDED.label, action=EXCLUDED.action,
		       action_data=EXCLUDED.action_data, sort_order=EXCLUDED.sort_order
		 RETURNING id`,
		tenantID, kcID, body.Digit, body.Label, body.Action, body.ActionData, body.SortOrder,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"id": id})
}

// DeleteOption removes an IVR option by digit.
func (h *IVRHandler) DeleteOption(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	digit := chi.URLParam(r, "digit")
	_, err = h.DB.ExecContext(r.Context(), `DELETE FROM ivr_options WHERE kc_number_id=$1 AND digit=$2`, kcID, digit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listMembers returns queue members for a KC number.
func (h *IVRHandler) listMembers(r *http.Request, tenantID, kcID int) ([]IVRMember, error) {
	members, err := asterisk.ListKCQueueMembers(h.DB, tenantID, kcID)
	if err != nil {
		return nil, err
	}
	result := make([]IVRMember, 0, len(members))
	for _, m := range members {
		result = append(result, IVRMember{
			Username:  m.Username,
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Interface: "PJSIP/" + m.Username,
			Paused:    m.Paused,
		})
	}
	return result, nil
}

// ListMembers returns queue members.
func (h *IVRHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	kcID, tenantID, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	members, err := h.listMembers(r, tenantID, kcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// AddMember adds a user to the KC number's queue.
func (h *IVRHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	kcID, tenantID, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Username string `json:"username"`
	}
	if err := decode(r, &body); err != nil || body.Username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}

	if err := asterisk.AddMemberToKCQueue(h.DB, tenantID, kcID, body.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember removes a user from the KC number's queue.
func (h *IVRHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	kcID, tenantID, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	username := chi.URLParam(r, "username")
	if err := asterisk.RemoveMemberFromKCQueue(h.DB, tenantID, kcID, username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sync applies the current IVR config to Asterisk (writes ast_extensions + updates ast_queues).
func (h *IVRHandler) Sync(w http.ResponseWriter, r *http.Request) {
	kcID, _, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cfg IVRConfig
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT greeting_file, wait_timeout, queue_timeout, strategy, max_callers,
		        closed_greeting_file, work_hours_enabled, work_hours_start, work_hours_end, work_days,
		        whitelist_enabled, moh_class
		 FROM ivr_configs WHERE kc_number_id=$1`, kcID,
	).Scan(&cfg.GreetingFile, &cfg.WaitTimeout, &cfg.QueueTimeout, &cfg.Strategy, &cfg.MaxCallers,
		&cfg.ClosedGreetingFile, &cfg.WorkHoursEnabled, &cfg.WorkHoursStart, &cfg.WorkHoursEnd, &cfg.WorkDays,
		&cfg.WhitelistEnabled, &cfg.MOHClass)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	options, err := h.listOptions(r, kcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build dialplan options
	queueName := asterisk.KCQueueName(kcID)
	var dpOptions []asterisk.IVRDialplanOption
	for _, opt := range options {
		var app, appData string
		switch opt.Action {
		case "queue":
			app = "Queue"
			if opt.ActionData != "" {
				appData = opt.ActionData
			} else {
				// 'c': continue in the dialplan instead of hanging up the
				// caller when the connected agent's leg disappears mid-call
				// — see the "Queue" case in writeKCDialplan's options loop,
				// which follows this with a Wait() to hold for reconnect.
				appData = fmt.Sprintf("%s,rHhc,,%d", queueName, cfg.QueueTimeout)
			}
		case "extension":
			app = "Dial"
			appData = fmt.Sprintf("PJSIP/%s,30,rU", opt.ActionData)
		case "playback":
			app = "Playback"
			appData = opt.ActionData
		case "hangup":
			app = "Hangup"
			appData = ""
		default:
			app = "Queue"
			appData = fmt.Sprintf("%s,rHhc,,%d", queueName, cfg.QueueTimeout)
		}
		dpOptions = append(dpOptions, asterisk.IVRDialplanOption{
			Digit: opt.Digit, App: app, AppData: appData,
		})
	}

	// Sync dialplan
	wh := asterisk.WorkHours{
		Enabled:        cfg.WorkHoursEnabled,
		Start:          cfg.WorkHoursStart,
		End:            cfg.WorkHoursEnd,
		Days:           cfg.WorkDays,
		ClosedGreeting: cfg.ClosedGreetingFile,
	}
	if err := asterisk.SyncKCNumberDialplan(h.DB, kcID, cfg.GreetingFile, cfg.WaitTimeout, cfg.QueueTimeout, dpOptions, wh, cfg.WhitelistEnabled); err != nil {
		writeError(w, http.StatusInternalServerError, "dialplan: "+err.Error())
		return
	}

	// Sync queue
	if err := asterisk.UpsertKCQueue(h.DB, kcID, cfg.Strategy, 15, cfg.MaxCallers, cfg.MOHClass); err != nil {
		writeError(w, http.StatusInternalServerError, "queue: "+err.Error())
		return
	}

	var providerContext string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT p.name FROM kc_numbers kn JOIN providers p ON p.id = kn.provider_id WHERE kn.id = $1`, kcID,
	).Scan(&providerContext)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"context": providerContext,
		"queue":   queueName,
	})
}

// ResyncAllDialplans rewrites every KC number's inbound dialplan + queue
// settings from its current ivr_configs/ivr_options — the bulk, no-HTTP-
// request counterpart of IVRHandler.Sync (see resyncOneDialplan below, which
// mirrors Sync's config-to-dialplan mapping exactly). Meant to run once at
// startup (see cmd/server/main.go): whenever how that dialplan gets
// generated changes (e.g. addRecording gaining its Set(CDR(userfield)=...)
// line so recordings become linkable in the UI), any KC number nobody has
// happened to resync since keeps running its old dialplan otherwise —
// MixMonitor still writes the file to MinIO fine, but ast_cdr.userfield
// never gets set for it, so CDRHandler has no way to find it. Idempotent
// (SyncKCNumberDialplan/UpsertKCQueue both DELETE+rewrite or UPDATE-or-INSERT),
// so safe to run on every startup. Best-effort per number: one bad
// ivr_configs row shouldn't block the rest from getting resynced.
func ResyncAllDialplans(db *sql.DB) error {
	rows, err := db.Query(`SELECT id FROM kc_numbers`)
	if err != nil {
		return fmt.Errorf("list kc numbers: %w", err)
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	rows.Close()

	for _, kcID := range ids {
		if err := resyncOneDialplan(db, kcID); err != nil {
			log.Printf("[IVR] resync kc number %d: %v", kcID, err)
		}
	}
	return nil
}

func resyncOneDialplan(db *sql.DB, kcID int) error {
	var cfg IVRConfig
	err := db.QueryRow(
		`SELECT greeting_file, wait_timeout, queue_timeout, strategy, max_callers,
		        closed_greeting_file, work_hours_enabled, work_hours_start, work_hours_end, work_days,
		        whitelist_enabled, moh_class
		 FROM ivr_configs WHERE kc_number_id=$1`, kcID,
	).Scan(&cfg.GreetingFile, &cfg.WaitTimeout, &cfg.QueueTimeout, &cfg.Strategy, &cfg.MaxCallers,
		&cfg.ClosedGreetingFile, &cfg.WorkHoursEnabled, &cfg.WorkHoursStart, &cfg.WorkHoursEnd, &cfg.WorkDays,
		&cfg.WhitelistEnabled, &cfg.MOHClass)
	if err != nil {
		return fmt.Errorf("load ivr_configs: %w", err)
	}

	optRows, err := db.Query(
		`SELECT digit, action, action_data FROM ivr_options WHERE kc_number_id=$1 ORDER BY sort_order, digit`, kcID)
	if err != nil {
		return fmt.Errorf("load ivr_options: %w", err)
	}
	type rawOpt struct{ digit, action, actionData string }
	var opts []rawOpt
	for optRows.Next() {
		var o rawOpt
		if err := optRows.Scan(&o.digit, &o.action, &o.actionData); err != nil {
			continue
		}
		opts = append(opts, o)
	}
	optRows.Close()

	// Same action → dialplan app/appdata mapping as Sync above.
	queueName := asterisk.KCQueueName(kcID)
	var dpOptions []asterisk.IVRDialplanOption
	for _, opt := range opts {
		var app, appData string
		switch opt.action {
		case "queue":
			app = "Queue"
			if opt.actionData != "" {
				appData = opt.actionData
			} else {
				appData = fmt.Sprintf("%s,rHhc,,%d", queueName, cfg.QueueTimeout)
			}
		case "extension":
			app = "Dial"
			appData = fmt.Sprintf("PJSIP/%s,30,rU", opt.actionData)
		case "playback":
			app = "Playback"
			appData = opt.actionData
		case "hangup":
			app = "Hangup"
			appData = ""
		default:
			app = "Queue"
			appData = fmt.Sprintf("%s,rHhc,,%d", queueName, cfg.QueueTimeout)
		}
		dpOptions = append(dpOptions, asterisk.IVRDialplanOption{Digit: opt.digit, App: app, AppData: appData})
	}

	wh := asterisk.WorkHours{
		Enabled:        cfg.WorkHoursEnabled,
		Start:          cfg.WorkHoursStart,
		End:            cfg.WorkHoursEnd,
		Days:           cfg.WorkDays,
		ClosedGreeting: cfg.ClosedGreetingFile,
	}
	if err := asterisk.SyncKCNumberDialplan(db, kcID, cfg.GreetingFile, cfg.WaitTimeout, cfg.QueueTimeout, dpOptions, wh, cfg.WhitelistEnabled); err != nil {
		return fmt.Errorf("dialplan: %w", err)
	}
	if err := asterisk.UpsertKCQueue(db, kcID, cfg.Strategy, 15, cfg.MaxCallers, cfg.MOHClass); err != nil {
		return fmt.Errorf("queue: %w", err)
	}
	return nil
}

// GetAvailableUsers returns tenant users not yet in this KC number's queue.
func (h *IVRHandler) GetAvailableUsers(w http.ResponseWriter, r *http.Request) {
	kcID, tenantID, err := h.kcNumberID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	users, err := asterisk.ListKCAvailableUsers(h.DB, tenantID, kcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}
