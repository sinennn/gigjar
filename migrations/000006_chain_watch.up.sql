CREATE TABLE jar_watch_state (
    user_id                  uuid PRIMARY KEY REFERENCES jars (user_id) ON DELETE RESTRICT,
    network                  text NOT NULL,
    address                  text NOT NULL,
    min_block_timestamp_ms   bigint NOT NULL DEFAULT 0,
    next_poll_at             timestamptz NOT NULL DEFAULT now(),
    last_polled_at           timestamptz,
    consecutive_failures     integer NOT NULL DEFAULT 0,
    last_error               text,
    last_reconciled_at       timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT jar_watch_state_address_unique UNIQUE (address),
    CONSTRAINT jar_watch_state_cursor_non_negative CHECK (min_block_timestamp_ms >= 0)
);

CREATE INDEX jar_watch_state_due_idx ON jar_watch_state (network, next_poll_at);

CREATE TRIGGER jar_watch_state_set_updated_at
    BEFORE UPDATE ON jar_watch_state
    FOR EACH ROW EXECUTE FUNCTION gigjar_set_updated_at();

CREATE TABLE incoming_transfers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    network         text NOT NULL,
    tx_hash         text NOT NULL,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    to_address      text NOT NULL,
    from_address    text NOT NULL,
    amount          bigint NOT NULL,
    status          text NOT NULL DEFAULT 'pending',
    failure_reason  text,
    log_indexes     integer[],
    block_number    bigint,
    block_time      timestamptz,
    payment_link_id uuid REFERENCES payment_links (id) ON DELETE RESTRICT,
    check_attempts  integer NOT NULL DEFAULT 0,
    next_check_at   timestamptz NOT NULL DEFAULT now(),
    observed_at     timestamptz NOT NULL DEFAULT now(),
    confirmed_at    timestamptz,
    failed_at       timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT incoming_transfers_tx_recipient_unique UNIQUE (network, tx_hash, to_address),
    CONSTRAINT incoming_transfers_network_valid CHECK (network IN ('tron-mainnet', 'tron-nile', 'simulated')),
    CONSTRAINT incoming_transfers_tx_hash_format CHECK (tx_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT incoming_transfers_to_format CHECK (to_address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$'),
    CONSTRAINT incoming_transfers_from_format CHECK (from_address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$'),
    CONSTRAINT incoming_transfers_amount_positive CHECK (amount > 0),
    CONSTRAINT incoming_transfers_status_valid CHECK (status IN ('pending', 'confirmed', 'failed')),
    CONSTRAINT incoming_transfers_confirmed_consistent CHECK (
        (status = 'confirmed') = (confirmed_at IS NOT NULL AND block_number IS NOT NULL AND block_time IS NOT NULL AND log_indexes IS NOT NULL)
    ),
    CONSTRAINT incoming_transfers_failed_consistent CHECK (
        (status = 'failed') = (failed_at IS NOT NULL AND failure_reason IS NOT NULL)
    ),
    CONSTRAINT incoming_transfers_link_only_when_confirmed CHECK (payment_link_id IS NULL OR status = 'confirmed')
);

CREATE INDEX incoming_transfers_due_idx ON incoming_transfers (network, next_check_at) WHERE status = 'pending';
CREATE INDEX incoming_transfers_user_observed_idx ON incoming_transfers (user_id, observed_at DESC, id DESC);
CREATE INDEX incoming_transfers_user_pending_idx ON incoming_transfers (user_id) WHERE status = 'pending';
CREATE INDEX incoming_transfers_link_idx ON incoming_transfers (payment_link_id) WHERE payment_link_id IS NOT NULL;

CREATE FUNCTION gigjar_freeze_resolved_transfer() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' OR OLD.status <> 'pending' THEN
        RAISE EXCEPTION '% on resolved incoming transfer % is not allowed', TG_OP, OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.network <> OLD.network OR NEW.tx_hash <> OLD.tx_hash
       OR NEW.user_id <> OLD.user_id OR NEW.to_address <> OLD.to_address OR NEW.observed_at <> OLD.observed_at THEN
        RAISE EXCEPTION 'identity columns of incoming transfer % are immutable', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER incoming_transfers_freeze
    BEFORE UPDATE OR DELETE ON incoming_transfers
    FOR EACH ROW EXECUTE FUNCTION gigjar_freeze_resolved_transfer();

ALTER TABLE journal_entries
    ADD CONSTRAINT journal_entries_incoming_transfer_fk
    FOREIGN KEY (incoming_transfer_id) REFERENCES incoming_transfers (id) ON DELETE RESTRICT;

CREATE TABLE simulated_chain_transfers (
    tx_hash       text PRIMARY KEY,
    block_number  bigint GENERATED ALWAYS AS IDENTITY,
    log_index     integer NOT NULL DEFAULT 0,
    from_address  text NOT NULL,
    to_address    text NOT NULL,
    amount        bigint NOT NULL,
    outcome       text NOT NULL DEFAULT 'success',
    created_at    timestamptz NOT NULL DEFAULT now(),
    solidifies_at timestamptz NOT NULL,
    CONSTRAINT simulated_chain_transfers_tx_hash_format CHECK (tx_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT simulated_chain_transfers_amount_positive CHECK (amount > 0),
    CONSTRAINT simulated_chain_transfers_outcome_valid CHECK (outcome IN ('success', 'reverted')),
    CONSTRAINT simulated_chain_transfers_solidifies_after_creation CHECK (solidifies_at >= created_at)
);

CREATE INDEX simulated_chain_transfers_to_idx ON simulated_chain_transfers (to_address, created_at);
CREATE INDEX simulated_chain_transfers_from_idx ON simulated_chain_transfers (from_address);
