package meeting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/goghi48/ryden/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const createOperation = "meeting.create"

type PostgresRepository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, now: time.Now}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	ownerID uuid.UUID,
	key string,
	requestHash []byte,
	input CreateInput,
) (Meeting, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Meeting{}, false, fmt.Errorf("begin meeting creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	reserved, err := q.ReserveIdempotencyKey(ctx, database.ReserveIdempotencyKeyParams{
		UserID:      ownerID,
		Operation:   createOperation,
		Key:         key,
		RequestHash: requestHash,
		ExpiresAt:   r.now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		return Meeting{}, false, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if reserved == 0 {
		prior, err := q.GetIdempotencyKey(ctx, database.GetIdempotencyKeyParams{
			UserID: ownerID, Operation: createOperation, Key: key,
		})
		if err != nil {
			return Meeting{}, false, fmt.Errorf("read idempotent response: %w", err)
		}
		if !bytes.Equal(prior.RequestHash, requestHash) {
			return Meeting{}, false, ErrIdempotencyConflict
		}
		var result Meeting
		if !prior.StatusCode.Valid || len(prior.ResponseBody) == 0 {
			return Meeting{}, false, errors.New("idempotent request has no completed response")
		}
		if err := json.Unmarshal(prior.ResponseBody, &result); err != nil {
			return Meeting{}, false, fmt.Errorf("decode idempotent response: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Meeting{}, false, fmt.Errorf("commit idempotent replay: %w", err)
		}
		return result, true, nil
	}

	row, err := q.CreateMeeting(ctx, database.CreateMeetingParams{
		ID:               uuid.New(),
		OwnerID:          ownerID,
		Title:            input.Title,
		Description:      input.Description,
		EventType:        input.EventType,
		CoordinationMode: input.CoordinationMode,
		CoverUrl:         optionalText(input.CoverURL),
		LocationName:     optionalText(input.LocationName),
		LocationUrl:      optionalText(input.LocationURL),
		Timezone:         input.Timezone,
	})
	if err != nil {
		return Meeting{}, false, fmt.Errorf("create meeting: %w", err)
	}
	if err := q.AddMeetingParticipant(ctx, database.AddMeetingParticipantParams{
		MeetingID: row.ID,
		UserID:    ownerID,
		Role:      "owner",
	}); err != nil {
		return Meeting{}, false, fmt.Errorf("add meeting owner: %w", err)
	}
	if input.CoordinationMode == "fixed" {
		if _, err := q.CreatePlanOption(ctx, database.CreatePlanOptionParams{
			ID:             uuid.New(),
			MeetingID:      row.ID,
			Title:          input.Title,
			Description:    input.Description,
			Position:       0,
			IdempotencyKey: "fixed:" + row.ID.String() + ":plan",
		}); err != nil {
			return Meeting{}, false, fmt.Errorf("create fixed meeting plan: %w", err)
		}
		if _, err := q.CreateTimeOption(ctx, database.CreateTimeOptionParams{
			ID:             uuid.New(),
			MeetingID:      row.ID,
			PlanOptionID:   nil,
			StartsAt:       *input.StartsAt,
			EndsAt:         input.EndsAt,
			Position:       0,
			IdempotencyKey: "fixed:" + row.ID.String() + ":time",
		}); err != nil {
			return Meeting{}, false, fmt.Errorf("create fixed meeting time: %w", err)
		}
	}
	result := mapMeeting(database.Meeting{
		ID: row.ID, OwnerID: row.OwnerID, Title: row.Title, Description: row.Description,
		CoverUrl: row.CoverUrl, LocationName: row.LocationName, LocationUrl: row.LocationUrl,
		EventType: row.EventType, CoordinationMode: row.CoordinationMode, Timezone: row.Timezone,
		State: row.State, SelectedPlanOptionID: row.SelectedPlanOptionID,
		SelectedTimeOptionID: row.SelectedTimeOptionID, Version: row.Version,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, "owner")
	responseBody, err := json.Marshal(result)
	if err != nil {
		return Meeting{}, false, fmt.Errorf("encode idempotent response: %w", err)
	}
	if err := q.CompleteIdempotencyKey(ctx, database.CompleteIdempotencyKeyParams{
		UserID:       ownerID,
		Operation:    createOperation,
		Key:          key,
		StatusCode:   pgtype.Int4{Int32: 201, Valid: true},
		ResponseBody: responseBody,
	}); err != nil {
		return Meeting{}, false, fmt.Errorf("complete idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Meeting{}, false, fmt.Errorf("commit meeting creation: %w", err)
	}
	return result, false, nil
}

func (r *PostgresRepository) Update(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	input UpdateInput,
) (Meeting, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Meeting{}, fmt.Errorf("begin meeting update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	locked, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Meeting{}, ErrNotFound
	}
	if err != nil {
		return Meeting{}, fmt.Errorf("lock meeting for update: %w", err)
	}
	editableFixed := locked.CoordinationMode == "fixed" && locked.State == "scheduled" && !locked.Archived
	if locked.State != "draft" && !editableFixed {
		return Meeting{}, ErrNotEditable
	}
	if locked.Version != input.ExpectedVersion {
		return Meeting{}, ErrVersionConflict
	}

	row, err := q.UpdateDraftMeeting(ctx, database.UpdateDraftMeetingParams{
		ID:           meetingID,
		Title:        input.Title,
		Description:  input.Description,
		EventType:    input.EventType,
		CoverUrl:     optionalText(input.CoverURL),
		LocationName: optionalText(input.LocationName),
		LocationUrl:  optionalText(input.LocationURL),
	})
	if err != nil {
		return Meeting{}, fmt.Errorf("update meeting: %w", err)
	}
	if locked.CoordinationMode == "fixed" {
		plans, err := q.ListPlanOptions(ctx, meetingID)
		if err != nil {
			return Meeting{}, fmt.Errorf("list fixed meeting plan for metadata update: %w", err)
		}
		if len(plans) == 1 {
			if _, err := q.UpdatePlanOption(ctx, database.UpdatePlanOptionParams{
				ID:          plans[0].ID,
				MeetingID:   meetingID,
				Title:       input.Title,
				Description: input.Description,
			}); err != nil {
				return Meeting{}, fmt.Errorf("update fixed meeting plan metadata: %w", err)
			}
		}
		if input.StartsAt != nil {
			times, err := q.ListTimeOptions(ctx, meetingID)
			if err != nil {
				return Meeting{}, fmt.Errorf("list fixed meeting time for update: %w", err)
			}
			if len(times) != 1 {
				return Meeting{}, ErrFixedSetupInvalid
			}
			if _, err := q.UpdateTimeOption(ctx, database.UpdateTimeOptionParams{
				ID: times[0].ID, MeetingID: meetingID, PlanOptionID: nil,
				StartsAt: *input.StartsAt, EndsAt: input.EndsAt,
			}); err != nil {
				return Meeting{}, fmt.Errorf("update fixed meeting time: %w", err)
			}
		}
	} else if input.StartsAt != nil {
		return Meeting{}, fmt.Errorf("%w: meeting time can only be changed here for a fixed meeting", ErrInvalidInput)
	}
	if err := tx.Commit(ctx); err != nil {
		return Meeting{}, fmt.Errorf("commit meeting update: %w", err)
	}
	return mapMeeting(database.Meeting{
		ID: row.ID, OwnerID: row.OwnerID, Title: row.Title, Description: row.Description,
		CoverUrl: row.CoverUrl, LocationName: row.LocationName, LocationUrl: row.LocationUrl,
		EventType: row.EventType, CoordinationMode: row.CoordinationMode, Timezone: row.Timezone,
		State: row.State, SelectedPlanOptionID: row.SelectedPlanOptionID,
		SelectedTimeOptionID: row.SelectedTimeOptionID, Version: row.Version,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, "owner"), nil
}

func (r *PostgresRepository) List(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
) ([]Meeting, error) {
	rows, err := database.New(r.pool).ListMeetingsForUser(ctx, database.ListMeetingsForUserParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}
	result := make([]Meeting, 0, len(rows))
	meetingIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		meetingIDs = append(meetingIDs, row.ID)
	}
	photoIDs, err := r.meetingPhotoIDs(ctx, meetingIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		role := "participant"
		if row.OwnerID == userID {
			role = "owner"
		}
		item := Meeting{
			ID: row.ID, OwnerID: row.OwnerID, Title: row.Title, Description: row.Description,
			EventType: row.EventType, CoordinationMode: row.CoordinationMode,
			CoverURL: optionalString(row.CoverUrl), LocationName: optionalString(row.LocationName),
			LocationURL: optionalString(row.LocationUrl), Timezone: row.Timezone, State: row.State,
			SelectedPlanOptionID: row.SelectedPlanOptionID, SelectedTimeOptionID: row.SelectedTimeOptionID,
			Version: row.Version, ParticipantRole: role, ParticipantJoinedAt: &row.ParticipantJoinedAt,
			SelectedStartsAt: row.SelectedStartsAt, SelectedEndsAt: row.SelectedEndsAt,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		if row.MyAttendanceStatus != "" {
			status := row.MyAttendanceStatus
			item.MyAttendanceStatus = &status
		}
		item.HasPhoto = photoIDs[row.ID]
		result = append(result, item)
	}
	return result, nil
}

func (r *PostgresRepository) Get(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (Detail, error) {
	q := database.New(r.pool)
	row, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("get meeting: %w", err)
	}
	plans, err := q.ListPlanOptions(ctx, meetingID)
	if err != nil {
		return Detail{}, fmt.Errorf("list plan options: %w", err)
	}
	times, err := q.ListTimeOptions(ctx, meetingID)
	if err != nil {
		return Detail{}, fmt.Errorf("list time options: %w", err)
	}
	participants, err := q.ListMeetingParticipants(ctx, meetingID)
	if err != nil {
		return Detail{}, fmt.Errorf("list participants: %w", err)
	}
	var hasPhoto bool
	if err := r.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM meeting_photos WHERE meeting_id = $1)",
		meetingID,
	).Scan(&hasPhoto); err != nil {
		return Detail{}, fmt.Errorf("check meeting photo: %w", err)
	}
	planPhotoIDs, err := r.planPhotoIDs(ctx, meetingID)
	if err != nil {
		return Detail{}, err
	}
	result := Detail{
		Meeting: Meeting{
			ID:                   row.ID,
			OwnerID:              row.OwnerID,
			Title:                row.Title,
			Description:          row.Description,
			EventType:            row.EventType,
			CoordinationMode:     row.CoordinationMode,
			CoverURL:             optionalString(row.CoverUrl),
			HasPhoto:             hasPhoto,
			LocationName:         optionalString(row.LocationName),
			LocationURL:          optionalString(row.LocationUrl),
			Timezone:             row.Timezone,
			State:                row.State,
			SelectedPlanOptionID: row.SelectedPlanOptionID,
			SelectedTimeOptionID: row.SelectedTimeOptionID,
			Version:              row.Version,
			ParticipantRole:      row.ParticipantRole,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
		},
		PlanOptions:  make([]PlanOption, 0, len(plans)),
		TimeOptions:  make([]TimeOption, 0, len(times)),
		Participants: make([]Participant, 0, len(participants)),
	}
	for _, item := range plans {
		option := mapPlanOption(item)
		option.HasPhoto = planPhotoIDs[option.ID]
		result.PlanOptions = append(result.PlanOptions, option)
	}
	for _, item := range times {
		result.TimeOptions = append(result.TimeOptions, mapTimeOption(item))
	}
	for _, item := range participants {
		result.Participants = append(result.Participants, Participant{
			UserID: item.UserID, DisplayName: item.DisplayName, Role: item.Role, JoinedAt: item.JoinedAt,
		})
	}
	if row.ParticipantRole == "owner" {
		expiresAt, err := q.GetActiveInvitationExpiry(ctx, meetingID)
		if err == nil {
			result.ActiveInvitationExpiresAt = &expiresAt
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, fmt.Errorf("get active invitation: %w", err)
		}
	}
	return result, nil
}

func (r *PostgresRepository) meetingPhotoIDs(
	ctx context.Context,
	meetingIDs []uuid.UUID,
) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return result, nil
	}
	rows, err := r.pool.Query(ctx,
		"SELECT meeting_id FROM meeting_photos WHERE meeting_id = ANY($1)",
		meetingIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("list meeting photo identifiers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var meetingID uuid.UUID
		if err := rows.Scan(&meetingID); err != nil {
			return nil, fmt.Errorf("scan meeting photo identifier: %w", err)
		}
		result[meetingID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meeting photo identifiers: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) planPhotoIDs(
	ctx context.Context,
	meetingID uuid.UUID,
) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool)
	rows, err := r.pool.Query(ctx, `
		SELECT photo.plan_option_id
		FROM plan_option_photos photo
		JOIN plan_options option ON option.id = photo.plan_option_id
		WHERE option.meeting_id = $1
	`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("list plan photo identifiers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var optionID uuid.UUID
		if err := rows.Scan(&optionID); err != nil {
			return nil, fmt.Errorf("scan plan photo identifier: %w", err)
		}
		result[optionID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plan photo identifiers: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) AddPlanOption(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	key string,
	input AddPlanOptionInput,
) (PlanOption, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PlanOption{}, false, fmt.Errorf("begin plan option creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwner(ctx, q, ownerID, meetingID); err != nil {
		return PlanOption{}, false, err
	}
	prior, err := q.GetPlanOptionByIdempotencyKey(ctx, database.GetPlanOptionByIdempotencyKeyParams{
		MeetingID: meetingID, IdempotencyKey: key,
	})
	if err == nil {
		if prior.Title != input.Title || prior.Description != input.Description {
			return PlanOption{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return PlanOption{}, false, fmt.Errorf("commit plan option replay: %w", err)
		}
		return mapPlanOption(prior), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PlanOption{}, false, fmt.Errorf("read prior plan option: %w", err)
	}
	position, err := q.NextPlanOptionPosition(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanOption{}, false, ErrOptionLimit
	}
	if err != nil {
		return PlanOption{}, false, fmt.Errorf("find plan option position: %w", err)
	}
	row, err := q.CreatePlanOption(ctx, database.CreatePlanOptionParams{
		ID: uuid.New(), MeetingID: meetingID, Title: input.Title,
		Description: input.Description, Position: position, IdempotencyKey: key,
	})
	if err != nil {
		return PlanOption{}, false, fmt.Errorf("create plan option: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return PlanOption{}, false, fmt.Errorf("touch meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PlanOption{}, false, fmt.Errorf("commit plan option: %w", err)
	}
	return mapPlanOption(row), false, nil
}

func (r *PostgresRepository) UpdatePlanOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	input UpdatePlanOptionInput,
) (PlanOption, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PlanOption{}, fmt.Errorf("begin plan option update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwnerAtVersion(
		ctx, q, ownerID, meetingID, input.ExpectedVersion,
	); err != nil {
		return PlanOption{}, err
	}
	row, err := q.UpdatePlanOption(ctx, database.UpdatePlanOptionParams{
		ID: optionID, MeetingID: meetingID,
		Title: input.Title, Description: input.Description,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanOption{}, ErrOptionNotFound
	}
	if err != nil {
		return PlanOption{}, fmt.Errorf("update plan option: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return PlanOption{}, fmt.Errorf("touch meeting after plan option update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PlanOption{}, fmt.Errorf("commit plan option update: %w", err)
	}
	return mapPlanOption(row), nil
}

func (r *PostgresRepository) DeletePlanOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin plan option deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwner(ctx, q, ownerID, meetingID); err != nil {
		return err
	}
	affected, err := q.DeletePlanOption(ctx, database.DeletePlanOptionParams{
		ID: optionID, MeetingID: meetingID,
	})
	if err != nil {
		return fmt.Errorf("delete plan option: %w", err)
	}
	if affected > 0 {
		if err := q.TouchMeeting(ctx, meetingID); err != nil {
			return fmt.Errorf("touch meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit plan option deletion: %w", err)
	}
	return nil
}

func (r *PostgresRepository) AddTimeOption(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	key string,
	input AddTimeOptionInput,
) (TimeOption, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return TimeOption{}, false, fmt.Errorf("begin time option creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwner(ctx, q, ownerID, meetingID); err != nil {
		return TimeOption{}, false, err
	}
	prior, err := q.GetTimeOptionByIdempotencyKey(ctx, database.GetTimeOptionByIdempotencyKeyParams{
		MeetingID: meetingID, IdempotencyKey: key,
	})
	if err == nil {
		if !prior.StartsAt.Equal(input.StartsAt) || !sameOptionalTime(prior.EndsAt, input.EndsAt) ||
			!sameOptionalUUID(prior.PlanOptionID, input.PlanOptionID) {
			return TimeOption{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return TimeOption{}, false, fmt.Errorf("commit time option replay: %w", err)
		}
		return mapTimeOption(prior), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TimeOption{}, false, fmt.Errorf("read prior time option: %w", err)
	}
	position, err := q.NextTimeOptionPosition(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimeOption{}, false, ErrOptionLimit
	}
	if err != nil {
		return TimeOption{}, false, fmt.Errorf("find time option position: %w", err)
	}
	if input.PlanOptionID != nil {
		belongs, err := q.PlanOptionBelongsToMeeting(ctx, database.PlanOptionBelongsToMeetingParams{
			ID: *input.PlanOptionID, MeetingID: meetingID,
		})
		if err != nil {
			return TimeOption{}, false, fmt.Errorf("validate time option plan: %w", err)
		}
		if !belongs {
			return TimeOption{}, false, fmt.Errorf("%w: plan option does not belong to meeting", ErrInvalidInput)
		}
	}
	row, err := q.CreateTimeOption(ctx, database.CreateTimeOptionParams{
		ID: uuid.New(), MeetingID: meetingID, PlanOptionID: input.PlanOptionID,
		StartsAt: input.StartsAt, EndsAt: input.EndsAt,
		Position: position, IdempotencyKey: key,
	})
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) &&
		databaseError.ConstraintName == "time_options_meeting_scope_starts_ends_key" {
		return TimeOption{}, false, ErrDuplicateOption
	}
	if err != nil {
		return TimeOption{}, false, fmt.Errorf("create time option: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return TimeOption{}, false, fmt.Errorf("touch meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TimeOption{}, false, fmt.Errorf("commit time option: %w", err)
	}
	return mapTimeOption(row), false, nil
}

func (r *PostgresRepository) UpdateTimeOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	input UpdateTimeOptionInput,
) (TimeOption, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return TimeOption{}, fmt.Errorf("begin time option update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwnerAtVersion(
		ctx, q, ownerID, meetingID, input.ExpectedVersion,
	); err != nil {
		return TimeOption{}, err
	}
	if input.PlanOptionID != nil {
		belongs, err := q.PlanOptionBelongsToMeeting(ctx, database.PlanOptionBelongsToMeetingParams{
			ID: *input.PlanOptionID, MeetingID: meetingID,
		})
		if err != nil {
			return TimeOption{}, fmt.Errorf("validate updated time option plan: %w", err)
		}
		if !belongs {
			return TimeOption{}, fmt.Errorf("%w: plan option does not belong to meeting", ErrInvalidInput)
		}
	}
	row, err := q.UpdateTimeOption(ctx, database.UpdateTimeOptionParams{
		ID: optionID, MeetingID: meetingID, PlanOptionID: input.PlanOptionID,
		StartsAt: input.StartsAt, EndsAt: input.EndsAt,
	})
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) &&
		databaseError.ConstraintName == "time_options_meeting_scope_starts_ends_key" {
		return TimeOption{}, ErrDuplicateOption
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return TimeOption{}, ErrOptionNotFound
	}
	if err != nil {
		return TimeOption{}, fmt.Errorf("update time option: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return TimeOption{}, fmt.Errorf("touch meeting after time option update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TimeOption{}, fmt.Errorf("commit time option update: %w", err)
	}
	return mapTimeOption(row), nil
}

func (r *PostgresRepository) DeleteTimeOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin time option deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := lockDraftOwner(ctx, q, ownerID, meetingID); err != nil {
		return err
	}
	affected, err := q.DeleteTimeOption(ctx, database.DeleteTimeOptionParams{
		ID: optionID, MeetingID: meetingID,
	})
	if err != nil {
		return fmt.Errorf("delete time option: %w", err)
	}
	if affected > 0 {
		if err := q.TouchMeeting(ctx, meetingID); err != nil {
			return fmt.Errorf("touch meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit time option deletion: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateInvitation(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	key string,
	secretHash []byte,
) (Invitation, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Invitation{}, false, fmt.Errorf("begin invitation creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	locked, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, false, ErrNotFound
	}
	if err != nil {
		return Invitation{}, false, fmt.Errorf("lock meeting: %w", err)
	}
	validState := locked.CoordinationMode == "planning" &&
		(locked.State == "draft" || locked.State == "collecting")
	if locked.CoordinationMode == "fixed" {
		validState = locked.State == "draft" || locked.State == "scheduled"
	}
	if !validState {
		return Invitation{}, false, ErrNotEditable
	}
	prior, err := q.GetInvitationByIdempotencyKey(ctx, database.GetInvitationByIdempotencyKeyParams{
		MeetingID: meetingID, IdempotencyKey: key,
	})
	if err == nil {
		if !bytes.Equal(prior.SecretHash, secretHash) {
			return Invitation{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return Invitation{}, false, fmt.Errorf("commit invitation replay: %w", err)
		}
		return Invitation{ExpiresAt: prior.ExpiresAt}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, false, fmt.Errorf("read prior invitation: %w", err)
	}
	var fixedPlanID, fixedTimeID uuid.UUID
	if locked.State == "draft" && locked.CoordinationMode == "planning" {
		planCount, err := q.CountPlanOptions(ctx, meetingID)
		if err != nil {
			return Invitation{}, false, fmt.Errorf("count plan options: %w", err)
		}
		timeCount, err := q.CountTimeOptions(ctx, meetingID)
		if err != nil {
			return Invitation{}, false, fmt.Errorf("count time options: %w", err)
		}
		if planCount < 2 || timeCount < 2 {
			return Invitation{}, false, ErrSetupIncomplete
		}
	} else if locked.State == "draft" {
		plans, err := q.ListPlanOptions(ctx, meetingID)
		if err != nil {
			return Invitation{}, false, fmt.Errorf("list fixed meeting plans: %w", err)
		}
		times, err := q.ListTimeOptions(ctx, meetingID)
		if err != nil {
			return Invitation{}, false, fmt.Errorf("list fixed meeting times: %w", err)
		}
		if len(plans) != 1 || len(times) != 1 {
			return Invitation{}, false, ErrFixedSetupInvalid
		}
		if times[0].PlanOptionID != nil && *times[0].PlanOptionID != plans[0].ID {
			return Invitation{}, false, ErrFixedSetupInvalid
		}
		fixedPlanID, fixedTimeID = plans[0].ID, times[0].ID
	}
	if _, err := q.RevokeActiveInvitations(ctx, meetingID); err != nil {
		return Invitation{}, false, fmt.Errorf("revoke prior invitation: %w", err)
	}
	expiresAt := r.now().UTC().Add(7 * 24 * time.Hour)
	row, err := q.CreateInvitation(ctx, database.CreateInvitationParams{
		ID: uuid.New(), MeetingID: meetingID, CreatedBy: ownerID,
		SecretHash: secretHash, IdempotencyKey: key, ExpiresAt: expiresAt,
	})
	if err != nil {
		return Invitation{}, false, fmt.Errorf("create invitation: %w", err)
	}
	if locked.State == "draft" {
		if locked.CoordinationMode == "planning" {
			if err := q.OpenMeetingCollection(ctx, meetingID); err != nil {
				return Invitation{}, false, fmt.Errorf("open meeting collection: %w", err)
			}
		} else if err := q.ScheduleMeeting(ctx, database.ScheduleMeetingParams{
			ID: meetingID, SelectedPlanOptionID: &fixedPlanID, SelectedTimeOptionID: &fixedTimeID,
		}); err != nil {
			return Invitation{}, false, fmt.Errorf("schedule fixed meeting: %w", err)
		}
	} else if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return Invitation{}, false, fmt.Errorf("touch meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, false, fmt.Errorf("commit invitation: %w", err)
	}
	return Invitation{ExpiresAt: row.ExpiresAt}, false, nil
}

func (r *PostgresRepository) RevokeInvitation(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin invitation revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	locked, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock meeting: %w", err)
	}
	if locked.State != "collecting" &&
		!(locked.CoordinationMode == "fixed" && locked.State == "scheduled") {
		return false, ErrNotEditable
	}
	affected, err := q.RevokeActiveInvitations(ctx, meetingID)
	if err != nil {
		return false, fmt.Errorf("revoke invitation: %w", err)
	}
	if affected > 0 {
		if err := q.TouchMeeting(ctx, meetingID); err != nil {
			return false, fmt.Errorf("touch meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit invitation revocation: %w", err)
	}
	return affected > 0, nil
}

func (r *PostgresRepository) JoinInvitation(
	ctx context.Context,
	userID uuid.UUID,
	secretHash []byte,
) (Detail, bool, error) {
	q := database.New(r.pool)
	meetingID, err := q.GetInvitationMeetingByHash(ctx, secretHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, false, ErrInvitationInvalid
	}
	if err != nil {
		return Detail{}, false, fmt.Errorf("find invitation: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Detail{}, false, fmt.Errorf("begin invitation join: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := database.New(tx)
	locked, err := tq.LockMeetingByID(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, false, ErrInvitationInvalid
	}
	if err != nil {
		return Detail{}, false, fmt.Errorf("lock invited meeting: %w", err)
	}
	if locked.State != "collecting" &&
		!(locked.CoordinationMode == "fixed" && locked.State == "scheduled") {
		return Detail{}, false, ErrInvitationInvalid
	}
	if _, err := tq.GetValidInvitationForUpdate(ctx, database.GetValidInvitationForUpdateParams{
		MeetingID: meetingID, SecretHash: secretHash,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, false, ErrInvitationInvalid
	} else if err != nil {
		return Detail{}, false, fmt.Errorf("validate invitation: %w", err)
	}
	affected, err := tq.JoinMeeting(ctx, database.JoinMeetingParams{
		MeetingID: meetingID, UserID: userID,
	})
	if err != nil {
		return Detail{}, false, fmt.Errorf("join meeting: %w", err)
	}
	joined := affected > 0
	if joined {
		if err := tq.TouchMeeting(ctx, meetingID); err != nil {
			return Detail{}, false, fmt.Errorf("touch meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, false, fmt.Errorf("commit invitation join: %w", err)
	}
	detail, err := r.Get(ctx, userID, meetingID)
	if err != nil {
		return Detail{}, false, err
	}
	return detail, joined, nil
}

func (r *PostgresRepository) Complete(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (Completion, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Completion{}, false, fmt.Errorf("begin meeting completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	locked, err := q.GetMeetingCompletionForOwnerForUpdate(
		ctx,
		database.GetMeetingCompletionForOwnerForUpdateParams{
			ID: meetingID, OwnerID: ownerID,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Completion{}, false, ErrNotFound
	}
	if err != nil {
		return Completion{}, false, fmt.Errorf("lock meeting for completion: %w", err)
	}
	if locked.State == "completed" {
		if err := tx.Commit(ctx); err != nil {
			return Completion{}, false, fmt.Errorf("commit meeting completion replay: %w", err)
		}
		return Completion{
			MeetingID: meetingID,
			State:     locked.State,
			Version:   locked.Version,
			UpdatedAt: locked.UpdatedAt,
		}, true, nil
	}
	if locked.State != "scheduled" {
		return Completion{}, false, ErrNotCompletable
	}

	openRequirements, err := q.CountOpenRequirements(ctx, meetingID)
	if err != nil {
		return Completion{}, false, fmt.Errorf("count open requirements: %w", err)
	}
	if openRequirements > 0 {
		return Completion{}, false, ErrPreparationIncomplete
	}

	if _, err := q.RevokeActiveInvitations(ctx, meetingID); err != nil {
		return Completion{}, false, fmt.Errorf("revoke invitations for completion: %w", err)
	}
	row, err := q.CompleteMeeting(ctx, meetingID)
	if err != nil {
		return Completion{}, false, fmt.Errorf("complete meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Completion{}, false, fmt.Errorf("commit meeting completion: %w", err)
	}
	return Completion{
		MeetingID: meetingID,
		State:     "completed",
		Version:   row.Version,
		UpdatedAt: row.UpdatedAt,
	}, false, nil
}

func (r *PostgresRepository) Cancel(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (Cancellation, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Cancellation{}, false, fmt.Errorf("begin meeting cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	locked, err := q.GetMeetingCancellationForOwnerForUpdate(
		ctx,
		database.GetMeetingCancellationForOwnerForUpdateParams{
			ID: meetingID, OwnerID: ownerID,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cancellation{}, false, ErrNotFound
	}
	if err != nil {
		return Cancellation{}, false, fmt.Errorf("lock meeting for cancellation: %w", err)
	}
	if locked.State == "cancelled" {
		if err := tx.Commit(ctx); err != nil {
			return Cancellation{}, false, fmt.Errorf("commit meeting cancellation replay: %w", err)
		}
		return Cancellation{
			MeetingID: meetingID,
			State:     locked.State,
			Version:   locked.Version,
			UpdatedAt: locked.UpdatedAt,
		}, true, nil
	}
	if locked.State != "draft" && locked.State != "collecting" && locked.State != "scheduled" {
		return Cancellation{}, false, ErrNotCancellable
	}

	if _, err := q.RevokeActiveInvitations(ctx, meetingID); err != nil {
		return Cancellation{}, false, fmt.Errorf("revoke invitations for cancellation: %w", err)
	}
	row, err := q.CancelMeeting(ctx, meetingID)
	if err != nil {
		return Cancellation{}, false, fmt.Errorf("cancel meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Cancellation{}, false, fmt.Errorf("commit meeting cancellation: %w", err)
	}
	return Cancellation{
		MeetingID: meetingID,
		State:     "cancelled",
		Version:   row.Version,
		UpdatedAt: row.UpdatedAt,
	}, false, nil
}

func lockDraftOwner(
	ctx context.Context,
	q *database.Queries,
	ownerID, meetingID uuid.UUID,
) error {
	row, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock meeting: %w", err)
	}
	if row.State != "draft" {
		return ErrNotEditable
	}
	return nil
}

func lockDraftOwnerAtVersion(
	ctx context.Context,
	q *database.Queries,
	ownerID, meetingID uuid.UUID,
	expectedVersion int64,
) error {
	row, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock meeting: %w", err)
	}
	if row.State != "draft" {
		return ErrNotEditable
	}
	if row.Version != expectedVersion {
		return ErrVersionConflict
	}
	return nil
}

func mapMeeting(row database.Meeting, role string) Meeting {
	return Meeting{
		ID:                   row.ID,
		OwnerID:              row.OwnerID,
		Title:                row.Title,
		Description:          row.Description,
		EventType:            row.EventType,
		CoordinationMode:     row.CoordinationMode,
		CoverURL:             optionalString(row.CoverUrl),
		LocationName:         optionalString(row.LocationName),
		LocationURL:          optionalString(row.LocationUrl),
		Timezone:             row.Timezone,
		State:                row.State,
		SelectedPlanOptionID: row.SelectedPlanOptionID,
		SelectedTimeOptionID: row.SelectedTimeOptionID,
		Version:              row.Version,
		ParticipantRole:      role,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func mapPlanOption(row database.PlanOption) PlanOption {
	return PlanOption{
		ID: row.ID, Title: row.Title, Description: row.Description,
		Position: row.Position, CreatedAt: row.CreatedAt,
	}
}

func mapTimeOption(row database.TimeOption) TimeOption {
	return TimeOption{
		ID: row.ID, PlanOptionID: row.PlanOptionID,
		StartsAt: row.StartsAt, EndsAt: row.EndsAt,
		Position: row.Position, CreatedAt: row.CreatedAt,
	}
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
