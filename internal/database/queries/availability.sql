-- name: GetTimeOptionMeetingID :one
SELECT meeting_id
FROM time_options
WHERE id = $1;

-- name: ListAvailabilityVotes :many
SELECT time_option_id, user_id, status
FROM availability_votes
WHERE meeting_id = $1
ORDER BY time_option_id, user_id;

-- name: UpsertAvailabilityVote :execrows
INSERT INTO availability_votes (
    meeting_id, time_option_id, user_id, status
) VALUES ($1, $2, $3, $4)
ON CONFLICT (time_option_id, user_id)
DO UPDATE SET status = EXCLUDED.status, updated_at = now()
WHERE availability_votes.status IS DISTINCT FROM EXCLUDED.status;

-- name: DeleteAvailabilityVote :execrows
DELETE FROM availability_votes
WHERE meeting_id = $1 AND time_option_id = $2 AND user_id = $3;
