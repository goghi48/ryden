-- name: CountMeetingNotes :one
SELECT count(*)::integer
FROM meeting_notes mn
JOIN meeting_participants mp
  ON mp.meeting_id = mn.meeting_id
 AND mp.user_id = mn.user_id
 AND mp.status = 'active'
WHERE mn.meeting_id = $1;

-- name: ListMeetingNotes :many
SELECT mn.user_id, u.display_name, mn.text, mn.created_at, mn.updated_at
FROM meeting_notes mn
JOIN users u ON u.id = mn.user_id
JOIN meeting_participants mp
  ON mp.meeting_id = mn.meeting_id
 AND mp.user_id = mn.user_id
 AND mp.status = 'active'
WHERE mn.meeting_id = $1
ORDER BY mn.updated_at DESC, mn.user_id
LIMIT $2 OFFSET $3;

-- name: GetMeetingForNoteMutation :one
SELECT m.state,
       CASE
           WHEN m.state = 'scheduled' AND selected_time.id IS NOT NULL
               THEN COALESCE(selected_time.ends_at, selected_time.starts_at) + interval '24 hours' <= now()
           ELSE false
       END AS archived
FROM meetings m
JOIN meeting_participants mp
  ON mp.meeting_id = m.id AND mp.user_id = $2 AND mp.status = 'active'
LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
WHERE m.id = $1
FOR UPDATE OF m;

-- name: UpsertMeetingNote :one
INSERT INTO meeting_notes (meeting_id, user_id, text)
VALUES ($1, $2, $3)
ON CONFLICT (meeting_id, user_id)
DO UPDATE SET text = EXCLUDED.text, updated_at = now()
WHERE meeting_notes.text IS DISTINCT FROM EXCLUDED.text
RETURNING true AS changed;

-- name: DeleteMeetingNote :execrows
DELETE FROM meeting_notes
WHERE meeting_id = $1 AND user_id = $2;
