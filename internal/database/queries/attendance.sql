-- name: ListAttendanceParticipants :many
SELECT mp.user_id, u.display_name, mp.role,
       COALESCE(ar.status, 'unanswered') AS status,
       ar.updated_at
FROM meeting_participants mp
JOIN users u ON u.id = mp.user_id
LEFT JOIN attendance_responses ar
  ON ar.meeting_id = mp.meeting_id AND ar.user_id = mp.user_id
WHERE mp.meeting_id = $1 AND mp.status = 'active'
ORDER BY (mp.role = 'owner') DESC, mp.joined_at, mp.user_id
LIMIT $2 OFFSET $3;

-- name: CountAttendanceParticipants :one
SELECT count(*)::integer AS participant_count,
       count(*) FILTER (WHERE ar.status = 'going')::integer AS going_count,
       count(*) FILTER (WHERE ar.status = 'maybe')::integer AS maybe_count,
       count(*) FILTER (WHERE ar.status = 'not_going')::integer AS not_going_count,
       count(*) FILTER (WHERE ar.status IS NULL)::integer AS unanswered_count
FROM meeting_participants mp
LEFT JOIN attendance_responses ar
  ON ar.meeting_id = mp.meeting_id AND ar.user_id = mp.user_id
WHERE mp.meeting_id = $1 AND mp.status = 'active';

-- name: GetAttendanceResponse :one
SELECT status
FROM attendance_responses
WHERE meeting_id = $1 AND user_id = $2;

-- name: UpsertAttendanceResponse :exec
INSERT INTO attendance_responses (meeting_id, user_id, status)
VALUES ($1, $2, $3)
ON CONFLICT (meeting_id, user_id)
DO UPDATE SET status = EXCLUDED.status, updated_at = now();

-- name: DeleteAttendanceResponse :execrows
DELETE FROM attendance_responses
WHERE meeting_id = $1 AND user_id = $2;
