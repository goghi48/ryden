package friendship

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ryden-app/ryden/internal/database"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Search(ctx context.Context, userID uuid.UUID, prefix string, limit int) ([]Person, error) {
	rows, err := database.New(r.pool).SearchUsersByNickname(ctx, database.SearchUsersByNicknameParams{
		UserID: userID, NicknamePrefix: prefix, ResultLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	items := make([]Person, 0, len(rows))
	for _, row := range rows {
		items = append(items, Person{
			ID: row.ID, Nickname: row.Nickname, DisplayName: row.DisplayName,
			AvatarURL: textPointer(row.AvatarUrl), Relationship: Relationship(row.Relationship),
			RequestID: row.FriendshipID, AvatarRevision: int8Pointer(row.AvatarRevision),
		})
	}
	return items, nil
}

func (r *PostgresRepository) Overview(ctx context.Context, userID uuid.UUID, limit, offset int) (Overview, error) {
	q := database.New(r.pool)
	friendsTotal, err := q.CountFriends(ctx, userID)
	if err != nil {
		return Overview{}, fmt.Errorf("count friends: %w", err)
	}
	friendsRows, err := q.ListFriends(ctx, database.ListFriendsParams{UserID: userID, ResultLimit: int32(limit), ResultOffset: int32(offset)})
	if err != nil {
		return Overview{}, fmt.Errorf("list friends: %w", err)
	}
	incomingTotal, err := q.CountIncomingFriendRequests(ctx, userID)
	if err != nil {
		return Overview{}, fmt.Errorf("count incoming friend requests: %w", err)
	}
	incomingRows, err := q.ListIncomingFriendRequests(ctx, database.ListIncomingFriendRequestsParams{UserID: userID, ResultLimit: int32(limit), ResultOffset: int32(offset)})
	if err != nil {
		return Overview{}, fmt.Errorf("list incoming friend requests: %w", err)
	}
	outgoingTotal, err := q.CountOutgoingFriendRequests(ctx, userID)
	if err != nil {
		return Overview{}, fmt.Errorf("count outgoing friend requests: %w", err)
	}
	outgoingRows, err := q.ListOutgoingFriendRequests(ctx, database.ListOutgoingFriendRequestsParams{UserID: userID, ResultLimit: int32(limit), ResultOffset: int32(offset)})
	if err != nil {
		return Overview{}, fmt.Errorf("list outgoing friend requests: %w", err)
	}

	friends := make([]Item, 0, len(friendsRows))
	for _, row := range friendsRows {
		friends = append(friends, Item{RequestID: row.ID, UserID: row.UserID, Nickname: row.Nickname, DisplayName: row.DisplayName, AvatarURL: textPointer(row.AvatarUrl), AvatarRevision: int8Pointer(row.AvatarRevision), ChangedAt: row.UpdatedAt})
	}
	incoming := make([]Item, 0, len(incomingRows))
	for _, row := range incomingRows {
		incoming = append(incoming, Item{RequestID: row.ID, UserID: row.UserID, Nickname: row.Nickname, DisplayName: row.DisplayName, AvatarURL: textPointer(row.AvatarUrl), AvatarRevision: int8Pointer(row.AvatarRevision), ChangedAt: row.CreatedAt})
	}
	outgoing := make([]Item, 0, len(outgoingRows))
	for _, row := range outgoingRows {
		outgoing = append(outgoing, Item{RequestID: row.ID, UserID: row.UserID, Nickname: row.Nickname, DisplayName: row.DisplayName, AvatarURL: textPointer(row.AvatarUrl), AvatarRevision: int8Pointer(row.AvatarRevision), ChangedAt: row.CreatedAt})
	}
	return Overview{
		Friends:  Page{Items: friends, Total: friendsTotal, Limit: limit, Offset: offset},
		Incoming: Page{Items: incoming, Total: incomingTotal, Limit: limit, Offset: offset},
		Outgoing: Page{Items: outgoing, Total: outgoingTotal, Limit: limit, Offset: offset},
	}, nil
}

func (r *PostgresRepository) Send(ctx context.Context, userID, targetUserID uuid.UUID) (bool, error) {
	q := database.New(r.pool)
	_, err := q.InsertFriendRequest(ctx, database.InsertFriendRequestParams{ID: uuid.New(), RequesterID: userID, AddresseeID: targetUserID})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if _, lookupErr := q.GetFriendshipByPair(ctx, database.GetFriendshipByPairParams{RequesterID: userID, AddresseeID: targetUserID}); lookupErr != nil {
			return false, fmt.Errorf("get existing friendship: %w", lookupErr)
		}
		return false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return false, ErrNotFound
	}
	return false, fmt.Errorf("send friend request: %w", err)
}

func (r *PostgresRepository) Accept(ctx context.Context, userID, requestID uuid.UUID) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin accepting friend request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	request, err := q.GetFriendshipByIDForUpdate(ctx, requestID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && request.AddresseeID != userID {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("get friend request: %w", err)
	}
	if request.Status == "accepted" {
		return false, ErrNotFound
	}
	if _, err := q.AcceptFriendRequest(ctx, database.AcceptFriendRequestParams{ID: requestID, AddresseeID: userID}); err != nil {
		return false, fmt.Errorf("accept friend request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit accepting friend request: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) DeleteRequest(ctx context.Context, userID, requestID uuid.UUID) (bool, error) {
	affected, err := database.New(r.pool).DeletePendingFriendRequest(ctx, database.DeletePendingFriendRequestParams{ID: requestID, RequesterID: userID})
	if err != nil {
		return false, fmt.Errorf("delete friend request: %w", err)
	}
	if affected == 0 {
		return false, ErrNotFound
	}
	return true, nil
}

func (r *PostgresRepository) RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) (bool, error) {
	affected, err := database.New(r.pool).DeleteAcceptedFriendship(ctx, database.DeleteAcceptedFriendshipParams{RequesterID: userID, AddresseeID: friendID})
	if err != nil {
		return false, fmt.Errorf("remove friend: %w", err)
	}
	if affected == 0 {
		return false, ErrNotFound
	}
	return true, nil
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int8Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
