ALTER TABLE users
    ADD COLUMN nickname varchar(24);

UPDATE users
SET nickname = 'user_' || left(replace(id::text, '-', ''), 12)
WHERE nickname IS NULL;

ALTER TABLE users
    ALTER COLUMN nickname SET NOT NULL,
    ADD CONSTRAINT users_nickname_format_check CHECK (
        nickname ~ '^[a-z][a-z0-9_]{2,23}$'
        AND nickname !~ '__'
        AND nickname !~ '_$'
    ),
    ADD CONSTRAINT users_nickname_key UNIQUE (nickname);

CREATE INDEX users_nickname_prefix_idx
    ON users (nickname text_pattern_ops);

CREATE TABLE friendships (
    id uuid PRIMARY KEY,
    requester_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    addressee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted')),
    responded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (requester_id <> addressee_id),
    CHECK (
        (status = 'pending' AND responded_at IS NULL)
        OR (status = 'accepted' AND responded_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX friendships_user_pair_key
    ON friendships (
        LEAST(requester_id, addressee_id),
        GREATEST(requester_id, addressee_id)
    );

CREATE INDEX friendships_requester_status_created_idx
    ON friendships (requester_id, status, created_at DESC);

CREATE INDEX friendships_addressee_status_created_idx
    ON friendships (addressee_id, status, created_at DESC);
