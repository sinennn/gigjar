CREATE TABLE ledger_accounts (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid REFERENCES users (id) ON DELETE RESTRICT,
    kind       text NOT NULL,
    balance    bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ledger_accounts_kind_valid CHECK (kind IN ('jar_available', 'jar_reserved', 'settled', 'chain_inflow')),
    CONSTRAINT ledger_accounts_owner_matches_kind CHECK ((kind = 'chain_inflow') = (user_id IS NULL)),
    CONSTRAINT ledger_accounts_user_balance_non_negative CHECK (user_id IS NULL OR balance >= 0)
);

CREATE UNIQUE INDEX ledger_accounts_user_kind_unique ON ledger_accounts (user_id, kind) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX ledger_accounts_system_kind_unique ON ledger_accounts (kind) WHERE user_id IS NULL;

CREATE TRIGGER ledger_accounts_set_updated_at
    BEFORE UPDATE ON ledger_accounts
    FOR EACH ROW EXECUTE FUNCTION gigjar_set_updated_at();

CREATE TRIGGER ledger_accounts_no_delete
    BEFORE DELETE ON ledger_accounts
    FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation();

INSERT INTO ledger_accounts (user_id, kind) VALUES (NULL, 'chain_inflow');

CREATE TABLE journal_entries (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sequence             bigint GENERATED ALWAYS AS IDENTITY,
    kind                 text NOT NULL,
    idempotency_key      text NOT NULL,
    user_id              uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    incoming_transfer_id uuid,
    settlement_id        uuid,
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT journal_entries_sequence_unique UNIQUE (sequence),
    CONSTRAINT journal_entries_idempotency_key_unique UNIQUE (idempotency_key),
    CONSTRAINT journal_entries_kind_valid CHECK (kind IN ('deposit', 'settlement_reserved', 'settlement_completed', 'settlement_released')),
    CONSTRAINT journal_entries_reference_matches_kind CHECK (
        (kind = 'deposit' AND incoming_transfer_id IS NOT NULL AND settlement_id IS NULL)
        OR (kind <> 'deposit' AND settlement_id IS NOT NULL AND incoming_transfer_id IS NULL)
    )
);

CREATE INDEX journal_entries_user_sequence_idx ON journal_entries (user_id, sequence DESC);

CREATE TABLE postings (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    journal_entry_id uuid NOT NULL REFERENCES journal_entries (id) ON DELETE RESTRICT,
    account_id       bigint NOT NULL REFERENCES ledger_accounts (id) ON DELETE RESTRICT,
    amount           bigint NOT NULL,
    CONSTRAINT postings_amount_non_zero CHECK (amount <> 0),
    CONSTRAINT postings_one_per_account_per_entry UNIQUE (journal_entry_id, account_id)
);

CREATE INDEX postings_account_idx ON postings (account_id);

CREATE TRIGGER journal_entries_immutable
    BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation();

CREATE TRIGGER postings_immutable
    BEFORE UPDATE OR DELETE ON postings
    FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation();

CREATE FUNCTION gigjar_apply_posting() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE ledger_accounts SET balance = balance + NEW.amount WHERE id = NEW.account_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER postings_apply_to_balance
    AFTER INSERT ON postings
    FOR EACH ROW EXECUTE FUNCTION gigjar_apply_posting();

CREATE FUNCTION gigjar_assert_entry_balanced() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    entry_id      uuid;
    posting_count integer;
    posting_sum   numeric;
BEGIN
    IF TG_TABLE_NAME = 'journal_entries' THEN
        entry_id := NEW.id;
    ELSE
        entry_id := NEW.journal_entry_id;
    END IF;

    SELECT count(*), coalesce(sum(amount), 0)
      INTO posting_count, posting_sum
      FROM postings
     WHERE journal_entry_id = entry_id;

    IF posting_count < 2 OR posting_sum <> 0 THEN
        RAISE EXCEPTION 'journal entry % is unbalanced: % postings summing to %', entry_id, posting_count, posting_sum
            USING ERRCODE = 'check_violation', CONSTRAINT = 'journal_entries_balanced';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER journal_entries_balanced
    AFTER INSERT ON journal_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION gigjar_assert_entry_balanced();

CREATE CONSTRAINT TRIGGER postings_balanced
    AFTER INSERT ON postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION gigjar_assert_entry_balanced();
