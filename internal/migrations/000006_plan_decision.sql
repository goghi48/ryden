CREATE TABLE plan_votes (
    meeting_id uuid NOT NULL,
    user_id uuid NOT NULL,
    plan_option_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (meeting_id, user_id),
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants (meeting_id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (meeting_id, plan_option_id)
        REFERENCES plan_options (meeting_id, id) ON DELETE CASCADE
);

CREATE INDEX plan_votes_meeting_option_idx
    ON plan_votes (meeting_id, plan_option_id, user_id);

CREATE TABLE plan_vote_history (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL,
    user_id uuid NOT NULL,
    action varchar(12) NOT NULL CHECK (action IN ('cast', 'change', 'retract')),
    previous_plan_option_id uuid,
    previous_plan_title varchar(120),
    new_plan_option_id uuid,
    new_plan_title varchar(120),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants (meeting_id, user_id) ON DELETE CASCADE,
    CHECK (
        (action = 'cast' AND previous_plan_option_id IS NULL AND previous_plan_title IS NULL
            AND new_plan_option_id IS NOT NULL AND new_plan_title IS NOT NULL)
        OR
        (action = 'change' AND previous_plan_option_id IS NOT NULL AND previous_plan_title IS NOT NULL
            AND new_plan_option_id IS NOT NULL AND new_plan_title IS NOT NULL
            AND previous_plan_option_id <> new_plan_option_id)
        OR
        (action = 'retract' AND previous_plan_option_id IS NOT NULL AND previous_plan_title IS NOT NULL
            AND new_plan_option_id IS NULL AND new_plan_title IS NULL)
    )
);

CREATE INDEX plan_vote_history_meeting_created_idx
    ON plan_vote_history (meeting_id, created_at DESC, id DESC);
