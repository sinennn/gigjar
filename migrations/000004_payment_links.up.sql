CREATE TABLE payment_links (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    reference       text NOT NULL,
    title           text NOT NULL,
    description     text NOT NULL DEFAULT '',
    amount          bigint NOT NULL,
    status          text NOT NULL DEFAULT 'open',
    idempotency_key text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    paid_at         timestamptz,
    closed_at       timestamptz,
    CONSTRAINT payment_links_reference_unique UNIQUE (reference),
    CONSTRAINT payment_links_idempotency_unique UNIQUE (user_id, idempotency_key),
    CONSTRAINT payment_links_title_length CHECK (char_length(title) BETWEEN 1 AND 120),
    CONSTRAINT payment_links_description_length CHECK (char_length(description) <= 1000),
    CONSTRAINT payment_links_amount_positive CHECK (amount > 0),
    CONSTRAINT payment_links_status_valid CHECK (status IN ('open', 'paid', 'closed')),
    CONSTRAINT payment_links_paid_at_consistent CHECK ((status <> 'paid' OR paid_at IS NOT NULL) AND (status <> 'open' OR paid_at IS NULL)),
    CONSTRAINT payment_links_closed_at_consistent CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);

CREATE INDEX payment_links_user_created_idx ON payment_links (user_id, created_at DESC, id DESC);
CREATE INDEX payment_links_open_amount_idx ON payment_links (user_id, amount, created_at) WHERE status = 'open';

CREATE TRIGGER payment_links_set_updated_at
    BEFORE UPDATE ON payment_links
    FOR EACH ROW EXECUTE FUNCTION gigjar_set_updated_at();
