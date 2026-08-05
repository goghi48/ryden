CREATE TABLE meeting_friend_invitations (
    id uuid PRIMARY KEY,
    meeting_id uuid NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    invited_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status varchar(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined')),
    created_at timestamptz NOT NULL DEFAULT now(),
    responded_at timestamptz,
    CHECK (invited_by <> invitee_id),
    CHECK ((status = 'pending') = (responded_at IS NULL)),
    UNIQUE (meeting_id, invitee_id)
);

CREATE INDEX meeting_friend_invitations_pending_invitee_idx
    ON meeting_friend_invitations (invitee_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE INDEX meeting_friend_invitations_meeting_idx
    ON meeting_friend_invitations (meeting_id, created_at DESC, id DESC);
