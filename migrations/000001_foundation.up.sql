CREATE EXTENSION IF NOT EXISTS citext;

CREATE FUNCTION gigjar_set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE FUNCTION gigjar_forbid_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'rows in % are immutable; record a compensating entry instead', TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$;
