CREATE TABLE availability_votes (
    meeting_id uuid NOT NULL,
    time_option_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status varchar(16) NOT NULL
        CHECK (status IN ('preferred', 'available', 'if_needed', 'unavailable')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (time_option_id, user_id),
    FOREIGN KEY (meeting_id, time_option_id)
        REFERENCES time_options (meeting_id, id) ON DELETE CASCADE,
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants (meeting_id, user_id) ON DELETE CASCADE
);

CREATE INDEX availability_votes_meeting_idx
    ON availability_votes (meeting_id, time_option_id, user_id);
