-- name: ReserveIdempotencyKey :execrows
INSERT INTO idempotency_keys (
    user_id, operation, key, request_hash, expires_at
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, operation, key) DO NOTHING;

-- name: GetIdempotencyKey :one
SELECT user_id, operation, key, request_hash, status_code, response_body, created_at, expires_at
FROM idempotency_keys
WHERE user_id = $1 AND operation = $2 AND key = $3;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET status_code = $4, response_body = $5
WHERE user_id = $1 AND operation = $2 AND key = $3;

-- name: CreateMeeting :one
INSERT INTO meetings (
    id, owner_id, title, description, event_type, coordination_mode,
    cover_url, location_name, location_url, timezone
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, owner_id, title, description, cover_url, location_name, location_url,
          event_type, coordination_mode, timezone, state,
          selected_plan_option_id, selected_time_option_id,
          version, created_at, updated_at;

-- name: UpdateDraftMeeting :one
UPDATE meetings
SET title = $2,
    description = $3,
    event_type = $4,
    cover_url = $5,
    location_name = $6,
    location_url = $7,
    version = version + 1,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_id, title, description, cover_url, location_name, location_url,
          event_type, coordination_mode, timezone, state,
          selected_plan_option_id, selected_time_option_id,
          version, created_at, updated_at;

-- name: AddMeetingParticipant :exec
INSERT INTO meeting_participants (meeting_id, user_id, role, status)
VALUES ($1, $2, $3, 'active');

-- name: ListMeetingsForUser :many
SELECT m.id, m.owner_id, m.title, m.description, m.cover_url, m.location_name,
       m.location_url, m.event_type, m.coordination_mode, m.timezone, m.state, m.selected_plan_option_id,
       m.selected_time_option_id, m.version, m.created_at, m.updated_at,
       mp.joined_at AS participant_joined_at,
       selected_time.starts_at AS selected_starts_at,
       selected_time.ends_at AS selected_ends_at,
       CASE
           WHEN m.coordination_mode = 'fixed' THEN COALESCE(ar.status, 'unanswered')
           ELSE ''
       END AS my_attendance_status
FROM meetings m
JOIN meeting_participants mp ON mp.meeting_id = m.id
LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
LEFT JOIN attendance_responses ar ON ar.meeting_id = m.id AND ar.user_id = mp.user_id
WHERE mp.user_id = $1 AND mp.status = 'active'
ORDER BY m.created_at DESC, m.id DESC
LIMIT $2 OFFSET $3;

-- name: GetMeetingForUser :one
SELECT m.id, m.owner_id, m.title, m.description, m.cover_url, m.location_name,
       m.location_url, m.event_type, m.coordination_mode, m.timezone, m.state, m.selected_plan_option_id,
       m.selected_time_option_id, m.version, m.created_at, m.updated_at,
       mp.role AS participant_role
FROM meetings m
JOIN meeting_participants mp ON mp.meeting_id = m.id
WHERE m.id = $1 AND mp.user_id = $2 AND mp.status = 'active';
