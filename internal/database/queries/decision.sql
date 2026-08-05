-- name: ListPlanVoteOptions :many
SELECT po.id, po.title, po.description, po.position,
       count(pv.user_id)::integer AS vote_count,
       EXISTS (
           SELECT 1
           FROM plan_votes mine
           WHERE mine.meeting_id = po.meeting_id
             AND mine.user_id = $2
             AND mine.plan_option_id = po.id
       ) AS selected_by_user
FROM plan_options po
LEFT JOIN plan_votes pv
  ON pv.meeting_id = po.meeting_id AND pv.plan_option_id = po.id
WHERE po.meeting_id = $1
GROUP BY po.id, po.title, po.description, po.position, po.meeting_id
ORDER BY po.position, po.id;

-- name: ListPlanVoteResponses :many
SELECT pv.user_id, u.display_name, pv.plan_option_id, pv.updated_at
FROM plan_votes pv
JOIN users u ON u.id = pv.user_id
WHERE pv.meeting_id = $1
ORDER BY pv.updated_at, pv.user_id;

-- name: CountActiveMeetingParticipants :one
SELECT count(*)::integer
FROM meeting_participants
WHERE meeting_id = $1 AND status = 'active';

-- name: GetCurrentPlanVote :one
SELECT pv.plan_option_id, po.title
FROM plan_votes pv
JOIN plan_options po
  ON po.meeting_id = pv.meeting_id AND po.id = pv.plan_option_id
WHERE pv.meeting_id = $1 AND pv.user_id = $2;

-- name: GetPlanOptionForVote :one
SELECT id, title
FROM plan_options
WHERE meeting_id = $1 AND id = $2;

-- name: UpsertPlanVote :exec
INSERT INTO plan_votes (meeting_id, user_id, plan_option_id)
VALUES ($1, $2, $3)
ON CONFLICT (meeting_id, user_id)
DO UPDATE SET plan_option_id = EXCLUDED.plan_option_id, updated_at = now();

-- name: DeletePlanVote :execrows
DELETE FROM plan_votes
WHERE meeting_id = $1 AND user_id = $2;

-- name: CreatePlanVoteHistory :exec
INSERT INTO plan_vote_history (
    id, meeting_id, user_id, action,
    previous_plan_option_id, previous_plan_title,
    new_plan_option_id, new_plan_title
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListPlanVoteHistory :many
SELECT h.id, h.user_id, u.display_name, h.action,
       h.previous_plan_option_id, h.previous_plan_title,
       h.new_plan_option_id, h.new_plan_title, h.created_at
FROM plan_vote_history h
JOIN users u ON u.id = h.user_id
WHERE h.meeting_id = $1
ORDER BY h.created_at DESC, h.id DESC
LIMIT $2 OFFSET $3;

-- name: CountPlanVoteHistory :one
SELECT count(*)::integer
FROM plan_vote_history
WHERE meeting_id = $1;

-- name: GetMeetingDecisionForOwnerForUpdate :one
SELECT state, selected_plan_option_id, selected_time_option_id
FROM meetings
WHERE id = $1 AND owner_id = $2
FOR UPDATE;

-- name: GetTimeOptionForDecision :one
SELECT id, plan_option_id
FROM time_options
WHERE meeting_id = $1 AND id = $2;

-- name: ScheduleMeeting :exec
UPDATE meetings
SET state = 'scheduled',
    selected_plan_option_id = $2,
    selected_time_option_id = $3,
    version = version + 1,
    updated_at = now()
WHERE id = $1;
