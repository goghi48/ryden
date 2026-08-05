CREATE TABLE plan_options (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    title varchar(120) NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 120),
    description varchar(500) NOT NULL DEFAULT '',
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 19),
    idempotency_key varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meeting_id, id),
    UNIQUE (meeting_id, idempotency_key),
    UNIQUE (meeting_id, position)
);

CREATE TABLE time_options (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 19),
    idempotency_key varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    UNIQUE (meeting_id, id),
    UNIQUE (meeting_id, idempotency_key),
    UNIQUE (meeting_id, position),
    UNIQUE (meeting_id, starts_at, ends_at)
);

CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users(id),
    secret_hash bytea NOT NULL UNIQUE,
    idempotency_key varchar(128) NOT NULL,
    default_role varchar(16) NOT NULL DEFAULT 'participant'
        CHECK (default_role = 'participant'),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    UNIQUE (meeting_id, idempotency_key)
);
CREATE INDEX invitations_active_meeting_idx
    ON invitations (meeting_id, expires_at)
    WHERE revoked_at IS NULL;

ALTER TABLE meetings
    ADD CONSTRAINT meetings_selected_plan_option_fk
    FOREIGN KEY (id, selected_plan_option_id)
    REFERENCES plan_options (meeting_id, id);

ALTER TABLE meetings
    ADD CONSTRAINT meetings_selected_time_option_fk
    FOREIGN KEY (id, selected_time_option_id)
    REFERENCES time_options (meeting_id, id);
