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

-- ─── Indexes ──────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_tickets_tenant    ON tickets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status    ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_user      ON tickets(user_id);
CREATE INDEX IF NOT EXISTS idx_comments_ticket   ON ticket_comments(ticket_id);
CREATE INDEX IF NOT EXISTS idx_users_tenant      ON users(tenant_id);
