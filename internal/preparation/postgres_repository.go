package preparation

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/goghi48/ryden/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	limit, offset int,
) (Page, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Page{}, fmt.Errorf("begin preparation snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	} else if err != nil {
		return Page{}, fmt.Errorf("authorize preparation list: %w", err)
	}
	rows, err := q.ListRequirements(ctx, database.ListRequirementsParams{
		MeetingID: meetingID, UserID: userID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return Page{}, fmt.Errorf("list requirements: %w", err)
	}
	claimRows, err := q.ListRequirementClaims(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("list requirement claims: %w", err)
	}
	counts, err := q.CountRequirementStatuses(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("count requirement statuses: %w", err)
	}
	claimsByRequirement := make(map[uuid.UUID][]Assignee, len(rows))
	for _, claim := range claimRows {
		claimsByRequirement[claim.RequirementID] = append(
			claimsByRequirement[claim.RequirementID],
			Assignee{
				UserID: claim.UserID, DisplayName: claim.DisplayName,
				Quantity: claim.Quantity, UpdatedAt: claim.UpdatedAt,
			},
		)
	}
	result := Page{
		Items:          make([]Requirement, 0, len(rows)),
		Total:          counts.Total,
		OpenCount:      counts.OpenCount,
		CompletedCount: counts.CompletedCount,
		Limit:          limit,
		Offset:         offset,
	}
	for _, row := range rows {
		assignees := claimsByRequirement[row.ID]
		if assignees == nil {
			assignees = []Assignee{}
		}
		result.Items = append(result.Items, Requirement{
			ID: row.ID, Name: row.Name,
			RequiredQuantity: row.RequiredQuantity,
			ClaimedQuantity:  row.ClaimedQuantity,
			RemainingQuantity: row.RequiredQuantity -
				row.ClaimedQuantity,
			Status: row.Status, MyQuantity: row.MyQuantity,
			Assignees: assignees,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit preparation snapshot: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	key string,
	requestHash []byte,
	input CreateInput,
) (Requirement, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Requirement{}, false, fmt.Errorf("begin requirement creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingDecisionForOwnerForUpdate(
		ctx,
		database.GetMeetingDecisionForOwnerForUpdateParams{ID: meetingID, OwnerID: ownerID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Requirement{}, false, ErrNotFound
	}
	if err != nil {
		return Requirement{}, false, fmt.Errorf("lock preparation meeting: %w", err)
	}
	prior, err := q.GetRequirementByIdempotencyKey(
		ctx,
		database.GetRequirementByIdempotencyKeyParams{
			MeetingID: meetingID, IdempotencyKey: key,
		},
	)
	if err == nil {
		if !bytes.Equal(prior.RequestHash, requestHash) {
			return Requirement{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return Requirement{}, false, fmt.Errorf("commit requirement replay: %w", err)
		}
		return mapRequirement(prior), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Requirement{}, false, fmt.Errorf("read prior requirement: %w", err)
	}
	if meetingRow.State != "scheduled" {
		return Requirement{}, false, ErrNotEditable
	}
	count, err := q.CountMeetingRequirements(ctx, meetingID)
	if err != nil {
		return Requirement{}, false, fmt.Errorf("count requirements: %w", err)
	}
	if count >= 50 {
		return Requirement{}, false, ErrLimit
	}
	row, err := q.CreateRequirement(ctx, database.CreateRequirementParams{
		ID: uuid.New(), MeetingID: meetingID, CreatedBy: ownerID,
		Name: input.Name, RequiredQuantity: input.RequiredQuantity,
		IdempotencyKey: key, RequestHash: requestHash,
	})
	if err != nil {
		if isDuplicateRequirement(err) {
			return Requirement{}, false, ErrDuplicate
		}
		return Requirement{}, false, fmt.Errorf("create requirement: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return Requirement{}, false, fmt.Errorf("touch preparation meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Requirement{}, false, fmt.Errorf("commit requirement creation: %w", err)
	}
	return mapRequirement(row), false, nil
}

func (r *PostgresRepository) Update(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
	input UpdateInput,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin requirement update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingDecisionForOwnerForUpdate(
		ctx,
		database.GetMeetingDecisionForOwnerForUpdateParams{ID: meetingID, OwnerID: ownerID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement meeting: %w", err)
	}
	if meetingRow.State != "scheduled" {
		return false, ErrNotEditable
	}
	requirementRow, err := q.GetRequirementForUpdate(
		ctx,
		database.GetRequirementForUpdateParams{MeetingID: meetingID, ID: requirementID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement: %w", err)
	}
	if requirementRow.Status != StatusOpen {
		return false, ErrNotEditable
	}
	claimed, err := q.SumRequirementClaims(ctx, requirementID)
	if err != nil {
		return false, fmt.Errorf("sum claims before requirement update: %w", err)
	}
	if input.RequiredQuantity < claimed {
		return false, ErrQuantityExceeded
	}
	if requirementRow.Name == input.Name &&
		requirementRow.RequiredQuantity == input.RequiredQuantity {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit requirement update replay: %w", err)
		}
		return false, nil
	}
	affected, err := q.UpdateRequirement(ctx, database.UpdateRequirementParams{
		MeetingID: meetingID, ID: requirementID,
		Name: input.Name, RequiredQuantity: input.RequiredQuantity,
	})
	if err != nil {
		if isDuplicateRequirement(err) {
			return false, ErrDuplicate
		}
		return false, fmt.Errorf("update requirement: %w", err)
	}
	if affected == 0 {
		return false, ErrNotFound
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch requirement meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit requirement update: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) Delete(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin requirement deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingDecisionForOwnerForUpdate(
		ctx,
		database.GetMeetingDecisionForOwnerForUpdateParams{ID: meetingID, OwnerID: ownerID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement meeting: %w", err)
	}
	if meetingRow.State != "scheduled" {
		return false, ErrNotEditable
	}
	requirementRow, err := q.GetRequirementForUpdate(
		ctx,
		database.GetRequirementForUpdateParams{MeetingID: meetingID, ID: requirementID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement: %w", err)
	}
	if requirementRow.Status != StatusOpen {
		return false, ErrNotEditable
	}
	claimed, err := q.SumRequirementClaims(ctx, requirementID)
	if err != nil {
		return false, fmt.Errorf("sum claims before requirement deletion: %w", err)
	}
	if claimed > 0 {
		return false, ErrHasClaims
	}
	affected, err := q.DeleteRequirement(ctx, database.DeleteRequirementParams{
		MeetingID: meetingID, ID: requirementID,
	})
	if err != nil {
		return false, fmt.Errorf("delete requirement: %w", err)
	}
	if affected == 0 {
		return false, ErrNotFound
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch requirement meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit requirement deletion: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) SetClaim(
	ctx context.Context,
	userID, meetingID, requirementID uuid.UUID,
	quantity int32,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin requirement claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.LockMeetingByID(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock claim meeting: %w", err)
	}
	if meetingRow.State != "scheduled" {
		return false, ErrNotEditable
	}
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, fmt.Errorf("authorize requirement claim: %w", err)
	}
	requirementRow, err := q.GetRequirementForUpdate(
		ctx,
		database.GetRequirementForUpdateParams{MeetingID: meetingID, ID: requirementID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement: %w", err)
	}
	if requirementRow.Status != StatusOpen {
		return false, ErrNotEditable
	}
	total, err := q.SumRequirementClaims(ctx, requirementID)
	if err != nil {
		return false, fmt.Errorf("sum requirement claims: %w", err)
	}
	current, err := q.GetRequirementClaim(ctx, database.GetRequirementClaimParams{
		RequirementID: requirementID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current = 0
	} else if err != nil {
		return false, fmt.Errorf("read current requirement claim: %w", err)
	}
	if current == quantity {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit requirement claim replay: %w", err)
		}
		return false, nil
	}
	if total-current+quantity > requirementRow.RequiredQuantity {
		return false, ErrQuantityExceeded
	}
	if quantity == 0 {
		if _, err := q.DeleteRequirementClaim(ctx, database.DeleteRequirementClaimParams{
			RequirementID: requirementID, UserID: userID,
		}); err != nil {
			return false, fmt.Errorf("retract requirement claim: %w", err)
		}
	} else if err := q.UpsertRequirementClaim(ctx, database.UpsertRequirementClaimParams{
		MeetingID: meetingID, RequirementID: requirementID,
		UserID: userID, Quantity: quantity,
	}); err != nil {
		return false, fmt.Errorf("store requirement claim: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch requirement claim meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit requirement claim: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) SetStatus(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
	status string,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin requirement status change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingDecisionForOwnerForUpdate(
		ctx,
		database.GetMeetingDecisionForOwnerForUpdateParams{ID: meetingID, OwnerID: ownerID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock status meeting: %w", err)
	}
	if meetingRow.State != "scheduled" {
		return false, ErrNotEditable
	}
	requirementRow, err := q.GetRequirementForUpdate(
		ctx,
		database.GetRequirementForUpdateParams{MeetingID: meetingID, ID: requirementID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock requirement status: %w", err)
	}
	if requirementRow.Status == status {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit requirement status replay: %w", err)
		}
		return false, nil
	}
	if status == StatusCompleted {
		total, err := q.SumRequirementClaims(ctx, requirementID)
		if err != nil {
			return false, fmt.Errorf("sum claims before completion: %w", err)
		}
		if total != requirementRow.RequiredQuantity {
			return false, ErrNotFullyClaimed
		}
	}
	if err := q.SetRequirementStatus(ctx, database.SetRequirementStatusParams{
		MeetingID: meetingID, ID: requirementID, Status: status,
	}); err != nil {
		return false, fmt.Errorf("set requirement status: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch requirement status meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit requirement status: %w", err)
	}
	return true, nil
}

func mapRequirement(row database.Requirement) Requirement {
	return Requirement{
		ID: row.ID, Name: row.Name, RequiredQuantity: row.RequiredQuantity,
		RemainingQuantity: row.RequiredQuantity, Status: row.Status,
		Assignees: []Assignee{}, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func isDuplicateRequirement(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) &&
		postgresError.Code == "23505" &&
		postgresError.ConstraintName == "requirements_meeting_name_idx"
}
