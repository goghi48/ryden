CREATE TABLE polls (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    question varchar(200) NOT NULL CHECK (length(trim(question)) BETWEEN 1 AND 200),
    response_mode varchar(16) NOT NULL CHECK (response_mode IN ('single', 'multiple')),
    deadline timestamptz,
    state varchar(16) NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')),
    selected_option_id uuid,
    idempotency_key varchar(128) NOT NULL,
    request_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    UNIQUE (meeting_id, id),
    UNIQUE (meeting_id, idempotency_key),
    CHECK ((state = 'open' AND closed_at IS NULL AND selected_option_id IS NULL)
        OR (state = 'closed' AND closed_at IS NOT NULL AND selected_option_id IS NOT NULL))
);
CREATE INDEX polls_meeting_created_idx ON polls (meeting_id, created_at, id);

CREATE TABLE poll_options (
    id uuid PRIMARY KEY,
    poll_id uuid NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    label varchar(120) NOT NULL CHECK (length(trim(label)) BETWEEN 1 AND 120),
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 9),
    UNIQUE (poll_id, id),
    UNIQUE (poll_id, position)
);

ALTER TABLE polls
    ADD CONSTRAINT polls_selected_option_fk
    FOREIGN KEY (id, selected_option_id)
    REFERENCES poll_options (poll_id, id);

CREATE TABLE poll_votes (
    poll_id uuid NOT NULL,
    option_id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (poll_id, option_id, user_id),
    FOREIGN KEY (poll_id, option_id)
        REFERENCES poll_options (poll_id, id) ON DELETE CASCADE
);
CREATE INDEX poll_votes_user_poll_idx ON poll_votes (user_id, poll_id);
