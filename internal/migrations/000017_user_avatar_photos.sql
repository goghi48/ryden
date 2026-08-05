ALTER TABLE users
    ADD COLUMN avatar_revision bigint
        CHECK (avatar_revision IS NULL OR avatar_revision > 0);

CREATE TABLE user_avatar_photos (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    content_type varchar(16) NOT NULL
        CHECK (content_type IN ('image/jpeg', 'image/png')),
    content bytea NOT NULL
        CHECK (octet_length(content) BETWEEN 1 AND 1048576),
    content_hash bytea NOT NULL
        CHECK (octet_length(content_hash) = 32),
    updated_at timestamptz NOT NULL DEFAULT now()
);
