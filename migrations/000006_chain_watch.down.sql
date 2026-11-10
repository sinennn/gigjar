DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM journal_entries WHERE incoming_transfer_id IS NOT NULL) THEN
        RAISE EXCEPTION 'refusing to roll back 000006_chain_watch: ledger deposits reference incoming transfers';
    END IF;
END
$$;

DROP TABLE simulated_chain_transfers;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_incoming_transfer_fk;
DROP TABLE incoming_transfers;
DROP FUNCTION gigjar_freeze_resolved_transfer();
DROP TABLE jar_watch_state;
