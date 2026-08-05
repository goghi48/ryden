CREATE TABLE poll_vote_history (
    id uuid PRIMARY KEY,
    poll_id uuid NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action text NOT NULL CHECK (action IN ('cast', 'change', 'retract')),
    previous_option_ids uuid[] NOT NULL DEFAULT '{}',
    previous_option_labels text[] NOT NULL DEFAULT '{}',
    new_option_ids uuid[] NOT NULL DEFAULT '{}',
    new_option_labels text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(previous_option_ids) = cardinality(previous_option_labels)),
    CHECK (cardinality(new_option_ids) = cardinality(new_option_labels)),
    CHECK (
        (action = 'cast'
            AND cardinality(previous_option_ids) = 0
            AND cardinality(new_option_ids) > 0)
        OR
        (action = 'change'
            AND cardinality(previous_option_ids) > 0
            AND cardinality(new_option_ids) > 0)
        OR
        (action = 'retract'
            AND cardinality(previous_option_ids) > 0
            AND cardinality(new_option_ids) = 0)
    )
);

CREATE INDEX poll_vote_history_poll_created_idx
    ON poll_vote_history (poll_id, created_at DESC, id DESC);
