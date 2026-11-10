DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM settlements)
       OR EXISTS (SELECT 1 FROM postings p JOIN ledger_accounts a ON a.id = p.account_id WHERE a.kind = 'platform_fees') THEN
        RAISE EXCEPTION 'refusing to roll back 000007_settlements: settlements or platform fee postings exist and would be destroyed';
    END IF;
END
$$;

DROP TABLE settlement_expenses;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_settlement_fk;
DROP TABLE settlements;
DROP FUNCTION gigjar_freeze_resolved_settlement();
ALTER TABLE ledger_accounts DISABLE TRIGGER ledger_accounts_no_delete;
DELETE FROM ledger_accounts WHERE kind = 'platform_fees';
ALTER TABLE ledger_accounts ENABLE TRIGGER ledger_accounts_no_delete;
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_kind_valid;
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_owner_matches_kind;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_kind_valid
    CHECK (kind IN ('jar_available', 'jar_reserved', 'settled', 'chain_inflow'));
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_owner_matches_kind
    CHECK ((kind = 'chain_inflow') = (user_id IS NULL));
