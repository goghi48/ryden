CREATE TABLE meeting_notes (
    meeting_id uuid NOT NULL,
    user_id uuid NOT NULL,
    text varchar(200) NOT NULL CHECK (char_length(btrim(text)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (meeting_id, user_id),
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants(meeting_id, user_id) ON DELETE CASCADE
);

CREATE INDEX meeting_notes_meeting_updated_idx
    ON meeting_notes (meeting_id, updated_at DESC, user_id);
