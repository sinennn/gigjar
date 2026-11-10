DROP TABLE simulated_chain_transfers;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_incoming_transfer_fk;
DROP TABLE incoming_transfers;
DROP FUNCTION gigjar_freeze_resolved_transfer();
DROP TABLE jar_watch_state;
