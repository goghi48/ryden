-- name: CountMeetingPolls :one
SELECT count(*)::integer
FROM polls
WHERE meeting_id = $1;

-- name: CreatePoll :one
INSERT INTO polls (
    id, meeting_id, created_by_user_id, question, response_mode, deadline,
    is_anonymous, allow_revote, idempotency_key, request_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, meeting_id, question, response_mode, deadline, state,
          selected_option_id, idempotency_key, request_hash, created_at, closed_at,
          created_by_user_id, is_anonymous, allow_revote;

-- name: CreatePollOption :exec
INSERT INTO poll_options (id, poll_id, label, position)
VALUES ($1, $2, $3, $4);

-- name: GetPollByIdempotencyKey :one
SELECT id, meeting_id, question, response_mode, deadline, state,
       selected_option_id, idempotency_key, request_hash, created_at, closed_at,
       created_by_user_id, is_anonymous, allow_revote
FROM polls
WHERE meeting_id = $1 AND idempotency_key = $2;

-- name: ListPollsForMeeting :many
SELECT id, meeting_id, question, response_mode, deadline, state,
       selected_option_id, idempotency_key, request_hash, created_at, closed_at,
       created_by_user_id, is_anonymous, allow_revote
FROM polls
WHERE meeting_id = $1
ORDER BY created_at, id;

-- name: ListPollOptionsWithResults :many
SELECT po.id, po.poll_id, po.label, po.position,
       count(pv.user_id)::integer AS vote_count,
       EXISTS (
           SELECT 1
           FROM poll_votes own_vote
           WHERE own_vote.poll_id = po.poll_id
             AND own_vote.option_id = po.id
             AND own_vote.user_id = $2
       ) AS selected_by_user
FROM poll_options po
LEFT JOIN poll_votes pv
  ON pv.poll_id = po.poll_id AND pv.option_id = po.id
WHERE po.poll_id = $1
GROUP BY po.id, po.poll_id, po.label, po.position
ORDER BY po.position, po.id;

-- name: ListPollVoters :many
SELECT pv.option_id, u.id AS user_id, u.display_name, pv.updated_at
FROM poll_votes pv
JOIN poll_options po ON po.id = pv.option_id AND po.poll_id = pv.poll_id
JOIN users u ON u.id = pv.user_id
WHERE pv.poll_id = $1
ORDER BY po.position, lower(u.display_name), u.id;

-- name: CountPollRespondents :one
SELECT count(DISTINCT user_id)::integer
FROM poll_votes
WHERE poll_id = $1;

-- name: DeleteDraftPoll :execrows
DELETE FROM polls
WHERE id = $1 AND meeting_id = $2;

-- name: GetPollMeetingID :one
SELECT meeting_id
FROM polls
WHERE id = $1;

-- name: GetPollForParticipantForUpdate :one
SELECT p.id, p.meeting_id, p.response_mode, p.deadline, p.state,
       p.allow_revote, m.state AS meeting_state
FROM polls p
JOIN meetings m ON m.id = p.meeting_id
JOIN meeting_participants mp ON mp.meeting_id = p.meeting_id
WHERE p.id = $1 AND mp.user_id = $2 AND mp.status = 'active'
FOR UPDATE OF p;

-- name: CountPollOptionsByIDs :one
SELECT count(*)::integer
FROM poll_options
WHERE poll_id = $1 AND id = ANY($2::uuid[]);

-- name: ListUserPollVoteOptions :many
SELECT po.id, po.label, po.position
FROM poll_votes pv
JOIN poll_options po ON po.poll_id = pv.poll_id AND po.id = pv.option_id
WHERE pv.poll_id = $1 AND pv.user_id = $2
ORDER BY po.position, po.id;

-- name: ListPollOptionsByIDs :many
SELECT id, label, position
FROM poll_options
WHERE poll_id = $1 AND id = ANY($2::uuid[])
ORDER BY position, id;

-- name: DeleteUserPollVotes :exec
DELETE FROM poll_votes
WHERE poll_id = $1 AND user_id = $2;

-- name: CreatePollVote :exec
INSERT INTO poll_votes (poll_id, option_id, user_id)
VALUES ($1, $2, $3);

-- name: CreatePollVoteHistory :exec
INSERT INTO poll_vote_history (
    id, poll_id, user_id, action,
    previous_option_ids, previous_option_labels,
    new_option_ids, new_option_labels
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: PollVisibleToUser :one
SELECT EXISTS (
    SELECT 1
    FROM polls p
    JOIN meeting_participants mp ON mp.meeting_id = p.meeting_id
    WHERE p.id = $1 AND mp.user_id = $2 AND mp.status = 'active'
      AND NOT p.is_anonymous
);

-- name: ListPollVoteHistory :many
SELECT pvh.id, pvh.user_id, u.display_name, pvh.action,
       pvh.previous_option_ids, pvh.previous_option_labels,
       pvh.new_option_ids, pvh.new_option_labels, pvh.created_at
FROM poll_vote_history pvh
JOIN users u ON u.id = pvh.user_id
WHERE pvh.poll_id = $1
ORDER BY pvh.created_at DESC, pvh.id DESC
LIMIT $2 OFFSET $3;

-- name: CountPollVoteHistory :one
SELECT count(*)::integer
FROM poll_vote_history
WHERE poll_id = $1;

-- name: GetPollForManagerForUpdate :one
SELECT p.id, p.meeting_id, p.state, p.selected_option_id, m.state AS meeting_state
FROM polls p
JOIN meetings m ON m.id = p.meeting_id
WHERE p.id = $1 AND p.meeting_id = $2
  AND (m.owner_id = $3 OR p.created_by_user_id = $3)
FOR UPDATE OF p;

-- name: PollOptionBelongsToPoll :one
SELECT EXISTS (
    SELECT 1 FROM poll_options WHERE id = $1 AND poll_id = $2
);

-- name: ClosePoll :exec
UPDATE polls
SET state = 'closed', selected_option_id = $2, closed_at = now()
WHERE id = $1;
