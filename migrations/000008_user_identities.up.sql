ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

CREATE TABLE user_identities (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider      text NOT NULL,
    subject       text NOT NULL,
    email         citext NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_identities_provider_valid CHECK (provider IN ('google')),
    CONSTRAINT user_identities_subject_length CHECK (char_length(subject) BETWEEN 1 AND 255),
    CONSTRAINT user_identities_provider_subject_unique UNIQUE (provider, subject),
    CONSTRAINT user_identities_user_provider_unique UNIQUE (user_id, provider)
);
