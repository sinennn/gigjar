DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE password_hash IS NULL) THEN
        RAISE EXCEPTION 'refusing to roll back 000008_user_identities: some users can only sign in with Google';
    END IF;
END
$$;

DROP TABLE user_identities;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
