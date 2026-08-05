package poll

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goghi48/ryden/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, now: time.Now}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	key string,
	requestHash []byte,
	input CreateInput,
) (Poll, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Poll{}, false, fmt.Errorf("begin poll creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if _, err := q.LockMeetingByID(ctx, meetingID); errors.Is(err, pgx.ErrNoRows) {
		return Poll{}, false, ErrNotFound
	} else if err != nil {
		return Poll{}, false, fmt.Errorf("lock meeting: %w", err)
	}
	meetingRow, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Poll{}, false, ErrNotFound
	}
	if err != nil {
		return Poll{}, false, fmt.Errorf("lock meeting: %w", err)
	}
	if meetingRow.State != "draft" && meetingRow.State != "collecting" && meetingRow.State != "scheduled" {
		return Poll{}, false, ErrNotEditable
	}
	participantCount, err := q.CountActiveMeetingParticipants(ctx, meetingID)
	if err != nil {
		return Poll{}, false, fmt.Errorf("count active participants: %w", err)
	}
	prior, err := q.GetPollByIdempotencyKey(ctx, database.GetPollByIdempotencyKeyParams{
		MeetingID: meetingID, IdempotencyKey: key,
	})
	if err == nil {
		if !bytes.Equal(prior.RequestHash, requestHash) {
			return Poll{}, false, ErrIdempotencyConflict
		}
		result, err := loadOne(ctx, q, userID, meetingRow.OwnerID, participantCount, meetingRow.State, prior)
		if err != nil {
			return Poll{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Poll{}, false, fmt.Errorf("commit poll replay: %w", err)
		}
		return result, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Poll{}, false, fmt.Errorf("read prior poll: %w", err)
	}
	count, err := q.CountMeetingPolls(ctx, meetingID)
	if err != nil {
		return Poll{}, false, fmt.Errorf("count polls: %w", err)
	}
	if count >= 10 {
		return Poll{}, false, ErrLimit
	}
	row, err := q.CreatePoll(ctx, database.CreatePollParams{
		ID: uuid.New(), MeetingID: meetingID, CreatedByUserID: userID, Question: input.Question,
		ResponseMode: input.ResponseMode, Deadline: input.Deadline,
		IsAnonymous: input.IsAnonymous, AllowRevote: input.AllowRevote,
		IdempotencyKey: key, RequestHash: requestHash,
	})
	if err != nil {
		return Poll{}, false, fmt.Errorf("create poll: %w", err)
	}
	for index, label := range input.Options {
		if err := q.CreatePollOption(ctx, database.CreatePollOptionParams{
			ID: uuid.New(), PollID: row.ID, Label: label, Position: int16(index),
		}); err != nil {
			return Poll{}, false, fmt.Errorf("create poll option: %w", err)
		}
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return Poll{}, false, fmt.Errorf("touch meeting: %w", err)
	}
	result, err := loadOne(ctx, q, userID, meetingRow.OwnerID, participantCount, meetingRow.State, row)
	if err != nil {
		return Poll{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Poll{}, false, fmt.Errorf("commit poll creation: %w", err)
	}
	return result, false, nil
}

func (r *PostgresRepository) List(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) ([]Poll, error) {
	q := database.New(r.pool)
	meetingRow, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("authorize poll list: %w", err)
	}
	rows, err := q.ListPollsForMeeting(ctx, meetingID)
	if err != nil {
		return nil, fmt.Errorf("list polls: %w", err)
	}
	participantCount, err := q.CountActiveMeetingParticipants(ctx, meetingID)
	if err != nil {
		return nil, fmt.Errorf("count active participants: %w", err)
	}
	result := make([]Poll, 0, len(rows))
	for _, row := range rows {
		item, err := loadOne(ctx, q, userID, meetingRow.OwnerID, participantCount, meetingRow.State, row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *PostgresRepository) History(
	ctx context.Context,
	userID, pollID uuid.UUID,
	limit, offset int,
) (HistoryPage, error) {
	q := database.New(r.pool)
	visible, err := q.PollVisibleToUser(ctx, database.PollVisibleToUserParams{
		ID: pollID, UserID: userID,
	})
	if err != nil {
		return HistoryPage{}, fmt.Errorf("authorize poll history: %w", err)
	}
	if !visible {
		return HistoryPage{}, ErrNotFound
	}
	rows, err := q.ListPollVoteHistory(ctx, database.ListPollVoteHistoryParams{
		PollID: pollID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return HistoryPage{}, fmt.Errorf("list poll vote history: %w", err)
	}
	total, err := q.CountPollVoteHistory(ctx, pollID)
	if err != nil {
		return HistoryPage{}, fmt.Errorf("count poll vote history: %w", err)
	}
	result := HistoryPage{
		Items: make([]HistoryEntry, 0, len(rows)), Total: total, Limit: limit, Offset: offset,
	}
	for _, row := range rows {
		result.Items = append(result.Items, HistoryEntry{
			ID: row.ID, UserID: row.UserID, DisplayName: row.DisplayName, Action: row.Action,
			PreviousOptionIDs: row.PreviousOptionIds, PreviousOptionLabels: row.PreviousOptionLabels,
			NewOptionIDs: row.NewOptionIds, NewOptionLabels: row.NewOptionLabels,
			CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func (r *PostgresRepository) Delete(
	ctx context.Context,
	ownerID, meetingID, pollID uuid.UUID,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin poll deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingForOwnerForUpdate(ctx, database.GetMeetingForOwnerForUpdateParams{
		ID: meetingID, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock meeting: %w", err)
	}
	if meetingRow.State != "draft" {
		return ErrNotEditable
	}
	affected, err := q.DeleteDraftPoll(ctx, database.DeleteDraftPollParams{
		ID: pollID, MeetingID: meetingID,
	})
	if err != nil {
		return fmt.Errorf("delete poll: %w", err)
	}
	if affected > 0 {
		if err := q.TouchMeeting(ctx, meetingID); err != nil {
			return fmt.Errorf("touch meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit poll deletion: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Vote(
	ctx context.Context,
	userID, pollID uuid.UUID,
	optionIDs []uuid.UUID,
) (bool, error) {
	q := database.New(r.pool)
	meetingID, err := q.GetPollMeetingID(ctx, pollID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("find poll meeting: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin poll vote: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := database.New(tx)
	if _, err := tq.LockMeetingByID(ctx, meetingID); err != nil {
		return false, fmt.Errorf("lock meeting: %w", err)
	}
	row, err := tq.GetPollForParticipantForUpdate(ctx, database.GetPollForParticipantForUpdateParams{
		ID: pollID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock poll: %w", err)
	}
	if row.MeetingState != "collecting" && row.MeetingState != "scheduled" {
		return false, ErrNotEditable
	}
	if row.State != "open" {
		return false, ErrClosed
	}
	if row.Deadline != nil && !row.Deadline.After(r.now().UTC()) {
		return false, ErrDeadline
	}
	if row.ResponseMode == "single" && len(optionIDs) > 1 {
		return false, fmt.Errorf("%w: single-choice poll accepts at most one option", ErrInvalidInput)
	}
	previousRows, err := tq.ListUserPollVoteOptions(
		ctx,
		database.ListUserPollVoteOptionsParams{PollID: pollID, UserID: userID},
	)
	if err != nil {
		return false, fmt.Errorf("read current poll answer: %w", err)
	}
	previousIDs := make([]uuid.UUID, 0, len(previousRows))
	previousLabels := make([]string, 0, len(previousRows))
	for _, option := range previousRows {
		previousIDs = append(previousIDs, option.ID)
		previousLabels = append(previousLabels, option.Label)
	}
	newIDs := make([]uuid.UUID, 0, len(optionIDs))
	newLabels := make([]string, 0, len(optionIDs))
	if len(optionIDs) > 0 {
		options, err := tq.ListPollOptionsByIDs(ctx, database.ListPollOptionsByIDsParams{
			PollID: pollID, Column2: optionIDs,
		})
		if err != nil {
			return false, fmt.Errorf("validate poll options: %w", err)
		}
		if len(options) != len(optionIDs) {
			return false, fmt.Errorf("%w: option does not belong to poll", ErrInvalidInput)
		}
		for _, option := range options {
			newIDs = append(newIDs, option.ID)
			newLabels = append(newLabels, option.Label)
		}
	}
	if sameUUIDSlice(previousIDs, newIDs) {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit unchanged poll vote: %w", err)
		}
		return false, nil
	}
	if !row.AllowRevote && len(previousIDs) > 0 {
		return false, ErrRevoteDisabled
	}
	if err := tq.DeleteUserPollVotes(ctx, database.DeleteUserPollVotesParams{
		PollID: pollID, UserID: userID,
	}); err != nil {
		return false, fmt.Errorf("replace poll vote: %w", err)
	}
	for _, optionID := range newIDs {
		if err := tq.CreatePollVote(ctx, database.CreatePollVoteParams{
			PollID: pollID, OptionID: optionID, UserID: userID,
		}); err != nil {
			return false, fmt.Errorf("create poll vote: %w", err)
		}
	}
	if err := tq.CreatePollVoteHistory(ctx, database.CreatePollVoteHistoryParams{
		ID: uuid.New(), PollID: pollID, UserID: userID,
		Action:            voteHistoryAction(previousIDs, newIDs),
		PreviousOptionIds: previousIDs, PreviousOptionLabels: previousLabels,
		NewOptionIds: newIDs, NewOptionLabels: newLabels,
	}); err != nil {
		return false, fmt.Errorf("record poll vote history: %w", err)
	}
	if err := tq.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit poll vote: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) Close(
	ctx context.Context,
	userID, meetingID, pollID uuid.UUID,
	selectedOptionID *uuid.UUID,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin poll close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if _, err := q.LockMeetingByID(ctx, meetingID); err != nil {
		return false, fmt.Errorf("lock meeting: %w", err)
	}
	row, err := q.GetPollForManagerForUpdate(ctx, database.GetPollForManagerForUpdateParams{
		ID: pollID, MeetingID: meetingID, OwnerID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock poll: %w", err)
	}
	if row.MeetingState != "collecting" && row.MeetingState != "scheduled" {
		return false, ErrNotEditable
	}
	if row.State == "closed" {
		if sameOptionalUUID(row.SelectedOptionID, selectedOptionID) {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit closed poll replay: %w", err)
			}
			return false, nil
		}
		return false, ErrConflict
	}
	if selectedOptionID != nil {
		belongs, err := q.PollOptionBelongsToPoll(ctx, database.PollOptionBelongsToPollParams{
			ID: *selectedOptionID, PollID: pollID,
		})
		if err != nil {
			return false, fmt.Errorf("validate selected poll option: %w", err)
		}
		if !belongs {
			return false, fmt.Errorf("%w: selected option does not belong to poll", ErrInvalidInput)
		}
	}
	if err := q.ClosePoll(ctx, database.ClosePollParams{
		ID: pollID, SelectedOptionID: selectedOptionID,
	}); err != nil {
		return false, fmt.Errorf("close poll: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit poll close: %w", err)
	}
	return true, nil
}

func loadOne(
	ctx context.Context,
	q *database.Queries,
	userID uuid.UUID,
	meetingOwnerID uuid.UUID,
	participantCount int32,
	meetingState string,
	row database.Poll,
) (Poll, error) {
	options, err := q.ListPollOptionsWithResults(ctx, database.ListPollOptionsWithResultsParams{
		PollID: row.ID, UserID: userID,
	})
	if err != nil {
		return Poll{}, fmt.Errorf("list poll options: %w", err)
	}
	respondentCount, err := q.CountPollRespondents(ctx, row.ID)
	if err != nil {
		return Poll{}, fmt.Errorf("count poll respondents: %w", err)
	}
	votersByOption := make(map[uuid.UUID][]Voter, len(options))
	if !row.IsAnonymous {
		voterRows, err := q.ListPollVoters(ctx, row.ID)
		if err != nil {
			return Poll{}, fmt.Errorf("list poll voters: %w", err)
		}
		for _, voter := range voterRows {
			votersByOption[voter.OptionID] = append(votersByOption[voter.OptionID], Voter{
				UserID: voter.UserID, DisplayName: voter.DisplayName, UpdatedAt: voter.UpdatedAt,
			})
		}
	}
	result := Poll{
		ID: row.ID, CreatedByUserID: row.CreatedByUserID,
		Question: row.Question, ResponseMode: row.ResponseMode,
		IsAnonymous: row.IsAnonymous, AllowRevote: row.AllowRevote,
		CanManage: userID == row.CreatedByUserID || userID == meetingOwnerID,
		State:     row.State, SelectedOptionID: row.SelectedOptionID,
		AcceptingAnswers: (meetingState == "collecting" || meetingState == "scheduled") && row.State == "open" &&
			(row.Deadline == nil || row.Deadline.After(time.Now().UTC())),
		Options:          make([]Option, 0, len(options)),
		ParticipantCount: participantCount,
		RespondentCount:  respondentCount,
		CreatedAt:        row.CreatedAt,
	}
	if row.Deadline != nil {
		deadline := *row.Deadline
		result.Deadline = &deadline
	}
	if row.ClosedAt != nil {
		closedAt := *row.ClosedAt
		result.ClosedAt = &closedAt
	}
	for _, option := range options {
		voters := votersByOption[option.ID]
		if voters == nil {
			voters = []Voter{}
		}
		result.Options = append(result.Options, Option{
			ID: option.ID, Label: option.Label, Position: option.Position,
			VoteCount: option.VoteCount, SelectedByUser: option.SelectedByUser,
			Voters: voters,
		})
		result.TotalSelections += option.VoteCount
	}
	return result, nil
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameUUIDSlice(left, right []uuid.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func voteHistoryAction(previousIDs, newIDs []uuid.UUID) string {
	if len(previousIDs) == 0 {
		return "cast"
	}
	if len(newIDs) == 0 {
		return "retract"
	}
	return "change"
}
