package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
	"callcentrix/internal/config"
	"callcentrix/internal/db"
	"callcentrix/internal/handlers"
	mw "callcentrix/internal/middleware"
	"callcentrix/internal/ws"
)

// Overridden at build time via:
//   go build -ldflags "-X main.buildCommit=<hash> -X main.buildTime=<ts>"
var (
	buildCommit = "dev"
	buildTime   = "unknown"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg.DBDSN)
	if err != nil { log.Fatalf("DB connect: %v", err) }
	if err := db.Migrate(database); err != nil { log.Fatalf("DB migrate: %v", err) }

	if err := asterisk.EnsureBlacklistCheckSubroutine(database); err != nil {
		log.Printf("[Asterisk] blacklist-check subroutine warning: %v", err)
	}
	if err := asterisk.EnsureWhitelistCheckSubroutine(database); err != nil {
		log.Printf("[Asterisk] whitelist-check subroutine warning: %v", err)
	}
	if err := asterisk.EnsureBlockedContext(database); err != nil {
		log.Printf("[Asterisk] blocked context warning: %v", err)
	}

	// Fallback AMI client for the single default server (cfg.AMIAddr) — used
	// by users with no server_id assigned, and as the base of the multi-server
	// registry below. See internal/ami/registry.go and asterisk_servers table.
	fallbackClient := ami.NewClient(cfg.AMIAddr, cfg.AMIUser, cfg.AMIPass)
	amiRegistry := ami.NewRegistry(fallbackClient)
	monitor := ami.NewMonitor()
	monitor.Attach(fallbackClient)
	go fallbackClient.ConnectWithRetry()

	// Attach one AMI client per configured Asterisk server, if any.
	if servers, err := asterisk.ListServers(database); err != nil {
		log.Printf("[Asterisk] list servers warning: %v", err)
	} else {
		for _, s := range servers {
			if !s.Active {
				continue
			}
			c := ami.NewClient(s.AMIHost, s.AMIUser, s.AMIPass)
			amiRegistry.Set(s.ID, c)
			monitor.Attach(c)
			go c.ConnectWithRetry()
		}
	}

	hub := ws.NewHub()
	go hub.Run()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C { hub.Broadcast(monitor.Snapshot()) }
	}()

	go func() {
		time.Sleep(3 * time.Second)
		monitor.RefreshFromAMI()
		refreshTicker := time.NewTicker(30 * time.Second)
		for range refreshTicker.C {
			monitor.RefreshFromAMI()
		}
	}()

	authH := &handlers.AuthHandler{DB: database, JWTSecret: cfg.JWTSecret, JWTMinutes: cfg.JWTMinutes}
	tenantsH := &handlers.TenantsHandler{DB: database, AMI: amiRegistry}
	usersH   := &handlers.UsersHandler{DB: database, SIPTransport: cfg.SIPTransport}
	ticketsH := &handlers.TicketsHandler{DB: database}
	topicsH  := &handlers.TopicsHandler{DB: database}
	ivrH       := &handlers.IVRHandler{DB: database, UploadsDir: cfg.UploadsDir}
	kcNumbersH := &handlers.KCNumbersHandler{DB: database, AMI: amiRegistry}
	providersH := &handlers.ProvidersHandler{DB: database, AMI: amiRegistry}
	asteriskServersH := &handlers.AsteriskServersHandler{DB: database, AMI: amiRegistry, Monitor: monitor}
	blacklistH := &handlers.BlacklistHandler{DB: database}
	whitelistH := &handlers.WhitelistHandler{DB: database}
	cdrDB := database
	if cfg.CDRDSN != "" {
		if c, err := db.Connect(cfg.CDRDSN); err == nil {
			cdrDB = c
		} else {
			log.Printf("CDR DB connect warning: %v", err)
		}
	}

	// Recordings live in a private MinIO bucket — only this backend talks to
	// it (see CDRHandler.Audio), so the browser never learns MinIO's address
	// or credentials. Left nil (Audio then 503s) if not configured.
	var minioClient *minio.Client
	if cfg.MinioEndpoint != "" {
		mc, err := minio.New(cfg.MinioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			Secure: cfg.MinioUseSSL,
		})
		if err != nil {
			log.Printf("MinIO client init warning: %v", err)
		} else {
			minioClient = mc
		}
	}

	cdrH     := &handlers.CDRHandler{DB: cdrDB, Minio: minioClient, Bucket: cfg.MinioBucket}
	reportsH := &handlers.ReportsHandler{DB: database, CDRDB: cdrDB}
	settingsH := &handlers.SettingsHandler{DB: database, UploadsDir: cfg.UploadsDir}
	regH := &handlers.RegistrationHandler{DB: database, JWTSecret: cfg.JWTSecret, JWTMinutes: cfg.JWTMinutes, SIPTransport: cfg.SIPTransport}
	monitorH := &handlers.MonitorHandler{DB: database, Monitor: monitor, Hub: hub, AMI: amiRegistry}
	phoneH   := &handlers.PhoneHandler{DB: database, AsteriskWSURI: cfg.AsteriskWSURI, SIPDomain: cfg.SIPDomain, Monitor: monitor, AMI: amiRegistry}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(mw.CORS)

	// Manual/debug lookup only — the dialplan itself now checks the blacklist
	// directly over ODBC (BLACKLISTCHECK(), see EnsureBlacklistCheckSubroutine)
	// instead of calling this endpoint, so call routing doesn't depend on this
	// backend being reachable. Kept for ops to check a number without DB access.
	// No JWT, protected by ASTERISK_KEY.
	r.Get("/internal/blacklist/check", blacklistH.CheckPlain(cfg.AsteriskKey))
	r.Get("/internal/whitelist/check", whitelistH.CheckPlain(cfg.AsteriskKey))

	// Public build-info endpoint, so a deployed instance's version can be confirmed at a glance
	r.Get("/api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"commit":    buildCommit,
			"buildTime": buildTime,
		})
	})

	// Public branding endpoints — the login screen (pre-auth) needs these.
	r.Get("/api/settings/branding",      settingsH.GetBranding)
	r.Get("/api/settings/branding/logo", settingsH.Logo)

	r.Post("/api/auth/login",       authH.Login)
	r.Post("/api/auth/register",    regH.Register)
	r.Post("/api/auth/verify-code", regH.VerifyCode)

	r.Group(func(r chi.Router) {
		r.Use(mw.Auth(cfg.JWTSecret))

		r.Get("/api/auth/me",      authH.Me)
		r.Post("/api/auth/logout", authH.Logout)

		// Tenants — SuperAdmin only
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(0))
			r.Get("/api/tenants",                          tenantsH.List)
			r.Post("/api/tenants",                         tenantsH.Create)
			r.Get("/api/tenants/{id}",                     tenantsH.Get)
			r.Put("/api/tenants/{id}",                     tenantsH.Update)
			r.Delete("/api/tenants/{id}",                  tenantsH.Delete)
			r.Patch("/api/tenants/{id}/activate",          tenantsH.Activate)
			r.Patch("/api/tenants/{id}/deactivate",        tenantsH.Deactivate)
			r.Post("/api/tenants/{id}/users",              tenantsH.AssignUser)
			r.Delete("/api/tenants/{id}/users/{userId}",   tenantsH.UnassignUser)
			r.Post("/api/tenants/{id}/kc-numbers",             kcNumbersH.Create)
			r.Delete("/api/tenants/{id}/kc-numbers/{numberId}", kcNumbersH.Delete)

			r.Get("/api/providers",           providersH.List)
			r.Post("/api/providers",          providersH.Create)
			r.Put("/api/providers/{id}",      providersH.Update)
			r.Delete("/api/providers/{id}",   providersH.Delete)

			r.Get("/api/asterisk-servers",         asteriskServersH.List)
			r.Post("/api/asterisk-servers",        asteriskServersH.Create)
			r.Put("/api/asterisk-servers/{id}",    asteriskServersH.Update)
			r.Delete("/api/asterisk-servers/{id}", asteriskServersH.Delete)

			r.Put("/api/settings/branding",       settingsH.UpdateBranding)
			r.Post("/api/settings/branding/logo", settingsH.UploadLogo)
			r.Get("/api/settings/smpp",           settingsH.GetSMPPSettings)
			r.Put("/api/settings/smpp",            settingsH.UpdateSMPPSettings)

			r.Get("/api/users/unauthorized",    usersH.ListUnauthorized)
			r.Post("/api/users/{id}/authorize", usersH.Authorize)
		})

		// Users — SuperAdmin + TenantAdmin
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(1))
			r.Get("/api/users",                   usersH.List)
			r.Post("/api/users",                  usersH.Create)
			r.Get("/api/users/{id}",              usersH.Get)
			r.Put("/api/users/{id}",              usersH.Update)
			r.Delete("/api/users/{id}",           usersH.Delete)
			r.Patch("/api/users/{id}/activate",   usersH.Activate)
			r.Patch("/api/users/{id}/deactivate", usersH.Deactivate)
			r.Patch("/api/users/{id}/password",   usersH.ResetPassword)
		})

		// Topic Catalog — read for all, write for SuperAdmin + TenantAdmin
		r.Get("/api/topics", topicsH.ListMy)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(1))
			r.Get("/api/tenants/{id}/topics",              topicsH.List)
			r.Post("/api/tenants/{id}/topics",             topicsH.Create)
			r.Put("/api/tenants/{id}/topics/{topicId}",    topicsH.Update)
			r.Delete("/api/tenants/{id}/topics/{topicId}", topicsH.Delete)
		})

		// Tickets — all roles
		r.Get("/api/tickets",                  ticketsH.List)
		r.Post("/api/tickets",                 ticketsH.Create)
		r.Get("/api/tickets/assignable-users", ticketsH.AssignableUsers)
		r.Get("/api/tickets/{id}",             ticketsH.Get)
		r.Put("/api/tickets/{id}",             ticketsH.Update)
		r.Delete("/api/tickets/{id}",          ticketsH.Delete)
		r.Patch("/api/tickets/{id}/assign",    ticketsH.Assign)
		r.Get("/api/tickets/{id}/comments",    ticketsH.ListComments)
		r.Post("/api/tickets/{id}/comments",   ticketsH.AddComment)

		// CDR list — all authenticated users (operators see only their calls)
		r.Get("/api/cdr", cdrH.List)

		// Dashboard's live agent/call widget — all authenticated users,
		// scoped server-side to their own tenant (see TenantSnapshot).
		r.Get("/api/dashboard/monitor", monitorH.TenantSnapshot)

		// CDR detail + Monitor — SuperAdmin + TenantAdmin + Supervisor
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(2))
			r.Get("/api/cdr/{id}",           cdrH.Get)
			r.Get("/api/cdr/{id}/audio",     cdrH.Audio)
			r.Get("/api/monitor/snapshot",   monitorH.Snapshot)
			r.Get("/api/agents/info",        monitorH.AgentsInfo)
			r.Post("/api/actions/pause",     monitorH.Pause)
			r.Post("/api/actions/unpause",   monitorH.Unpause)
			r.Post("/api/actions/hangup",    monitorH.Hangup)
		})

		// Reports — SuperAdmin + TenantAdmin + Supervisor
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(2))
			r.Get("/api/reports/tickets", reportsH.Tickets)
		})

		// Blacklist — SuperAdmin + TenantAdmin
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(1))
			r.Get("/api/blacklist",              blacklistH.List)
			r.Post("/api/blacklist",             blacklistH.Create)
			r.Put("/api/blacklist/{id}",         blacklistH.Update)
			r.Delete("/api/blacklist/{id}",      blacklistH.Delete)
			r.Patch("/api/blacklist/{id}/toggle", blacklistH.Toggle)
			r.Get("/api/blacklist/check",        blacklistH.Check)
		})

		// Whitelist — SuperAdmin + TenantAdmin
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(1))
			r.Get("/api/whitelist",              whitelistH.List)
			r.Post("/api/whitelist",             whitelistH.Create)
			r.Put("/api/whitelist/{id}",         whitelistH.Update)
			r.Delete("/api/whitelist/{id}",      whitelistH.Delete)
			r.Patch("/api/whitelist/{id}/toggle", whitelistH.Toggle)
			r.Get("/api/whitelist/check",        whitelistH.Check)
		})

		// KC numbers overview — SuperAdmin + TenantAdmin + Supervisor (own tenant only)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(2))
			r.Get("/api/kc-numbers", kcNumbersH.ListMine)
		})

		// IVR — greeting/menu/queue-settings: SuperAdmin + TenantAdmin only
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(1))
			r.Get("/api/kc-numbers/{id}/ivr",                    ivrH.GetConfig)
			r.Put("/api/kc-numbers/{id}/ivr",                    ivrH.UpdateConfig)
			r.Post("/api/kc-numbers/{id}/ivr/greeting",          ivrH.UploadGreeting)
			r.Post("/api/kc-numbers/{id}/ivr/sync",              ivrH.Sync)
			r.Get("/api/kc-numbers/{id}/ivr/options",            ivrH.ListOptions)
			r.Post("/api/kc-numbers/{id}/ivr/options",           ivrH.SaveOption)
			r.Delete("/api/kc-numbers/{id}/ivr/options/{digit}", ivrH.DeleteOption)
		})

		// IVR — operators: SuperAdmin + TenantAdmin + Supervisor
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole(2))
			r.Get("/api/kc-numbers/{id}/ivr/members",               ivrH.ListMembers)
			r.Post("/api/kc-numbers/{id}/ivr/members",              ivrH.AddMember)
			r.Delete("/api/kc-numbers/{id}/ivr/members/{username}", ivrH.RemoveMember)
			r.Get("/api/kc-numbers/{id}/ivr/available-users",       ivrH.GetAvailableUsers)
		})

		r.Get("/api/phone/config",        phoneH.Config)
		r.Get("/api/phone/active-call",   phoneH.ActiveCall)
		r.Post("/api/phone/hangup",       phoneH.Hangup)
		r.Post("/api/phone/resume-call",  phoneH.ResumeCall)
		r.Post("/api/phone/hangup-intent", phoneH.HangupIntent)
		r.Get("/ws/monitor",       monitorH.ServeWS)
		r.Get("/ws/phone",         phoneH.ServeWS)
	})

	if _, err := os.Stat(cfg.StaticDir); err == nil {
		r.Get("/*", spaHandler(cfg.StaticDir))
	} else {
		log.Printf("static dir %q not found, frontend will not be served", cfg.StaticDir)
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("CallCentrix starting on %s (TLS)", cfg.HTTPAddr)
		err = http.ListenAndServeTLS(cfg.HTTPAddr, cfg.TLSCertFile, cfg.TLSKeyFile, r)
	} else {
		log.Printf("CallCentrix starting on %s", cfg.HTTPAddr)
		err = http.ListenAndServe(cfg.HTTPAddr, r)
	}
	if err != nil {
		log.Fatalf("server: %v", err)
	}
}

// spaHandler serves the built frontend from staticDir, falling back to
// index.html for unknown paths so client-side routing (React Router) works.
func spaHandler(staticDir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))
	index := filepath.Join(staticDir, "index.html")

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") || strings.HasPrefix(r.URL.Path, "/internal/") {
			http.NotFound(w, r)
			return
		}

		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, r, index)
			return
		}
		fileServer.ServeHTTP(w, r)
	}
}
