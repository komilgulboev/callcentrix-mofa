-- CallCentrix initial schema
-- Run automatically on server start

-- ─── Tenants ──────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tenants (
    id           SERIAL PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    domain       VARCHAR(255),
    max_users    INT  DEFAULT 50,
    active       BOOLEAN DEFAULT TRUE,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- ─── Users ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    tenant_id     INT REFERENCES tenants(id) ON DELETE SET NULL,
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name    VARCHAR(100) DEFAULT '',
    last_name     VARCHAR(100) DEFAULT '',
    user_type     INT DEFAULT 3,   -- 0=SuperAdmin 1=TenantAdmin 2=Supervisor 3=Operator
    role          INT DEFAULT 0,
    sip_no        VARCHAR(50)  DEFAULT '',
    sip_password  VARCHAR(255) DEFAULT '',
    active        BOOLEAN DEFAULT FALSE,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Seed default SuperAdmin (password: admin123) if no users exist
INSERT INTO users (username, password_hash, user_type, active)
SELECT 'admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 0, TRUE
WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'admin');

-- ─── Tickets ──────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tickets (
    id          SERIAL PRIMARY KEY,
    tenant_id   INT REFERENCES tenants(id) ON DELETE SET NULL,
    subject     VARCHAR(500) NOT NULL,
    body        TEXT DEFAULT '',
    caller_no   VARCHAR(50)  DEFAULT '',
    callee_no   VARCHAR(50)  DEFAULT '',
    user_id     INT REFERENCES users(id) ON DELETE SET NULL,
    status      VARCHAR(50)  DEFAULT 'new',   -- new open pending resolved closed
    priority    VARCHAR(50)  DEFAULT 'normal', -- low normal high urgent
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ticket_comments (
    id         SERIAL PRIMARY KEY,
    ticket_id  INT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id    INT REFERENCES users(id) ON DELETE SET NULL,
    username   VARCHAR(100) DEFAULT '',
    text       TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ─── Topic Catalog ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS topic_catalog (
    id         SERIAL PRIMARY KEY,
    tenant_id  INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    names      JSONB NOT NULL DEFAULT '{}',  -- {"ru":"...","tj":"...","en":"..."}
    active     BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS topic_id INT REFERENCES topic_catalog(id) ON DELETE SET NULL;
-- The specialist (TenantAdmin/Supervisor) an operator has assigned this
-- ticket to — distinct from user_id, which is whoever created/handled it.
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS assigned_user_id INT REFERENCES users(id) ON DELETE SET NULL;

-- ─── Tasks (Dashboard Kanban) ─────────────────────────────────────────────────
-- Internal work-item tracking, distinct from Tickets (customer-issue
-- tracking): SuperAdmin/TenantAdmin create and assign tasks to
-- Supervisors/Operators, who work them on a 4-column Kanban board.
CREATE TABLE IF NOT EXISTS tasks (
    id               SERIAL PRIMARY KEY,
    tenant_id        INT REFERENCES tenants(id) ON DELETE SET NULL,
    title            VARCHAR(500) NOT NULL,
    description      TEXT DEFAULT '',
    status           VARCHAR(20) NOT NULL DEFAULT 'todo',  -- todo in_progress waiting resolved
    created_by       INT REFERENCES users(id) ON DELETE SET NULL,
    assigned_user_id INT REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW()
);

-- Many-to-many: a task can be assigned to several Supervisors/Operators, one
-- of whom may optionally be marked primary (see internal/handlers/tasks.go
-- canChangeStatus) — when a primary is set, only they may change the task's
-- status; everyone else on the task sees the same shared status/board entry
-- and gets notified of every change regardless.
CREATE TABLE IF NOT EXISTS task_assignees (
    task_id    INT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (task_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_task_assignees_user ON task_assignees(user_id);
-- At most one primary per task.
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_assignees_one_primary ON task_assignees(task_id) WHERE is_primary;

-- One-time backfill: migrate the old single-assignee column into the new
-- many-assignees model (as that task's primary, since it was previously the
-- sole/exclusive status-changer) before dropping it. Guarded by an
-- information_schema check, not just IF EXISTS on the ALTER, because the
-- backfill SELECT itself references the column by name — on a second run
-- (column already gone) that SELECT would fail to parse if it weren't
-- skipped entirely, unlike a plain DROP COLUMN IF EXISTS.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'tasks' AND column_name = 'assigned_user_id'
    ) THEN
        INSERT INTO task_assignees (task_id, user_id, is_primary)
        SELECT id, assigned_user_id, TRUE FROM tasks
        WHERE assigned_user_id IS NOT NULL
        ON CONFLICT (task_id, user_id) DO NOTHING;

        ALTER TABLE tasks DROP COLUMN assigned_user_id;
    END IF;
END $$;

-- In-app notifications for a task's creator, fired whenever its status
-- changes — the Dashboard header bell polls this. See also telegram_settings
-- below and users.telegram_chat_id for the Telegram delivery side.
CREATE TABLE IF NOT EXISTS task_notifications (
    id         SERIAL PRIMARY KEY,
    task_id    INT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- recipient (the creator)
    status     VARCHAR(20) NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    is_read    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ─── Blacklist (phone numbers) ───────────────────────────────────────────────
-- Default-allow gate: callers are let through unless their number is an
-- active, non-expired blacklist entry for the tenant. expires_at NULL means
-- the block is permanent. Applies to every KC number of the tenant.
CREATE TABLE IF NOT EXISTS blacklist (
    id          SERIAL PRIMARY KEY,
    tenant_id   INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    phone       VARCHAR(50) NOT NULL,
    comment     VARCHAR(255) DEFAULT '',
    active      BOOLEAN DEFAULT TRUE,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, phone)
);
CREATE INDEX IF NOT EXISTS idx_blacklist_tenant ON blacklist(tenant_id);
CREATE INDEX IF NOT EXISTS idx_blacklist_phone  ON blacklist(phone);

-- One-time backfill: dialplan rows written by the previous (whitelist,
-- default-deny) code still literally call Gosub(whitelist-check,...) gated on
-- GOSUB_RETVAL="0". EnsureBlacklistCheckSubroutine (see internal/asterisk)
-- only (re)writes the shared subroutine context itself — a KC number's own
-- dialplan and a provider's anonymous-fallback "s" extension are only
-- rewritten on specific triggers (create/delete/sync), so a plain binary
-- upgrade would otherwise leave existing numbers pointed at the old,
-- inverted gate (and the dead /internal/whitelist/check route). Rewrite them
-- here so upgrading is enough. Safe to rerun: matches 0 rows once applied.
UPDATE ast_extensions SET appdata = REPLACE(appdata, 'whitelist-check,s,1(', 'blacklist-check,s,1(')
WHERE app = 'Gosub' AND appdata LIKE 'whitelist-check,s,1(%';

UPDATE ast_extensions SET appdata = '$["${GOSUB_RETVAL}" = "1"]?blocked,s,1'
WHERE app = 'GotoIf' AND appdata = '$["${GOSUB_RETVAL}" = "0"]?blocked,s,1';

-- One-time backfill: existing tenants' 'h' (hangup handler) extension was
-- written with an explicit Hangup() app (see asterisk.CreateTenantContext).
-- The channel is already tearing down by the time 'h' runs, so that call was
-- redundant — and was spawning a spurious zero-duration CDR row (dst='h')
-- alongside every real call's own CDR. The generator now writes NoOp here
-- instead; this backfill applies the same fix to tenants created before that
-- change. Safe to rerun: matches 0 rows once applied.
UPDATE ast_extensions SET app = 'NoOp'
WHERE context LIKE 'tenant-%' AND exten = 'h' AND priority = 1 AND app = 'Hangup';

-- ─── Whitelist (phone numbers) ───────────────────────────────────────────────
-- Opt-in default-deny gate, per KC number (see ivr_configs.whitelist_enabled
-- below): when enabled for a number, callers are blocked unless their number
-- is an active whitelist entry for the tenant. Entries themselves are
-- tenant-wide, same as blacklist — the per-number flag only controls whether
-- that number enforces the gate.
CREATE TABLE IF NOT EXISTS whitelist (
    id          SERIAL PRIMARY KEY,
    tenant_id   INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    phone       VARCHAR(50) NOT NULL,
    comment     VARCHAR(255) DEFAULT '',
    active      BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, phone)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist(tenant_id);
CREATE INDEX IF NOT EXISTS idx_whitelist_phone  ON whitelist(phone);

-- ─── SIP Providers (carrier trunks) ──────────────────────────────────────────
-- One row = one PJSIP trunk to a telecom carrier (endpoint+aor+auth+identify,
-- optionally a registration). SuperAdmin manages these; a KC number is then
-- created against a provider so inbound routing/identify knows which trunk a
-- DID belongs to. See internal/asterisk/provider.go.
CREATE TABLE IF NOT EXISTS providers (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    host       VARCHAR(255) NOT NULL,
    port       INT NOT NULL DEFAULT 5060,
    transport  VARCHAR(50)  NOT NULL DEFAULT 'transport-udp',
    codecs     VARCHAR(100) NOT NULL DEFAULT 'ulaw,alaw',
    username   VARCHAR(100) NOT NULL DEFAULT '', -- SIP auth username, also used as from_user
    password   VARCHAR(255) NOT NULL DEFAULT '',
    register   BOOLEAN      NOT NULL DEFAULT TRUE, -- whether we send REGISTER to this provider
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- A tenant's single outbound trunk (agents dialing "9"+number, see
-- CreateTenantContext) — separate from kc_numbers.provider_id, which is
-- per-DID and inbound-only. NULL = outbound calling disabled for the tenant.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS outbound_provider_id INT REFERENCES providers(id);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS outbound_caller_id VARCHAR(50) DEFAULT '';

-- ─── KC Numbers (call-center DIDs) ───────────────────────────────────────────
-- A tenant can own several inbound numbers ("КЦ"); each gets its own fully
-- independent IVR: greeting, menu, queue and operators. Created/removed only
-- by SuperAdmin (see TenantsHandler / KCNumbersHandler); everything else
-- (greeting/menu/queue/members) is configured per-number by the tenant admin.
CREATE TABLE IF NOT EXISTS kc_numbers (
    id          SERIAL PRIMARY KEY,
    tenant_id   INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider_id INT REFERENCES providers(id),
    number      VARCHAR(50) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_kc_numbers_tenant ON kc_numbers(tenant_id);
ALTER TABLE kc_numbers ADD COLUMN IF NOT EXISTS provider_id INT REFERENCES providers(id);

-- ─── IVR / Queue Management (now scoped per KC number, not per tenant) ───────
CREATE TABLE IF NOT EXISTS ivr_configs (
    id             SERIAL PRIMARY KEY,
    tenant_id      INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kc_number_id   INT UNIQUE REFERENCES kc_numbers(id) ON DELETE CASCADE,
    greeting_file  VARCHAR(255) DEFAULT '',      -- filename for Asterisk Background()
    moh_class      VARCHAR(100) DEFAULT 'default',
    wait_timeout   INT DEFAULT 5,                -- WaitExten seconds
    queue_timeout  INT DEFAULT 300,              -- max time in queue (seconds)
    strategy       VARCHAR(50)  DEFAULT 'ringall',
    max_callers    INT DEFAULT 0,                -- 0 = unlimited
    did_number     VARCHAR(50)  DEFAULT '',      -- legacy free-text field, superseded by kc_numbers.number
    closed_greeting_file VARCHAR(255) DEFAULT '', -- played instead of greeting_file outside work hours
    work_hours_enabled BOOLEAN DEFAULT FALSE,
    work_hours_start   VARCHAR(5)  DEFAULT '09:00',
    work_hours_end     VARCHAR(5)  DEFAULT '18:00',
    work_days          VARCHAR(30) DEFAULT 'mon,tue,wed,thu,fri', -- Asterisk GotoIfTime weekday list
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);
-- Was "tenant_id INT NOT NULL UNIQUE" (one config per tenant) before KC numbers
-- existed; a tenant can now have several numbers, each with its own config.
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS kc_number_id INT REFERENCES kc_numbers(id) ON DELETE CASCADE;
ALTER TABLE ivr_configs DROP CONSTRAINT IF EXISTS ivr_configs_tenant_id_key;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ivr_configs_kc_number_id_key') THEN
        ALTER TABLE ivr_configs ADD CONSTRAINT ivr_configs_kc_number_id_key UNIQUE (kc_number_id);
    END IF;
END $$;
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS closed_greeting_file VARCHAR(255) DEFAULT '';
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS work_hours_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS work_hours_start VARCHAR(5) DEFAULT '09:00';
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS work_hours_end VARCHAR(5) DEFAULT '18:00';
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS work_days VARCHAR(30) DEFAULT 'mon,tue,wed,thu,fri';
-- Per-number opt-in for the whitelist gate (see the whitelist table above).
-- Defaults to FALSE so existing/new numbers keep working without an admin
-- having to populate a tenant's whitelist first.
ALTER TABLE ivr_configs ADD COLUMN IF NOT EXISTS whitelist_enabled BOOLEAN DEFAULT FALSE;

-- One-time backfill: pre-existing configs (from before KC numbers existed) that
-- already had a did_number get a matching kc_numbers row so they aren't orphaned.
-- Configs with no did_number can't be backfilled (no real number to attach to)
-- and simply won't show up until an admin creates a KC number for them.
INSERT INTO kc_numbers (tenant_id, number)
SELECT ic.tenant_id, ic.did_number
FROM ivr_configs ic
WHERE ic.kc_number_id IS NULL AND ic.did_number <> ''
ON CONFLICT (number) DO NOTHING;

UPDATE ivr_configs ic SET kc_number_id = kn.id
FROM kc_numbers kn
WHERE ic.kc_number_id IS NULL AND ic.did_number <> ''
  AND kn.tenant_id = ic.tenant_id AND kn.number = ic.did_number;

CREATE TABLE IF NOT EXISTS ivr_options (
    id           SERIAL PRIMARY KEY,
    tenant_id    INT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kc_number_id INT  REFERENCES kc_numbers(id) ON DELETE CASCADE,
    digit        CHAR(1) NOT NULL,
    label        VARCHAR(255) NOT NULL DEFAULT '',
    action       VARCHAR(50)  NOT NULL DEFAULT 'queue', -- queue|extension|hangup|playback
    action_data  VARCHAR(255) DEFAULT '',
    sort_order   INT DEFAULT 0
);
ALTER TABLE ivr_options ADD COLUMN IF NOT EXISTS kc_number_id INT REFERENCES kc_numbers(id) ON DELETE CASCADE;
ALTER TABLE ivr_options DROP CONSTRAINT IF EXISTS ivr_options_tenant_id_digit_key;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ivr_options_kc_number_id_digit_key') THEN
        ALTER TABLE ivr_options ADD CONSTRAINT ivr_options_kc_number_id_digit_key UNIQUE (kc_number_id, digit);
    END IF;
END $$;

-- Backfill ivr_options.kc_number_id from the (now backfilled) config of the same tenant.
-- Only safe/unambiguous when a tenant had exactly one legacy config, which was
-- always true pre-KC-numbers (ivr_configs.tenant_id used to be UNIQUE).
UPDATE ivr_options io SET kc_number_id = ic.kc_number_id
FROM ivr_configs ic
WHERE io.kc_number_id IS NULL AND ic.kc_number_id IS NOT NULL AND ic.tenant_id = io.tenant_id;

-- ─── Music On Hold (realtime, one class per KC number) ───────────────────────
-- Standard Asterisk realtime "musiconhold" family columns (res_musiconhold's
-- ODBC/realtime backend) — this app owns this table's schema the same way it
-- owns ast_cdr's, unlike ast_extensions/ast_ps_endpoints/ast_queues, which
-- are assumed pre-provisioned. IMPORTANT: creating the table alone isn't
-- enough — Asterisk only reads MOH classes from here once musiconhold.conf
-- on the Asterisk server has a realtime family pointing at it, e.g.:
--   [general]
--   realtime=yes
-- and in extconfig.conf (or res_odbc.conf's realtime mapping):
--   musiconhold => odbc,<dsn>,ast_musiconhold
-- This is a one-time manual step per Asterisk server — same category as the
-- provider-context declarations in extensions.conf documented in
-- ASTERISK_CLUSTER_SETUP.md, which this backend also can't do for you since
-- it has no filesystem access to Asterisk's own config files. See
-- IVRHandler.UploadMOH / asterisk.UpsertMOHClass for how rows here get
-- written, and note `directory` is resolved by Asterisk relative to its own
-- MOH root (/var/lib/asterisk/moh/ by default) — the uploaded file only
-- plays if that path is reachable from the Asterisk box, same shared-storage
-- assumption already relied on for IVR greetings (UploadsDir).
CREATE TABLE IF NOT EXISTS ast_musiconhold (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(80) NOT NULL UNIQUE,
    mode        VARCHAR(20)  NOT NULL DEFAULT 'files',
    directory   VARCHAR(255) NOT NULL DEFAULT '',
    application VARCHAR(255) DEFAULT '',
    digit       VARCHAR(1)   DEFAULT '',
    sort        VARCHAR(20)  DEFAULT 'random',
    format      VARCHAR(20)  DEFAULT ''
);

-- ─── Asterisk CDR (populated directly by Asterisk's cdr_pgsql module) ────────
-- Column names/types follow the standard Asterisk CDR field set so cdr_pgsql
-- can map to them automatically; `id` is app-only (used for row lookups).
CREATE TABLE IF NOT EXISTS ast_cdr (
    id          SERIAL PRIMARY KEY,
    calldate    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    clid        VARCHAR(80)  NOT NULL DEFAULT '',
    src         VARCHAR(80)  NOT NULL DEFAULT '',
    dst         VARCHAR(80)  NOT NULL DEFAULT '',
    dcontext    VARCHAR(80)  NOT NULL DEFAULT '',
    channel     VARCHAR(80)  NOT NULL DEFAULT '',
    dstchannel  VARCHAR(80)  NOT NULL DEFAULT '',
    lastapp     VARCHAR(80)  NOT NULL DEFAULT '',
    lastdata    VARCHAR(80)  NOT NULL DEFAULT '',
    duration    BIGINT       NOT NULL DEFAULT 0,
    billsec     BIGINT       NOT NULL DEFAULT 0,
    disposition VARCHAR(45)  NOT NULL DEFAULT '',
    amaflags    INT          NOT NULL DEFAULT 0,
    accountcode VARCHAR(20)  NOT NULL DEFAULT '',
    uniqueid    VARCHAR(150) NOT NULL DEFAULT '',
    userfield   VARCHAR(256) NOT NULL DEFAULT '',
    peeraccount VARCHAR(80)  NOT NULL DEFAULT '',
    linkedid    VARCHAR(150) NOT NULL DEFAULT '',
    sequence    INT          NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_ast_cdr_calldate    ON ast_cdr(calldate DESC);
CREATE INDEX IF NOT EXISTS idx_ast_cdr_src         ON ast_cdr(src);
CREATE INDEX IF NOT EXISTS idx_ast_cdr_dst         ON ast_cdr(dst);
CREATE INDEX IF NOT EXISTS idx_ast_cdr_accountcode ON ast_cdr(accountcode);
CREATE INDEX IF NOT EXISTS idx_ast_cdr_uniqueid    ON ast_cdr(uniqueid);

-- ─── System Settings (global branding for the login screen) ─────────────────
-- Single-row table (id fixed to 1): platform-wide, not per-tenant — the login
-- screen renders before the app knows which tenant a user belongs to.
CREATE TABLE IF NOT EXISTS system_settings (
    id            INT PRIMARY KEY DEFAULT 1,
    platform_name VARCHAR(255) NOT NULL DEFAULT 'CallCentrix',
    system_info   TEXT NOT NULL DEFAULT '',
    logo_path     VARCHAR(255) NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT system_settings_single_row CHECK (id = 1)
);
INSERT INTO system_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS registration_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- ─── Public self-registration (SMS-confirmed via SMPP) ───────────────────────
-- phone_verified is distinct from `active`: it tracks whether the user proved
-- ownership of their phone number via SMS code. Existing/admin-created users
-- default to TRUE so they never show up in the "unauthorized users" list —
-- only self-registrations start out unverified.
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_verified BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_code VARCHAR(10) DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_code_sent_at TIMESTAMPTZ;

-- Single-row SMPP gateway config (same pattern as system_settings).
CREATE TABLE IF NOT EXISTS smpp_settings (
    id         INT PRIMARY KEY DEFAULT 1,
    host       VARCHAR(255) NOT NULL DEFAULT '',
    port       INT NOT NULL DEFAULT 2775,
    system_id  VARCHAR(100) NOT NULL DEFAULT '',  -- логин
    password   VARCHAR(255) NOT NULL DEFAULT '',
    sender_id  VARCHAR(50)  NOT NULL DEFAULT '',  -- идентификатор / имя отправителя (source_addr)
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT smpp_settings_single_row CHECK (id = 1)
);
INSERT INTO smpp_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Single-row Telegram bot config (same pattern as smpp_settings). Outbound
-- notifications only — no getUpdates polling/bot-linking flow — chat IDs are
-- entered manually per-user (see users.telegram_chat_id below).
CREATE TABLE IF NOT EXISTS telegram_settings (
    id         INT PRIMARY KEY DEFAULT 1,
    bot_token  VARCHAR(255) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT telegram_settings_single_row CHECK (id = 1)
);
INSERT INTO telegram_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Tracks getUpdates' offset for the long-polling bot (see
-- TasksHandler.RunTelegramBot) so a backend restart doesn't replay
-- already-handled task-status button presses.
ALTER TABLE telegram_settings ADD COLUMN IF NOT EXISTS update_offset INT NOT NULL DEFAULT 0;

-- Manually entered by SuperAdmin/TenantAdmin on the user's create/edit form —
-- targets Telegram task-assignment/status-change notifications at this user.
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(64) DEFAULT '';

-- ─── Asterisk Servers (multi-box telephony, single shared DB) ───────────────
-- One row per physical Asterisk box. Users are assigned to a server
-- (least-loaded, see asterisk.PickLeastLoadedServer) so their softphone WS
-- session and AMI actions target the right box, while SIP/dialplan config
-- itself stays fully shared via realtime — see ASTERISK_CLUSTER_SETUP.md.
CREATE TABLE IF NOT EXISTS asterisk_servers (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL UNIQUE,
    ami_host   VARCHAR(255) NOT NULL,   -- host:port for AMI, e.g. 10.0.0.5:5038
    ami_user   VARCHAR(100) NOT NULL,
    ami_pass   VARCHAR(255) NOT NULL,
    ws_uri     VARCHAR(255) NOT NULL,   -- wss://host:8089/ws — handed to browsers via the WS proxy
    sip_host   VARCHAR(255) NOT NULL,   -- public IP/host other Asterisk boxes use to reach this one
    sip_port   INT NOT NULL DEFAULT 5060,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Nullable: legacy/unassigned users keep working against the single
-- fallback server (cfg.AMIAddr/AsteriskWSURI) until servers are configured.
ALTER TABLE users ADD COLUMN IF NOT EXISTS server_id INT REFERENCES asterisk_servers(id);

-- ─── Knowledge Base ───────────────────────────────────────────────────────────
-- Guides TenantAdmin writes for their own tenant's operators. Categories are a
-- shared global taxonomy curated by SuperAdmin (same shape as topic_catalog);
-- articles themselves are strictly tenant-scoped and never visible cross-tenant.
CREATE TABLE IF NOT EXISTS kb_categories (
    id         SERIAL PRIMARY KEY,
    names      JSONB NOT NULL DEFAULT '{}',  -- {"ru":"...","tj":"...","en":"..."}
    active     BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb_articles (
    id          SERIAL PRIMARY KEY,
    tenant_id   INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    category_id INT REFERENCES kb_categories(id) ON DELETE SET NULL,
    title       VARCHAR(500) NOT NULL,
    body        TEXT DEFAULT '',
    created_by  INT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Freeform hashtags, many per article; normalized lowercase, no leading '#'.
CREATE TABLE IF NOT EXISTS kb_article_tags (
    article_id INT NOT NULL REFERENCES kb_articles(id) ON DELETE CASCADE,
    tag        VARCHAR(100) NOT NULL,
    PRIMARY KEY (article_id, tag)
);

-- Photos/videos attached to an article, stored in MinIO (see
-- KnowledgeBaseHandler.UploadArticleMedia) under object key "kb/<article_id>/<id><ext>"
-- — same bucket as call recordings (cfg.MinioBucket), segregated by prefix.
-- Replaces the earlier local-disk kb_article_photos: video support was added
-- alongside a move to MinIO, so photos moved there too rather than keeping
-- two different storage backends for the same "article media" concept.
DROP TABLE IF EXISTS kb_article_photos;
CREATE TABLE IF NOT EXISTS kb_article_media (
    id           SERIAL PRIMARY KEY,
    article_id   INT NOT NULL REFERENCES kb_articles(id) ON DELETE CASCADE,
    media_type   VARCHAR(10) NOT NULL DEFAULT 'photo',  -- photo | video
    object_key   VARCHAR(255) NOT NULL DEFAULT '',
    content_type VARCHAR(100) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

-- Article visibility: by default every tenant member can read an article
-- (visible_to_all). A TenantAdmin may instead restrict it to specific
-- Supervisors/Operators via kb_article_users — see
-- KnowledgeBaseHandler.ListArticles/GetArticle for the enforcement (the
-- authoring TenantAdmin and SuperAdmin always bypass this, same as they
-- already bypass tenant-scoping for management purposes).
ALTER TABLE kb_articles ADD COLUMN IF NOT EXISTS visible_to_all BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS kb_article_users (
    article_id INT NOT NULL REFERENCES kb_articles(id) ON DELETE CASCADE,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, user_id)
);

-- ─── Indexes ──────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_tickets_tenant    ON tickets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status    ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_user      ON tickets(user_id);
CREATE INDEX IF NOT EXISTS idx_comments_ticket   ON ticket_comments(ticket_id);
CREATE INDEX IF NOT EXISTS idx_users_tenant      ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_server      ON users(server_id);
CREATE INDEX IF NOT EXISTS idx_topic_tenant      ON topic_catalog(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tickets_topic     ON tickets(topic_id);
CREATE INDEX IF NOT EXISTS idx_tickets_assigned  ON tickets(assigned_user_id);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant      ON tasks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tasks_creator     ON tasks(created_by);
CREATE INDEX IF NOT EXISTS idx_tasks_status      ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_task_notif_user   ON task_notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_kb_articles_tenant   ON kb_articles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_kb_articles_category ON kb_articles(category_id);
CREATE INDEX IF NOT EXISTS idx_kb_article_tags_tag   ON kb_article_tags(tag);
CREATE INDEX IF NOT EXISTS idx_kb_article_media_article ON kb_article_media(article_id);
CREATE INDEX IF NOT EXISTS idx_kb_article_users_user ON kb_article_users(user_id);
