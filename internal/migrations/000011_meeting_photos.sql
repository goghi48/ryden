CREATE TABLE meeting_photos (
    meeting_id uuid PRIMARY KEY REFERENCES meetings (id) ON DELETE CASCADE,
    content_type varchar(16) NOT NULL
        CHECK (content_type IN ('image/jpeg', 'image/png')),
    content bytea NOT NULL
        CHECK (octet_length(content) BETWEEN 1 AND 3145728),
    content_hash bytea NOT NULL
        CHECK (octet_length(content_hash) = 32),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plan_option_photos (
    plan_option_id uuid PRIMARY KEY REFERENCES plan_options (id) ON DELETE CASCADE,
    content_type varchar(16) NOT NULL
        CHECK (content_type IN ('image/jpeg', 'image/png')),
    content bytea NOT NULL
        CHECK (octet_length(content) BETWEEN 1 AND 3145728),
    content_hash bytea NOT NULL
        CHECK (octet_length(content_hash) = 32),
    updated_at timestamptz NOT NULL DEFAULT now()
);
