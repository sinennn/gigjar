CREATE TABLE password_resets (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    CONSTRAINT password_resets_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT password_resets_expiry_after_creation CHECK (expires_at > created_at)
);

CREATE INDEX password_resets_user_idx ON password_resets (user_id) WHERE used_at IS NULL;
