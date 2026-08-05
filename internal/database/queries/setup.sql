-- name: GetMeetingForOwnerForUpdate :one
SELECT m.id, m.state, m.coordination_mode, m.version,
       CASE
           WHEN m.state = 'scheduled' AND selected_time.id IS NOT NULL
               THEN COALESCE(selected_time.ends_at, selected_time.starts_at) + interval '24 hours' <= now()
           ELSE false
       END AS archived
FROM meetings m
LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
WHERE m.id = $1 AND m.owner_id = $2
FOR UPDATE OF m;

-- name: LockMeetingByID :one
SELECT id, state, coordination_mode, version
FROM meetings
WHERE id = $1
FOR UPDATE;

-- name: GetMeetingCompletionForOwnerForUpdate :one
SELECT state, version, updated_at
FROM meetings
WHERE id = $1 AND owner_id = $2
FOR UPDATE;

-- name: CountOpenRequirements :one
SELECT count(*)::integer
FROM requirements
WHERE meeting_id = $1 AND status <> 'completed';

-- name: CompleteMeeting :one
UPDATE meetings
SET state = 'completed', version = version + 1, updated_at = now()
WHERE id = $1
RETURNING version, updated_at;

-- name: GetMeetingCancellationForOwnerForUpdate :one
SELECT state, version, updated_at
FROM meetings
WHERE id = $1 AND owner_id = $2
FOR UPDATE;

-- name: CancelMeeting :one
UPDATE meetings
SET state = 'cancelled', version = version + 1, updated_at = now()
WHERE id = $1
RETURNING version, updated_at;

-- name: CountPlanOptions :one
SELECT count(*)::integer
FROM plan_options
WHERE meeting_id = $1;

-- name: CountTimeOptions :one
SELECT count(*)::integer
FROM time_options
WHERE meeting_id = $1;

-- name: NextPlanOptionPosition :one
SELECT candidate::smallint
FROM generate_series(0, 19) AS candidate
WHERE NOT EXISTS (
    SELECT 1
    FROM plan_options
    WHERE meeting_id = $1 AND position = candidate
)
ORDER BY candidate
LIMIT 1;

-- name: NextTimeOptionPosition :one
SELECT candidate::smallint
FROM generate_series(0, 19) AS candidate
WHERE NOT EXISTS (
    SELECT 1
    FROM time_options
    WHERE meeting_id = $1 AND position = candidate
)
ORDER BY candidate
LIMIT 1;

-- name: CreatePlanOption :one
INSERT INTO plan_options (
    id, meeting_id, title, description, position, idempotency_key
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (meeting_id, idempotency_key) DO NOTHING
RETURNING id, meeting_id, title, description, position, idempotency_key, created_at;

-- name: GetPlanOptionByIdempotencyKey :one
SELECT id, meeting_id, title, description, position, idempotency_key, created_at
FROM plan_options
WHERE meeting_id = $1 AND idempotency_key = $2;

-- name: ListPlanOptions :many
SELECT id, meeting_id, title, description, position, idempotency_key, created_at
FROM plan_options
WHERE meeting_id = $1
ORDER BY position, created_at, id;

-- name: DeletePlanOption :execrows
DELETE FROM plan_options
WHERE id = $1 AND meeting_id = $2;

-- name: UpdatePlanOption :one
UPDATE plan_options
SET title = $3,
    description = $4
WHERE id = $1 AND meeting_id = $2
RETURNING id, meeting_id, title, description, position, idempotency_key, created_at;

-- name: CreateTimeOption :one
INSERT INTO time_options (
    id, meeting_id, plan_option_id, starts_at, ends_at, position, idempotency_key
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (meeting_id, idempotency_key) DO NOTHING
RETURNING id, meeting_id, starts_at, ends_at, position, idempotency_key, created_at, plan_option_id;

-- name: GetTimeOptionByIdempotencyKey :one
SELECT id, meeting_id, starts_at, ends_at, position, idempotency_key, created_at, plan_option_id
FROM time_options
WHERE meeting_id = $1 AND idempotency_key = $2;

-- name: ListTimeOptions :many
SELECT id, meeting_id, starts_at, ends_at, position, idempotency_key, created_at, plan_option_id
FROM time_options
WHERE meeting_id = $1
ORDER BY position, starts_at, id;

-- name: PlanOptionBelongsToMeeting :one
SELECT EXISTS (
    SELECT 1
    FROM plan_options
    WHERE id = $1 AND meeting_id = $2
);

-- name: DeleteTimeOption :execrows
DELETE FROM time_options
WHERE id = $1 AND meeting_id = $2;

-- name: UpdateTimeOption :one
UPDATE time_options
SET plan_option_id = $3,
    starts_at = $4,
    ends_at = $5
WHERE id = $1 AND meeting_id = $2
RETURNING id, meeting_id, starts_at, ends_at, position, idempotency_key, created_at, plan_option_id;

-- name: ListMeetingParticipants :many
SELECT u.id AS user_id, u.display_name, mp.role, mp.joined_at
FROM meeting_participants mp
JOIN users u ON u.id = mp.user_id
WHERE mp.meeting_id = $1 AND mp.status = 'active'
ORDER BY (mp.role = 'owner') DESC, mp.joined_at, u.id;

-- name: TouchMeeting :exec
UPDATE meetings
SET version = version + 1, updated_at = now()
WHERE id = $1;

-- name: OpenMeetingCollection :exec
UPDATE meetings
SET state = 'collecting', version = version + 1, updated_at = now()
WHERE id = $1;

-- name: RevokeActiveInvitations :execrows
UPDATE invitations
SET revoked_at = now()
WHERE meeting_id = $1 AND revoked_at IS NULL;

-- name: CreateInvitation :one
INSERT INTO invitations (
    id, meeting_id, created_by, secret_hash, idempotency_key, expires_at
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (meeting_id, idempotency_key) DO NOTHING
RETURNING id, meeting_id, created_by, secret_hash, idempotency_key, default_role,
          expires_at, revoked_at, created_at;

-- name: GetInvitationByIdempotencyKey :one
SELECT id, meeting_id, created_by, secret_hash, idempotency_key, default_role,
       expires_at, revoked_at, created_at
FROM invitations
WHERE meeting_id = $1 AND idempotency_key = $2;

-- name: GetActiveInvitationExpiry :one
SELECT expires_at
FROM invitations
WHERE meeting_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: GetInvitationMeetingByHash :one
SELECT meeting_id
FROM invitations
WHERE secret_hash = $1;

-- name: GetValidInvitationForUpdate :one
SELECT id, meeting_id, created_by, secret_hash, idempotency_key, default_role,
       expires_at, revoked_at, created_at
FROM invitations
WHERE meeting_id = $1
  AND secret_hash = $2
  AND revoked_at IS NULL
  AND expires_at > now()
FOR UPDATE;

-- name: JoinMeeting :execrows
INSERT INTO meeting_participants (meeting_id, user_id, role, status)
VALUES ($1, $2, 'participant', 'active')
ON CONFLICT (meeting_id, user_id)
DO UPDATE SET status = 'active'
WHERE meeting_participants.status <> 'active';
