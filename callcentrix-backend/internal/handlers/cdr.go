package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

type CDRHandler struct {
	DB           *sql.DB
	RecordingURL string
}

type CDRRecord struct {
	ID          int    `json:"id"`
	CallDate    string `json:"callDate"`
	Clid        string `json:"clid"`
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	Dcontext    string `json:"dcontext"`
	Channel     string `json:"channel"`
	DstChannel  string `json:"dstChannel"`
	Duration    int    `json:"duration"`
	Billsec     int    `json:"billsec"`
	Disposition string `json:"disposition"`
	AccountCode string `json:"accountCode"`
	UniqueID    string `json:"uniqueId"`
	UserField   string `json:"userField"`
	Recording   bool   `json:"recording"`
}

func (h *CDRHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()

	dateFrom   := q.Get("date_from")
	dateTo     := q.Get("date_to")
	disposition := q.Get("disposition")
	search     := q.Get("search")
	limitStr   := q.Get("limit")
	limit      := 500
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	query := `SELECT id, calldate, clid, src, dst, dcontext, channel, dstchannel,
	                 duration, billsec, disposition, accountcode, uniqueid, userfield
	          FROM ast_cdr WHERE 1=1`
	args := []any{}
	n := 1

	// Operator: only their own calls
	if c.UserType == 3 {
		query += ` AND (src = $` + strconv.Itoa(n) + ` OR dst = $` + strconv.Itoa(n) + `)`
		args = append(args, c.Username)
		n++
	} else if c.UserType != 0 && c.TenantID != nil {
		// TenantAdmin / Supervisor: all calls of their tenant
		query += ` AND (accountcode = $` + strconv.Itoa(n) + ` OR src IN (
			SELECT sip_no FROM users WHERE tenant_id = $` + strconv.Itoa(n) + ` AND sip_no != ''
		) OR dst IN (
			SELECT sip_no FROM users WHERE tenant_id = $` + strconv.Itoa(n) + ` AND sip_no != ''
		))`
		args = append(args, strconv.Itoa(*c.TenantID))
		n++
	}
	if dateFrom != "" {
		query += ` AND calldate >= $` + strconv.Itoa(n)
		args = append(args, dateFrom)
		n++
	}
	if dateTo != "" {
		query += ` AND calldate < ($` + strconv.Itoa(n) + `::date + interval '1 day')`
		args = append(args, dateTo)
		n++
	}
	if disposition != "" {
		query += ` AND disposition = $` + strconv.Itoa(n)
		args = append(args, disposition)
		n++
	}
	if search != "" {
		query += ` AND (src ILIKE $` + strconv.Itoa(n) + ` OR dst ILIKE $` + strconv.Itoa(n) + `)`
		args = append(args, "%"+search+"%")
		n++
	}
	query += ` ORDER BY calldate DESC LIMIT $` + strconv.Itoa(n)
	args = append(args, limit)

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []CDRRecord{}
	for rows.Next() {
		var rec CDRRecord
		if err := rows.Scan(&rec.ID, &rec.CallDate, &rec.Clid, &rec.Src, &rec.Dst,
			&rec.Dcontext, &rec.Channel, &rec.DstChannel,
			&rec.Duration, &rec.Billsec, &rec.Disposition,
			&rec.AccountCode, &rec.UniqueID, &rec.UserField); err != nil {
			continue
		}
		rec.Recording = rec.UserField != ""
		result = append(result, rec)
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": result, "total": len(result)})
}

func (h *CDRHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var rec CDRRecord
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, calldate, clid, src, dst, dcontext, channel, dstchannel,
		        duration, billsec, disposition, accountcode, uniqueid, userfield
		 FROM ast_cdr WHERE id = $1`, id,
	).Scan(&rec.ID, &rec.CallDate, &rec.Clid, &rec.Src, &rec.Dst,
		&rec.Dcontext, &rec.Channel, &rec.DstChannel,
		&rec.Duration, &rec.Billsec, &rec.Disposition,
		&rec.AccountCode, &rec.UniqueID, &rec.UserField)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rec.Recording = rec.UserField != ""
	writeJSON(w, http.StatusOK, rec)
}

func (h *CDRHandler) Audio(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var userField string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT userfield FROM ast_cdr WHERE id = $1`, id).Scan(&userField)
	if err != nil || userField == "" {
		writeError(w, http.StatusNotFound, "no recording")
		return
	}
	http.Redirect(w, r, h.RecordingURL+"/"+userField, http.StatusFound)
}
