-- name: SearchUsersByNickname :many
SELECT u.id, u.nickname, u.display_name, u.avatar_url, u.avatar_revision,
       f.id AS friendship_id,
       CASE
           WHEN f.status = 'accepted' THEN 'friend'
           WHEN f.status = 'pending' AND f.requester_id = sqlc.arg(user_id) THEN 'outgoing'
           WHEN f.status = 'pending' THEN 'incoming'
           ELSE 'none'
       END AS relationship
FROM users u
LEFT JOIN friendships f
  ON (f.requester_id = sqlc.arg(user_id) AND f.addressee_id = u.id)
  OR (f.addressee_id = sqlc.arg(user_id) AND f.requester_id = u.id)
WHERE u.id <> sqlc.arg(user_id)
  AND u.nickname LIKE replace(sqlc.arg(nickname_prefix)::text, '_', '\_') || '%' ESCAPE '\'
ORDER BY (u.nickname = sqlc.arg(nickname_prefix)::text) DESC, u.nickname
LIMIT sqlc.arg(result_limit);

-- name: GetFriendshipByPair :one
SELECT id, requester_id, addressee_id, status, responded_at, created_at, updated_at
FROM friendships
WHERE (requester_id = $1 AND addressee_id = $2)
   OR (requester_id = $2 AND addressee_id = $1);

-- name: InsertFriendRequest :one
INSERT INTO friendships (id, requester_id, addressee_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING
RETURNING id, requester_id, addressee_id, status, responded_at, created_at, updated_at;

-- name: GetFriendshipByIDForUpdate :one
SELECT id, requester_id, addressee_id, status, responded_at, created_at, updated_at
FROM friendships
WHERE id = $1
FOR UPDATE;

-- name: AcceptFriendRequest :one
UPDATE friendships
SET status = 'accepted', responded_at = now(), updated_at = now()
WHERE id = $1 AND addressee_id = $2 AND status = 'pending'
RETURNING id, requester_id, addressee_id, status, responded_at, created_at, updated_at;

-- name: DeletePendingFriendRequest :execrows
DELETE FROM friendships
WHERE id = $1
  AND status = 'pending'
  AND (requester_id = $2 OR addressee_id = $2);

-- name: DeleteAcceptedFriendship :execrows
DELETE FROM friendships
WHERE status = 'accepted'
  AND ((requester_id = $1 AND addressee_id = $2)
    OR (requester_id = $2 AND addressee_id = $1));

-- name: CountFriends :one
SELECT count(*)
FROM friendships
WHERE status = 'accepted'
  AND (requester_id = $1 OR addressee_id = $1);

-- name: ListFriends :many
SELECT f.id,
       u.id AS user_id, u.nickname, u.display_name, u.avatar_url, u.avatar_revision,
       f.updated_at
FROM friendships f
JOIN users u ON u.id = CASE
    WHEN f.requester_id = sqlc.arg(user_id) THEN f.addressee_id
    ELSE f.requester_id
END
WHERE f.status = 'accepted'
  AND (f.requester_id = sqlc.arg(user_id) OR f.addressee_id = sqlc.arg(user_id))
ORDER BY lower(u.display_name), u.nickname, u.id
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: CountIncomingFriendRequests :one
SELECT count(*)
FROM friendships
WHERE addressee_id = $1 AND status = 'pending';

-- name: ListIncomingFriendRequests :many
SELECT f.id,
       u.id AS user_id, u.nickname, u.display_name, u.avatar_url, u.avatar_revision,
       f.created_at
FROM friendships f
JOIN users u ON u.id = f.requester_id
WHERE f.addressee_id = sqlc.arg(user_id) AND f.status = 'pending'
ORDER BY f.created_at DESC, f.id
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: CountOutgoingFriendRequests :one
SELECT count(*)
FROM friendships
WHERE requester_id = $1 AND status = 'pending';

-- name: ListOutgoingFriendRequests :many
SELECT f.id,
       u.id AS user_id, u.nickname, u.display_name, u.avatar_url, u.avatar_revision,
       f.created_at
FROM friendships f
JOIN users u ON u.id = f.addressee_id
WHERE f.requester_id = sqlc.arg(user_id) AND f.status = 'pending'
ORDER BY f.created_at DESC, f.id
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);
