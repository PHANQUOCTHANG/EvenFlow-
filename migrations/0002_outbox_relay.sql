-- +goose Up
-- Bang outbox_events nho (row cu duoc don o phase 2) nen rewrite do default volatile chap nhan duoc;
-- lock_timeout de migration bo cuoc thay vi xep hang sau relay / PersistHold va chan ca he thong.
SET LOCAL lock_timeout = '5s';
ALTER TABLE outbox_events
  ADD COLUMN event_id        UUID        NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN headers         JSONB       NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN attempts        INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_error      TEXT,
  ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_id_key UNIQUE (event_id);
DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX idx_outbox_due ON outbox_events (next_attempt_at, id) WHERE published_at IS NULL;

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP INDEX IF EXISTS idx_outbox_due;
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS outbox_events_event_id_key;
ALTER TABLE outbox_events
  DROP COLUMN IF EXISTS next_attempt_at,
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS attempts,
  DROP COLUMN IF EXISTS headers,
  DROP COLUMN IF EXISTS event_id;
