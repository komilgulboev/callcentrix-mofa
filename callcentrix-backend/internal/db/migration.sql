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
