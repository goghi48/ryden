CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE CHECK (email = lower(email)),
    password_hash text NOT NULL,
    display_name varchar(80) NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 80),
    avatar_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    replaced_by uuid REFERENCES refresh_sessions(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX refresh_sessions_active_user_idx
    ON refresh_sessions (user_id, expires_at)
    WHERE revoked_at IS NULL;

CREATE TABLE meetings (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES users(id),
    title varchar(120) NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 120),
    description varchar(2000) NOT NULL DEFAULT '',
    cover_url text,
    location_name varchar(200),
    location_url text,
    event_type varchar(40) NOT NULL DEFAULT 'other',
    timezone varchar(64) NOT NULL,
    state varchar(16) NOT NULL DEFAULT 'draft'
        CHECK (state IN ('draft', 'collecting', 'scheduled', 'cancelled', 'completed')),
    selected_plan_option_id uuid,
    selected_time_option_id uuid,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX meetings_owner_created_idx ON meetings (owner_id, created_at DESC, id DESC);

CREATE TABLE meeting_participants (
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role varchar(16) NOT NULL CHECK (role IN ('owner', 'participant')),
    status varchar(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'left', 'removed')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (meeting_id, user_id)
);
CREATE INDEX meeting_participants_user_idx
    ON meeting_participants (user_id, joined_at DESC)
    WHERE status = 'active';

CREATE TABLE idempotency_keys (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation varchar(80) NOT NULL,
    key varchar(128) NOT NULL,
    request_hash bytea NOT NULL,
    status_code integer,
    response_body jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, operation, key),
    CHECK (expires_at > created_at)
);
CREATE INDEX idempotency_keys_expiry_idx ON idempotency_keys (expires_at);
