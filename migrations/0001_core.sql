-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================ IDENTITY ============================
CREATE TABLE identities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT UNIQUE,
    phone_enc       BYTEA,
    phone_hash      TEXT UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'buyer'
                    CHECK (role IN ('buyer','organizer','moderator','ops')),
    otp_verified_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identity_id  UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    fingerprint  TEXT NOT NULL,
    last_ip      INET,
    first_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (identity_id, fingerprint)
);
-- R4: mot fingerprint gan nhieu identity la tin hieu bot
CREATE INDEX idx_devices_fingerprint ON devices (fingerprint);

-- ============================ EVENT ============================
CREATE TABLE events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organizer_id   UUID NOT NULL REFERENCES identities(id),
    slug           TEXT NOT NULL UNIQUE,
    title          TEXT NOT NULL,
    description    TEXT,
    venue          TEXT,
    starts_at      TIMESTAMPTZ NOT NULL,
    sale_start_at  TIMESTAMPTZ NOT NULL,
    sale_end_at    TIMESTAMPTZ,
    status         TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN
                   ('DRAFT','PENDING_REVIEW','SCHEDULED','ON_SALE',
                    'SOLD_OUT','CLOSED','COMPLETED','REJECTED','CANCELLED')),
    max_per_identity INT NOT NULL DEFAULT 4 CHECK (max_per_identity > 0),
    max_per_order    INT NOT NULL DEFAULT 4 CHECK (max_per_order > 0),
    hold_ttl_seconds INT NOT NULL DEFAULT 600,   -- BR-O2: giu ghe 10 phut
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_sale_start ON events (sale_start_at) WHERE status = 'SCHEDULED';

CREATE TABLE ticket_types (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    price_cents   BIGINT NOT NULL CHECK (price_cents >= 0),
    currency      TEXT NOT NULL DEFAULT 'VND',
    quota         INT NOT NULL CHECK (quota > 0),      -- BR-E3: chi duoc tang
    bucket_count  INT NOT NULL DEFAULT 32 CHECK (bucket_count > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, name)
);

-- ===================== TON KHO PHAN MANH (BR-O1) =====================
-- Quota chia deu ra bucket_count dong de giam row-lock contention.
-- CHECK (available >= 0) la chot chan CUOI CUNG chong oversell o tang schema:
-- du moi tang tren deu sai, database van tu choi ban qua so ve.
CREATE TABLE ticket_inventory (
    ticket_type_id UUID NOT NULL REFERENCES ticket_types(id) ON DELETE CASCADE,
    bucket         INT  NOT NULL,
    capacity       INT  NOT NULL CHECK (capacity >= 0),
    available      INT  NOT NULL CHECK (available >= 0),
    PRIMARY KEY (ticket_type_id, bucket),
    CONSTRAINT inv_available_le_capacity CHECK (available <= capacity)
);

CREATE TABLE moderation_reviews (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    ai_score      NUMERIC(4,3),
    ai_verdict    TEXT CHECK (ai_verdict IN ('AUTO_APPROVED','NEEDS_HUMAN','AUTO_REJECTED')),
    ai_reasons    JSONB,
    human_id      UUID REFERENCES identities(id),
    human_verdict TEXT CHECK (human_verdict IN ('APPROVED','REJECTED')),
    human_note    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at    TIMESTAMPTZ
);

-- ============================ ORDER ============================
CREATE TABLE orders (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      UUID NOT NULL REFERENCES events(id),
    identity_id   UUID NOT NULL REFERENCES identities(id),
    status        TEXT NOT NULL DEFAULT 'HELD' CHECK (status IN
                  ('HELD','PAYMENT_PENDING','PAID','ISSUED',
                   'EXPIRED','CANCELLED','REFUNDING','REFUNDED')),
    total_cents   BIGINT NOT NULL DEFAULT 0,
    currency      TEXT NOT NULL DEFAULT 'VND',
    expires_at    TIMESTAMPTZ NOT NULL,   -- BR-O2: nguon su that cua dong ho dem nguoc
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- BR-O3: moi identity chi 1 don dang hoat dong tren moi su kien
CREATE UNIQUE INDEX uq_active_order_per_identity_event
    ON orders (event_id, identity_id)
    WHERE status IN ('HELD','PAYMENT_PENDING');
CREATE INDEX idx_orders_expiring ON orders (expires_at) WHERE status = 'HELD';

CREATE TABLE order_items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    ticket_type_id UUID NOT NULL REFERENCES ticket_types(id),
    bucket         INT  NOT NULL,
    quantity       INT  NOT NULL CHECK (quantity > 0),
    unit_cents     BIGINT NOT NULL CHECK (unit_cents >= 0)
);

-- BR-O7: released_at IS NULL la dieu kien cua UPDATE -> tra kho idempotent
CREATE TABLE seat_holds (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    ticket_type_id UUID NOT NULL REFERENCES ticket_types(id),
    bucket         INT  NOT NULL,
    quantity       INT  NOT NULL CHECK (quantity > 0),
    expires_at     TIMESTAMPTZ NOT NULL,
    released_at    TIMESTAMPTZ,
    consumed_at    TIMESTAMPTZ,
    CONSTRAINT hold_not_both CHECK (NOT (released_at IS NOT NULL AND consumed_at IS NOT NULL))
);
CREATE INDEX idx_holds_open ON seat_holds (expires_at)
    WHERE released_at IS NULL AND consumed_at IS NULL;

CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders(id),
    provider        TEXT NOT NULL,
    provider_txn_id TEXT NOT NULL,
    amount_cents    BIGINT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('PENDING','SUCCEEDED','FAILED','REFUNDED')),
    raw_payload     JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- BR-O6: chot chan exactly-once cho webhook
    CONSTRAINT uq_provider_txn UNIQUE (provider, provider_txn_id)
);

-- Webhook den truoc khi don ton tai -> luu day, replay sau
CREATE TABLE payment_inbox (
    id              BIGSERIAL PRIMARY KEY,
    provider        TEXT NOT NULL,
    provider_txn_id TEXT NOT NULL,
    payload         JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ,
    attempts        INT NOT NULL DEFAULT 0,
    UNIQUE (provider, provider_txn_id)
);

CREATE TABLE tickets (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL REFERENCES orders(id),
    ticket_type_id UUID NOT NULL REFERENCES ticket_types(id),
    jti            TEXT NOT NULL UNIQUE,        -- BR-O8: chong sao chep QR
    qr_sig         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'ISSUED'
                   CHECK (status IN ('ISSUED','CHECKED_IN','VOID','REFUNDED')),
    checked_in_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===================== HA TANG DUNG CHUNG =====================
-- BR-O5
CREATE TABLE idempotency_keys (
    key          TEXT NOT NULL,
    endpoint     TEXT NOT NULL,
    identity_id  UUID,
    request_hash TEXT NOT NULL,
    status_code  INT,
    response     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (key, endpoint)
);
CREATE INDEX idx_idem_expiry ON idempotency_keys (expires_at);

-- Transactional outbox: ghi cung transaction nghiep vu, relay day ra RabbitMQ
CREATE TABLE outbox_events (
    id             BIGSERIAL PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id   UUID NOT NULL,
    routing_key    TEXT NOT NULL,
    payload        JSONB NOT NULL,
    trace_id       TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);
CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    actor_type  TEXT NOT NULL,          -- human | ai | system
    actor_id    TEXT,
    action      TEXT NOT NULL,
    subject     TEXT NOT NULL,
    reason      TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_subject ON audit_logs (subject, created_at DESC);

-- BR-B1: moi quyet dinh chan phai giai thich duoc
CREATE TABLE bot_decisions (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID,
    identity_id  UUID,
    device_fp    TEXT,
    ip           INET,
    score        NUMERIC(4,3) NOT NULL,
    action       TEXT NOT NULL CHECK (action IN ('ALLOW','CHALLENGE','SLOW_LANE','BLOCK')),
    fired_rules  TEXT[] NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_bot_decisions_event ON bot_decisions (event_id, created_at DESC);

CREATE TABLE sale_reports (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     UUID NOT NULL REFERENCES events(id),
    metrics      JSONB NOT NULL,
    narrative    TEXT,
    model        TEXT,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Anh xa phien hang cho de audit (nguon nong van la Redis)
CREATE TABLE queue_sessions (
    token       TEXT PRIMARY KEY,
    event_id    UUID NOT NULL REFERENCES events(id),
    identity_id UUID NOT NULL REFERENCES identities(id),
    rank        BIGINT,
    state       TEXT NOT NULL CHECK (state IN
                ('LOBBY','QUEUED','ADMITTED','COMPLETED','EXPIRED','ABANDONED','DROPPED','BLOCKED')),
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    admitted_at TIMESTAMPTZ,
    UNIQUE (event_id, identity_id)
);

-- +goose Down
DROP TABLE IF EXISTS queue_sessions, sale_reports, bot_decisions, audit_logs,
    outbox_events, idempotency_keys, tickets, payment_inbox, payments,
    seat_holds, order_items, orders, moderation_reviews, ticket_inventory,
    ticket_types, events, devices, identities CASCADE;
