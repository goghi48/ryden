ALTER TABLE meetings
    ADD COLUMN coordination_mode varchar(16) NOT NULL DEFAULT 'planning'
        CHECK (coordination_mode IN ('planning', 'fixed'));

CREATE TABLE attendance_responses (
    meeting_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status varchar(16) NOT NULL CHECK (status IN ('going', 'not_going')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (meeting_id, user_id),
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants (meeting_id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX attendance_responses_meeting_status_idx
    ON attendance_responses (meeting_id, status);
