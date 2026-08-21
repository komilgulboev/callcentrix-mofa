package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	mw "callcentrix/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/minio/minio-go/v7"
)

// kbBodyPolicy sanitizes article body HTML on the way in (see
// sanitizeArticleBody) — UGCPolicy allows the formatting tags a rich-text
// editor's output actually needs (p, strong/em, lists, headings, links,
// img[src,alt], ...) while stripping anything a malicious or compromised
// TenantAdmin account could use to attack their own tenant's Operators, who
// read this exact HTML back verbatim via dangerouslySetInnerHTML (see
// KnowledgeBaseArticle.jsx) — script tags, on* handlers, javascript: URLs, etc.
var kbBodyPolicy = bluemonday.UGCPolicy()

func sanitizeArticleBody(html string) string {
	return kbBodyPolicy.Sanitize(html)
}

// KnowledgeBaseHandler.Minio/Bucket store article photos/videos in the same
// private MinIO bucket call recordings use (cfg.MinioBucket), under a "kb/"
// key prefix — not local disk, so this backend's uploads scale the same way
// across a multi-server Asterisk deployment without needing shared storage.
type KnowledgeBaseHandler struct {
	DB     *sql.DB
	Minio  *minio.Client // nil if MinIO isn't configured — media endpoints then 503
	Bucket string
}

// KBCategory is a shared, global taxonomy entry curated by SuperAdmin (see
// DeleteCategory for why it's soft-deleted rather than hard-deleted, unlike
// the tenant-scoped topic_catalog it's otherwise modeled on).
type KBCategory struct {
	ID           int               `json:"id"`
	Names        map[string]string `json:"names"`
	Active       bool              `json:"active"`
	ArticleCount int               `json:"articleCount"`
}

// KBMediaRef is one photo/video attached to an article — just enough for the
// frontend to build a URL (ArticleMedia) and pick <img> vs <video>.
type KBMediaRef struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}

type KBArticle struct {
	ID             int          `json:"id"`
	TenantID       int          `json:"tenantId"`
	CategoryID     *int         `json:"categoryId"`
	Title          string       `json:"title"`
	Body           string       `json:"body"`
	Tags           []string     `json:"tags"`
	Media          []KBMediaRef `json:"media"`
	VisibleToAll   bool         `json:"visibleToAll"`
	AllowedUserIDs []int        `json:"allowedUserIds"`
	CreatedBy      int          `json:"createdBy"`
	CreatedByName  string       `json:"createdByName"`
	CreatedAt      string       `json:"createdAt"`
	UpdatedAt      string       `json:"updatedAt"`
}

// ListCategories returns every active category plus, for the caller's own
// tenant (or ?tenantId= for SuperAdmin), how many of that tenant's articles
// are filed under each one — powers the landing page's category cards.
func (h *KnowledgeBaseHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var tenantID int
	if c.TenantID != nil {
		tenantID = *c.TenantID
	} else if tid, err := strconv.Atoi(r.URL.Query().Get("tenantId")); err == nil {
		tenantID = tid
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT c.id, c.names, c.active,
		        (SELECT COUNT(*) FROM kb_articles a WHERE a.category_id = c.id AND a.tenant_id = $1)
		 FROM kb_categories c WHERE c.active = TRUE ORDER BY c.id`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []KBCategory{}
	for rows.Next() {
		var cat KBCategory
		var namesJSON []byte
		if err := rows.Scan(&cat.ID, &namesJSON, &cat.Active, &cat.ArticleCount); err != nil {
			continue
		}
		if err := json.Unmarshal(namesJSON, &cat.Names); err != nil || cat.Names == nil {
			cat.Names = map[string]string{}
		}
		result = append(result, cat)
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": result})
}

func (h *KnowledgeBaseHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Names map[string]string `json:"names"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names required")
		return
	}
	namesJSON, err := json.Marshal(body.Names)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid names")
		return
	}

	var id int
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO kb_categories (names) VALUES ($1) RETURNING id`, namesJSON,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *KnowledgeBaseHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var body struct {
		Names map[string]string `json:"names"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names required")
		return
	}
	namesJSON, err := json.Marshal(body.Names)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid names")
		return
	}

	result, err := h.DB.ExecContext(r.Context(),
		`UPDATE kb_categories SET names=$1 WHERE id=$2`, namesJSON, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteCategory soft-deletes (active=false) rather than hard-deleting: unlike
// topic_catalog (tenant-scoped, so Topics.Delete can safely hard-delete),
// kb_categories is shared by every tenant — a hard delete would orphan every
// tenant's articles filed under it at once. See migration.sql's kb_categories
// comment for the full rationale.
func (h *KnowledgeBaseHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_, err := h.DB.ExecContext(r.Context(), `UPDATE kb_categories SET active=FALSE WHERE id=$1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// kbTagsJSONExpr aggregates an article's hashtags into a JSON array, used by
// both ListArticles (many articles) and GetArticle (one) — same shape as
// tasks.go's assigneesJSONExpr.
const kbTagsJSONExpr = `COALESCE((
	SELECT json_agg(tag ORDER BY tag) FROM kb_article_tags WHERE article_id = a.id
), '[]')`

// kbMediaJSONExpr aggregates an article's attached photos/videos — the
// frontend builds each item's URL from articleId+mediaId (see ArticleMedia).
const kbMediaJSONExpr = `COALESCE((
	SELECT json_agg(json_build_object('id', id, 'type', media_type) ORDER BY id)
	FROM kb_article_media WHERE article_id = a.id
), '[]')`

// kbAllowedUsersJSONExpr aggregates the explicit allow-list for an article
// that isn't visible to the whole tenant (see visible_to_all).
const kbAllowedUsersJSONExpr = `COALESCE((
	SELECT json_agg(user_id ORDER BY user_id) FROM kb_article_users WHERE article_id = a.id
), '[]')`

func scanKBArticle(scan func(dest ...any) error) (KBArticle, error) {
	var a KBArticle
	var categoryID sql.NullInt64
	var tagsJSON, mediaJSON, allowedUsersJSON []byte
	var creatorName sql.NullString
	err := scan(&a.ID, &a.TenantID, &categoryID, &a.Title, &a.Body,
		&tagsJSON, &mediaJSON, &a.VisibleToAll, &allowedUsersJSON,
		&a.CreatedBy, &creatorName, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if categoryID.Valid {
		cid := int(categoryID.Int64)
		a.CategoryID = &cid
	}
	a.Tags = []string{}
	_ = json.Unmarshal(tagsJSON, &a.Tags)
	a.Media = []KBMediaRef{}
	_ = json.Unmarshal(mediaJSON, &a.Media)
	a.AllowedUserIDs = []int{}
	_ = json.Unmarshal(allowedUsersJSON, &a.AllowedUserIDs)
	a.CreatedByName = creatorName.String
	return a, nil
}

const kbArticleSelect = `SELECT a.id, a.tenant_id, a.category_id, a.title, a.body,
	       ` + kbTagsJSONExpr + `,
	       ` + kbMediaJSONExpr + `,
	       a.visible_to_all,
	       ` + kbAllowedUsersJSONExpr + `,
	       a.created_by, COALESCE(NULLIF(TRIM(CONCAT(cu.first_name,' ',cu.last_name)), ''), cu.username),
	       a.created_at, a.updated_at
	FROM kb_articles a
	LEFT JOIN users cu ON cu.id = a.created_by`

// kbVisibilityFilter appends the "can this Supervisor/Operator see this
// article" check — TenantAdmin/SuperAdmin bypass it entirely (they already
// see every article in scope for management purposes). userID is the
// caller's own id ($N placeholder n).
func kbVisibilityFilter(userType, userID, n int) (clause string, arg any, ok bool) {
	if userType != 2 && userType != 3 {
		return "", nil, false
	}
	return fmt.Sprintf(` AND (a.visible_to_all = TRUE OR EXISTS (
		SELECT 1 FROM kb_article_users au WHERE au.article_id = a.id AND au.user_id = $%d
	))`, n), userID, true
}

// ListArticles is tenant-scoped: non-SuperAdmin callers always see only
// their own tenant's articles; SuperAdmin (no tenant of their own) must pass
// ?tenantId= or gets an empty list — mirrors TasksHandler.List/AssignableUsers.
// Supervisor/Operator are further filtered by kbVisibilityFilter.
func (h *KnowledgeBaseHandler) ListArticles(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()

	var tenantID int
	hasTenant := false
	if c.TenantID != nil {
		tenantID = *c.TenantID
		hasTenant = true
	} else if tidStr := q.Get("tenantId"); tidStr != "" {
		if tid, err := strconv.Atoi(tidStr); err == nil {
			tenantID = tid
			hasTenant = true
		}
	}
	if !hasTenant {
		writeJSON(w, http.StatusOK, map[string]any{"articles": []KBArticle{}})
		return
	}

	query := kbArticleSelect + ` WHERE a.tenant_id = $1`
	args := []any{tenantID}
	n := 2

	if clause, arg, ok := kbVisibilityFilter(c.UserType, c.Sub, n); ok {
		query += clause
		args = append(args, arg)
		n++
	}
	if catStr := q.Get("categoryId"); catStr != "" {
		if cat, err := strconv.Atoi(catStr); err == nil {
			query += ` AND a.category_id = $` + strconv.Itoa(n)
			args = append(args, cat)
			n++
		}
	}
	if tag := q.Get("tag"); tag != "" {
		query += ` AND EXISTS (SELECT 1 FROM kb_article_tags t WHERE t.article_id = a.id AND t.tag = $` + strconv.Itoa(n) + `)`
		args = append(args, strings.ToLower(strings.TrimSpace(tag)))
		n++
	}
	if search := q.Get("search"); search != "" {
		query += ` AND a.title ILIKE $` + strconv.Itoa(n)
		args = append(args, "%"+search+"%")
		n++
	}
	query += ` ORDER BY a.updated_at DESC LIMIT 500`

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []KBArticle{}
	for rows.Next() {
		a, err := scanKBArticle(rows.Scan)
		if err != nil {
			continue
		}
		result = append(result, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"articles": result})
}

func (h *KnowledgeBaseHandler) GetArticle(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	row := h.DB.QueryRowContext(r.Context(), kbArticleSelect+` WHERE a.id = $1`, id)
	a, err := scanKBArticle(row.Scan)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != a.TenantID) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if (c.UserType == 2 || c.UserType == 3) && !a.VisibleToAll {
		allowed := false
		for _, uid := range a.AllowedUserIDs {
			if uid == c.Sub {
				allowed = true
				break
			}
		}
		if !allowed {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, a)
}

// normalizeTags trims/lowercases/dedupes freeform hashtag input, stripping a
// leading '#' if the admin typed one — e.g. "#Оплата, faq" -> ["оплата","faq"].
func normalizeTags(raw []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range raw {
		t = strings.TrimSpace(t)
		t = strings.TrimPrefix(t, "#")
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func insertArticleTags(ctx context.Context, tx *sql.Tx, articleID int, raw []string) error {
	for _, tag := range normalizeTags(raw) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kb_article_tags (article_id, tag) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			articleID, tag); err != nil {
			return err
		}
	}
	return nil
}

// insertArticleUsers writes the explicit allow-list for an article that
// isn't visible to the whole tenant — deduped, silently skipping non-positive
// ids (defensive only; the frontend never sends those).
func insertArticleUsers(ctx context.Context, tx *sql.Tx, articleID int, userIDs []int) error {
	seen := map[int]bool{}
	for _, uid := range userIDs {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kb_article_users (article_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			articleID, uid); err != nil {
			return err
		}
	}
	return nil
}

type kbArticleBody struct {
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	CategoryID     *int     `json:"categoryId"`
	Tags           []string `json:"tags"`
	VisibleToAll   bool     `json:"visibleToAll"`
	AllowedUserIDs []int    `json:"allowedUserIds"`
}

// CreateArticle is restricted to TenantAdmin (not SuperAdmin, who has no
// tenant to attach an article to, and not Supervisor) — confirmed with the
// user as the intended authorship model. The route group itself only
// enforces RequireRole(1) (SuperAdmin+TenantAdmin), so this explicit
// UserType check is what actually excludes SuperAdmin.
func (h *KnowledgeBaseHandler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	if c.UserType != 1 || c.TenantID == nil {
		writeError(w, http.StatusForbidden, "only tenant admins can create articles")
		return
	}

	var body kbArticleBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	body.Body = sanitizeArticleBody(body.Body)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var id int
	if err := tx.QueryRowContext(r.Context(),
		`INSERT INTO kb_articles (tenant_id, category_id, title, body, created_by, visible_to_all)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		*c.TenantID, body.CategoryID, body.Title, body.Body, c.Sub, body.VisibleToAll,
	).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := insertArticleTags(r.Context(), tx, id, body.Tags); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := insertArticleUsers(r.Context(), tx, id, body.AllowedUserIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *KnowledgeBaseHandler) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if c.UserType != 1 || c.TenantID == nil {
		writeError(w, http.StatusForbidden, "only tenant admins can edit articles")
		return
	}

	if err := h.checkArticleOwnership(r.Context(), id, *c.TenantID); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var body kbArticleBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	body.Body = sanitizeArticleBody(body.Body)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		`UPDATE kb_articles SET category_id=$1, title=$2, body=$3, visible_to_all=$4, updated_at=NOW() WHERE id=$5`,
		body.CategoryID, body.Title, body.Body, body.VisibleToAll, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Tags and the allowed-users list are fully replaced (not merged) —
	// simplest correct semantics for form fields that always re-submit their
	// whole current value.
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM kb_article_tags WHERE article_id=$1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := insertArticleTags(r.Context(), tx, id, body.Tags); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM kb_article_users WHERE article_id=$1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := insertArticleUsers(r.Context(), tx, id, body.AllowedUserIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkArticleOwnership returns nil iff articleID exists and belongs to
// tenantID — shared by UpdateArticle and the media handlers below.
func (h *KnowledgeBaseHandler) checkArticleOwnership(ctx context.Context, articleID, tenantID int) error {
	var ownerTenant int
	if err := h.DB.QueryRowContext(ctx, `SELECT tenant_id FROM kb_articles WHERE id=$1`, articleID).Scan(&ownerTenant); err != nil {
		return err
	}
	if ownerTenant != tenantID {
		return sql.ErrNoRows
	}
	return nil
}

func (h *KnowledgeBaseHandler) DeleteArticle(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if c.UserType != 1 || c.TenantID == nil {
		writeError(w, http.StatusForbidden, "only tenant admins can delete articles")
		return
	}

	// Collect object keys before the row (and its ON DELETE CASCADE children)
	// disappear — kb_article_media rows are gone once kb_articles is deleted,
	// but the MinIO objects they pointed at aren't, so remove those separately.
	var objectKeys []string
	if h.Minio != nil {
		if rows, err := h.DB.QueryContext(r.Context(),
			`SELECT object_key FROM kb_article_media WHERE article_id=$1 AND object_key <> ''`, id); err == nil {
			for rows.Next() {
				var k string
				if rows.Scan(&k) == nil {
					objectKeys = append(objectKeys, k)
				}
			}
			rows.Close()
		}
	}

	result, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM kb_articles WHERE id=$1 AND tenant_id=$2`, id, *c.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	for _, key := range objectKeys {
		_ = h.Minio.RemoveObject(r.Context(), h.Bucket, key, minio.RemoveObjectOptions{})
	}
	w.WriteHeader(http.StatusNoContent)
}

// kbMediaContentType validates ext against the claimed mediaType and returns
// the MIME type to store/serve, or "" if unsupported.
func kbMediaContentType(mediaType, ext string) string {
	if mediaType == "photo" {
		switch ext {
		case ".png":
			return "image/png"
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".webp":
			return "image/webp"
		case ".gif":
			return "image/gif"
		}
		return ""
	}
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".ogg":
		return "video/ogg"
	}
	return ""
}

// UploadArticleMedia attaches a photo or video to an article — TenantAdmin +
// ownership, same gating as UpdateArticle. Stored in MinIO under object key
// "kb/<articleId>/<mediaId><ext>" (see the KnowledgeBaseHandler doc comment);
// the DB row is inserted first so its SERIAL id can double as a
// collision-free object key component.
func (h *KnowledgeBaseHandler) UploadArticleMedia(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	articleID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if c.UserType != 1 || c.TenantID == nil {
		writeError(w, http.StatusForbidden, "only tenant admins can upload media")
		return
	}
	if err := h.checkArticleOwnership(r.Context(), articleID, *c.TenantID); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if h.Minio == nil {
		writeError(w, http.StatusServiceUnavailable, "media storage not configured")
		return
	}

	if err := r.ParseMultipartForm(200 << 20); err != nil { // 200 MB — video-sized
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	mediaType := r.FormValue("type")
	if mediaType != "photo" && mediaType != "video" {
		writeError(w, http.StatusBadRequest, "type must be photo or video")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	contentType := kbMediaContentType(mediaType, ext)
	if contentType == "" {
		if mediaType == "photo" {
			writeError(w, http.StatusBadRequest, "supported photo formats: png, jpg, jpeg, webp, gif")
		} else {
			writeError(w, http.StatusBadRequest, "supported video formats: mp4, webm, mov, ogg")
		}
		return
	}

	var mediaID int
	if err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO kb_article_media (article_id, media_type, content_type) VALUES ($1,$2,$3) RETURNING id`,
		articleID, mediaType, contentType).Scan(&mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	objectKey := fmt.Sprintf("kb/%d/%d%s", articleID, mediaID, ext)
	if _, err := h.Minio.PutObject(r.Context(), h.Bucket, objectKey, file, header.Size,
		minio.PutObjectOptions{ContentType: contentType}); err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed: "+err.Error())
		return
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE kb_article_media SET object_key=$1 WHERE id=$2`, objectKey, mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": mediaID, "type": mediaType})
}

// DeleteArticleMedia removes one attached photo/video — TenantAdmin + ownership.
func (h *KnowledgeBaseHandler) DeleteArticleMedia(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	articleID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	mediaID, _ := strconv.Atoi(chi.URLParam(r, "mediaId"))
	if c.UserType != 1 || c.TenantID == nil {
		writeError(w, http.StatusForbidden, "only tenant admins can delete media")
		return
	}
	if err := h.checkArticleOwnership(r.Context(), articleID, *c.TenantID); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var objectKey string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT object_key FROM kb_article_media WHERE id=$1 AND article_id=$2`, mediaID, articleID).Scan(&objectKey)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM kb_article_media WHERE id=$1`, mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Minio != nil && objectKey != "" {
		_ = h.Minio.RemoveObject(r.Context(), h.Bucket, objectKey, minio.RemoveObjectOptions{})
	}
	w.WriteHeader(http.StatusNoContent)
}

// ArticleMedia streams one attached photo/video from MinIO. Auth-gated
// (unlike settings.go's public Logo) since KB articles are private per
// tenant — the caller's own tenant must match the owning article's, same
// check as GetArticle. Reached either via normal fetch (Authorization
// header) or an <img>/<video> tag using the ?token= query param mw.Auth
// already supports (see extractToken). http.ServeContent (via
// minio.Object's io.ReadSeeker) is what makes video seeking work.
func (h *KnowledgeBaseHandler) ArticleMedia(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	articleID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	mediaID, _ := strconv.Atoi(chi.URLParam(r, "mediaId"))

	var tenantID int
	var objectKey, contentType string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT a.tenant_id, m.object_key, m.content_type FROM kb_article_media m
		 JOIN kb_articles a ON a.id = m.article_id
		 WHERE m.id = $1 AND m.article_id = $2`, mediaID, articleID,
	).Scan(&tenantID, &objectKey, &contentType)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		http.NotFound(w, r)
		return
	}
	if h.Minio == nil {
		writeError(w, http.StatusServiceUnavailable, "media storage not configured")
		return
	}

	obj, err := h.Minio.GetObject(r.Context(), h.Bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	defer obj.Close()
	stat, err := obj.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, objectKey, stat.LastModified, obj)
}

// ListTags returns the distinct hashtags in use across the caller's own
// tenant's articles (or ?tenantId= for SuperAdmin) — powers the search box's
// tag suggestions on the frontend.
func (h *KnowledgeBaseHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var tenantID int
	hasTenant := false
	if c.TenantID != nil {
		tenantID = *c.TenantID
		hasTenant = true
	} else if tid, err := strconv.Atoi(r.URL.Query().Get("tenantId")); err == nil {
		tenantID = tid
		hasTenant = true
	}
	if !hasTenant {
		writeJSON(w, http.StatusOK, map[string]any{"tags": []string{}})
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT DISTINCT t.tag FROM kb_article_tags t
		 JOIN kb_articles a ON a.id = t.article_id
		 WHERE a.tenant_id = $1 ORDER BY t.tag`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			continue
		}
		result = append(result, tag)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": result})
}
