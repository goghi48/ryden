-- name: GetRequirementByIdempotencyKey :one
SELECT id, meeting_id, created_by, name, required_quantity, status,
       idempotency_key, request_hash, created_at, updated_at
FROM requirements
WHERE meeting_id = $1 AND idempotency_key = $2;

-- name: CountMeetingRequirements :one
SELECT count(*)::integer
FROM requirements
WHERE meeting_id = $1;

-- name: CreateRequirement :one
INSERT INTO requirements (
    id, meeting_id, created_by, name, required_quantity, idempotency_key, request_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, meeting_id, created_by, name, required_quantity, status,
          idempotency_key, request_hash, created_at, updated_at;

-- name: ListRequirements :many
SELECT r.id, r.name, r.required_quantity,
       COALESCE(sum(c.quantity), 0)::integer AS claimed_quantity,
       r.status,
       COALESCE(mine.quantity, 0)::integer AS my_quantity,
       r.created_at, r.updated_at
FROM requirements r
LEFT JOIN requirement_claims c
  ON c.meeting_id = r.meeting_id AND c.requirement_id = r.id
LEFT JOIN requirement_claims mine
  ON mine.meeting_id = r.meeting_id
 AND mine.requirement_id = r.id
 AND mine.user_id = $2
WHERE r.meeting_id = $1
GROUP BY r.id, mine.quantity
ORDER BY (r.status = 'completed'), r.created_at, r.id
LIMIT $3 OFFSET $4;

-- name: ListRequirementClaims :many
SELECT c.requirement_id, c.user_id, u.display_name, c.quantity, c.updated_at
FROM requirement_claims c
JOIN users u ON u.id = c.user_id
WHERE c.meeting_id = $1
ORDER BY c.requirement_id, c.created_at, c.user_id;

-- name: CountRequirementStatuses :one
SELECT count(*)::integer AS total,
       count(*) FILTER (WHERE status = 'open')::integer AS open_count,
       count(*) FILTER (WHERE status = 'completed')::integer AS completed_count
FROM requirements
WHERE meeting_id = $1;

-- name: GetRequirementForUpdate :one
SELECT id, meeting_id, name, required_quantity, status, created_at, updated_at
FROM requirements
WHERE meeting_id = $1 AND id = $2
FOR UPDATE;

-- name: SumRequirementClaims :one
SELECT COALESCE(sum(quantity), 0)::integer
FROM requirement_claims
WHERE requirement_id = $1;

-- name: GetRequirementClaim :one
SELECT quantity
FROM requirement_claims
WHERE requirement_id = $1 AND user_id = $2;

-- name: UpsertRequirementClaim :exec
INSERT INTO requirement_claims (meeting_id, requirement_id, user_id, quantity)
VALUES ($1, $2, $3, $4)
ON CONFLICT (requirement_id, user_id)
DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = now();

-- name: DeleteRequirementClaim :execrows
DELETE FROM requirement_claims
WHERE requirement_id = $1 AND user_id = $2;

-- name: SetRequirementStatus :exec
UPDATE requirements
SET status = $3, updated_at = now()
WHERE meeting_id = $1 AND id = $2;

-- name: UpdateRequirement :execrows
UPDATE requirements
SET name = $3, required_quantity = $4, updated_at = now()
WHERE meeting_id = $1 AND id = $2;

-- name: DeleteRequirement :execrows
DELETE FROM requirements
WHERE meeting_id = $1 AND id = $2;
