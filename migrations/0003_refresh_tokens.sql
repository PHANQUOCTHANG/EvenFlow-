-- +goose Up
-- EV-112: refresh token xoay vong, thu hoi duoc.
-- So 0003 vi 0002 da dung cho outbox relay (PR #55, EVF-36). Neu PR do khong merge,
-- goose van chay duoc (cho phep khoang trong so phien ban).

-- Chi luu SHA-256 cua token: lo bang nay cung khong dung duoc token.
-- family_id: moi lan dang nhap mot ho; moi lan refresh tao ban ghi moi cung ho va thu hoi
-- ban ghi cu. Dung lai ban ghi da thu hoi -> thu hoi ca ho (phat hien token bi danh cap).
CREATE TABLE refresh_tokens (
    id              UUID PRIMARY KEY,
    identity_id     UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    family_id       UUID NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ,
    revoked_reason  TEXT CHECK (revoked_reason IN
                    ('rotated','reuse_detected','logout','logout_all','password_reset')),
    replaced_by     UUID REFERENCES refresh_tokens(id),
    CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL))
);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_refresh_tokens_identity ON refresh_tokens (identity_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
