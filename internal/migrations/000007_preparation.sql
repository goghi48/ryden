CREATE TABLE requirements (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    created_by uuid NOT NULL,
    name varchar(120) NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    required_quantity integer NOT NULL CHECK (required_quantity BETWEEN 1 AND 100000),
    status varchar(16) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'completed')),
    idempotency_key varchar(128) NOT NULL,
    request_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meeting_id, id),
    UNIQUE (meeting_id, idempotency_key),
    FOREIGN KEY (meeting_id, created_by)
        REFERENCES meeting_participants (meeting_id, user_id)
);

CREATE UNIQUE INDEX requirements_meeting_name_idx
    ON requirements (meeting_id, lower(name));

CREATE INDEX requirements_meeting_status_created_idx
    ON requirements (meeting_id, status, created_at, id);

CREATE TABLE requirement_claims (
    meeting_id uuid NOT NULL,
    requirement_id uuid NOT NULL,
    user_id uuid NOT NULL,
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 100000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (requirement_id, user_id),
    FOREIGN KEY (meeting_id, requirement_id)
        REFERENCES requirements (meeting_id, id) ON DELETE CASCADE,
    FOREIGN KEY (meeting_id, user_id)
        REFERENCES meeting_participants (meeting_id, user_id) ON DELETE CASCADE
);

CREATE INDEX requirement_claims_meeting_user_idx
    ON requirement_claims (meeting_id, user_id, requirement_id);
