ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_kind_valid;
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_owner_matches_kind;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_kind_valid
    CHECK (kind IN ('jar_available', 'jar_reserved', 'settled', 'chain_inflow', 'platform_fees'));
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_owner_matches_kind
    CHECK ((kind IN ('chain_inflow', 'platform_fees')) = (user_id IS NULL));

INSERT INTO ledger_accounts (user_id, kind) VALUES (NULL, 'platform_fees');

CREATE TABLE settlements (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    network             text NOT NULL,
    jar_address         text NOT NULL,
    jar_index           bigint NOT NULL,
    destination_id      bigint NOT NULL REFERENCES cleva_destinations (id) ON DELETE RESTRICT,
    destination_address text NOT NULL,
    amount              bigint NOT NULL,
    fee                 bigint NOT NULL,
    state               text NOT NULL DEFAULT 'queued',
    resources_ready_at  timestamptz,
    energy_required     bigint,
    energy_order_ref    text,
    energy_ordered_at   timestamptz,
    burn_fallback       boolean NOT NULL DEFAULT false,
    topup_tx_id         text,
    topup_sent_at       timestamptz,
    fee_limit_sun       bigint,
    tx_id               text,
    signed_tx           jsonb,
    tx_expires_at       timestamptz,
    broadcast_at        timestamptz,
    block_number        bigint,
    attempts            integer NOT NULL DEFAULT 0,
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    last_error          text,
    failure_reason      text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    confirmed_at        timestamptz,
    failed_at           timestamptz,
    CONSTRAINT settlements_network_valid CHECK (network IN ('tron-mainnet', 'tron-nile', 'simulated')),
    CONSTRAINT settlements_state_valid CHECK (state IN ('queued', 'signing', 'broadcast', 'confirmed', 'failed')),
    CONSTRAINT settlements_amounts_valid CHECK (amount > 0 AND fee >= 0 AND fee < amount),
    CONSTRAINT settlements_jar_format CHECK (jar_address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$'),
    CONSTRAINT settlements_destination_format CHECK (destination_address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$'),
    CONSTRAINT settlements_tx_id_format CHECK (tx_id IS NULL OR tx_id ~ '^[0-9a-f]{64}$'),
    CONSTRAINT settlements_tx_persisted_before_broadcast CHECK (state NOT IN ('broadcast', 'confirmed') OR (tx_id IS NOT NULL AND signed_tx IS NOT NULL AND tx_expires_at IS NOT NULL)),
    CONSTRAINT settlements_confirmed_consistent CHECK ((state = 'confirmed') = (confirmed_at IS NOT NULL)),
    CONSTRAINT settlements_failed_consistent CHECK ((state = 'failed') = (failed_at IS NOT NULL AND failure_reason IS NOT NULL))
);

CREATE UNIQUE INDEX settlements_one_in_flight_per_user ON settlements (user_id) WHERE state IN ('queued', 'signing', 'broadcast');
CREATE UNIQUE INDEX settlements_tx_id_unique ON settlements (tx_id) WHERE tx_id IS NOT NULL;
CREATE INDEX settlements_due_idx ON settlements (network, next_attempt_at) WHERE state IN ('queued', 'signing', 'broadcast');
CREATE INDEX settlements_user_created_idx ON settlements (user_id, created_at DESC, id DESC);

CREATE FUNCTION gigjar_freeze_resolved_settlement() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' OR OLD.state IN ('confirmed', 'failed') THEN
        RAISE EXCEPTION '% on resolved settlement % is not allowed', TG_OP, OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.user_id <> OLD.user_id OR NEW.amount <> OLD.amount OR NEW.fee <> OLD.fee
       OR NEW.destination_address <> OLD.destination_address OR NEW.jar_address <> OLD.jar_address
       OR NEW.network <> OLD.network OR (OLD.tx_id IS NOT NULL AND NEW.tx_id IS DISTINCT FROM OLD.tx_id) THEN
        RAISE EXCEPTION 'immutable columns of settlement % cannot change', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER settlements_freeze
    BEFORE UPDATE OR DELETE ON settlements
    FOR EACH ROW EXECUTE FUNCTION gigjar_freeze_resolved_settlement();

ALTER TABLE journal_entries
    ADD CONSTRAINT journal_entries_settlement_fk
    FOREIGN KEY (settlement_id) REFERENCES settlements (id) ON DELETE RESTRICT;

CREATE TABLE settlement_expenses (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    settlement_id uuid NOT NULL REFERENCES settlements (id) ON DELETE RESTRICT,
    kind          text NOT NULL,
    trx_sun       bigint NOT NULL,
    reference     text NOT NULL,
    detail        jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT settlement_expenses_kind_valid CHECK (kind IN ('activation', 'energy_rental', 'burn_topup')),
    CONSTRAINT settlement_expenses_amount_non_negative CHECK (trx_sun >= 0),
    CONSTRAINT settlement_expenses_reference_unique UNIQUE (kind, reference)
);

CREATE INDEX settlement_expenses_settlement_idx ON settlement_expenses (settlement_id);

CREATE TRIGGER settlement_expenses_immutable
    BEFORE UPDATE OR DELETE ON settlement_expenses
    FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation();
