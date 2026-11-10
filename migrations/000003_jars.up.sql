CREATE SEQUENCE jar_derivation_index_seq START WITH 1;

CREATE TABLE jars (
    user_id          uuid PRIMARY KEY REFERENCES users (id) ON DELETE RESTRICT,
    network          text NOT NULL,
    derivation_index bigint NOT NULL DEFAULT nextval('jar_derivation_index_seq'),
    address          text,
    provisioned_at   timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT jars_network_valid CHECK (network IN ('tron-mainnet', 'tron-nile', 'simulated')),
    CONSTRAINT jars_derivation_index_unique UNIQUE (derivation_index),
    CONSTRAINT jars_address_unique UNIQUE (address),
    CONSTRAINT jars_address_format CHECK (address IS NULL OR address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$'),
    CONSTRAINT jars_provisioning_consistent CHECK ((address IS NULL) = (provisioned_at IS NULL))
);

ALTER SEQUENCE jar_derivation_index_seq OWNED BY jars.derivation_index;

CREATE TABLE cleva_destinations (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    address    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cleva_destinations_address_format CHECK (address ~ '^T[1-9A-HJ-NP-Za-km-z]{33}$')
);

CREATE INDEX cleva_destinations_user_latest_idx ON cleva_destinations (user_id, id DESC);

CREATE TRIGGER cleva_destinations_immutable
    BEFORE UPDATE OR DELETE ON cleva_destinations
    FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation();
