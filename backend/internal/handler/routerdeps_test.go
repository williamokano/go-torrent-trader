package handler

import (
	"context"
	"database/sql"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// The repositories below exist only so that every service in Deps can be
// constructed, and therefore so that every route in router.go actually
// registers. router.go is full of `if deps.X != nil` guards, so a nil service
// silently removes its routes from the table — and a route that is not in the
// table is a route the router-walking tests cannot see. Behaviour for each of
// these lives in the handler's own test file; here they are deliberately inert,
// exactly like nopConnectorRepo in route_authz_test.go.

type nopCommentRepo struct{}

func (nopCommentRepo) Create(context.Context, *model.Comment) error { return nil }
func (nopCommentRepo) GetByID(context.Context, int64) (*model.Comment, error) {
	return nil, sql.ErrNoRows
}
func (nopCommentRepo) ListByTorrent(context.Context, int64, int, int) ([]model.Comment, int64, error) {
	return nil, 0, nil
}
func (nopCommentRepo) Update(context.Context, *model.Comment) error { return nil }
func (nopCommentRepo) Delete(context.Context, int64) error          { return sql.ErrNoRows }

type nopRatingRepo struct{}

func (nopRatingRepo) Upsert(context.Context, *model.Rating) error { return nil }
func (nopRatingRepo) GetByTorrentAndUser(context.Context, int64, int64) (*model.Rating, error) {
	return nil, sql.ErrNoRows
}
func (nopRatingRepo) GetStatsByTorrent(context.Context, int64) (float64, int, error) {
	return 0, 0, nil
}

type nopInviteRepo struct{}

func (nopInviteRepo) Create(context.Context, *model.Invite) error { return nil }
func (nopInviteRepo) GetByID(context.Context, int64) (*model.Invite, error) {
	return nil, sql.ErrNoRows
}
func (nopInviteRepo) GetByToken(context.Context, string) (*model.Invite, error) {
	return nil, sql.ErrNoRows
}
func (nopInviteRepo) ListByInviter(context.Context, int64, int, int) ([]model.Invite, int64, error) {
	return nil, 0, nil
}
func (nopInviteRepo) Redeem(context.Context, string, int64) error               { return nil }
func (nopInviteRepo) CountPendingByInviter(context.Context, int64) (int, error) { return 0, nil }
func (nopInviteRepo) Delete(context.Context, int64) error                       { return sql.ErrNoRows }

type nopBonusRepo struct{}

func (nopBonusRepo) ListEnabledItems(context.Context) ([]model.BonusStoreItem, error) {
	return nil, nil
}
func (nopBonusRepo) GetItem(context.Context, int64) (*model.BonusStoreItem, error) {
	return nil, sql.ErrNoRows
}
func (nopBonusRepo) AwardPoints(context.Context, map[int64]int64, string) error { return nil }
func (nopBonusRepo) SetPoints(context.Context, int64, int64, int64, []model.UserEditHistory) error {
	return nil
}
func (nopBonusRepo) PurchaseItem(context.Context, int64, int64) (*model.BonusStoreItem, int64, error) {
	return nil, 0, sql.ErrNoRows
}
func (nopBonusRepo) SeedingCounts(context.Context) (map[int64]int64, error) { return nil, nil }

type nopPromotionRepo struct{}

func (nopPromotionRepo) ListRules(context.Context) ([]model.PromotionRule, error) { return nil, nil }
func (nopPromotionRepo) UpsertRule(context.Context, *model.PromotionRule) error   { return nil }
func (nopPromotionRepo) DeleteRule(context.Context, int64) error                  { return nil }
func (nopPromotionRepo) LadderMetrics(context.Context, []int64) ([]model.PromotionUserMetrics, error) {
	return nil, nil
}
func (nopPromotionRepo) SeedHoursByUser(context.Context, time.Time, int) (map[int64]int, error) {
	return nil, nil
}
func (nopPromotionRepo) SetUserGroup(context.Context, int64, int64) error { return nil }
func (nopPromotionRepo) LastRunAt(context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (nopPromotionRepo) RecordRun(context.Context, int, int) error { return nil }

type nopInviteDistributionRepo struct{}

func (nopInviteDistributionRepo) ListRules(context.Context) ([]model.InviteDistributionRule, error) {
	return nil, nil
}
func (nopInviteDistributionRepo) UpsertRule(context.Context, *model.InviteDistributionRule) error {
	return nil
}
func (nopInviteDistributionRepo) DeleteRule(context.Context, int64) error { return nil }
func (nopInviteDistributionRepo) GroupMetrics(context.Context, []int64) ([]model.PromotionUserMetrics, error) {
	return nil, nil
}
func (nopInviteDistributionRepo) UserInviteStates(context.Context, []int64) (map[int64]model.UserInviteState, error) {
	return nil, nil
}
func (nopInviteDistributionRepo) LastRunAt(context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (nopInviteDistributionRepo) RecordRun(context.Context, int) error { return nil }

type nopMetadataAuditRepo struct{}

func (nopMetadataAuditRepo) ListMissingRequiredMetadata(context.Context, int64, []string, *int64) ([]model.Torrent, error) {
	return nil, nil
}

// Compile-time proof that the inert repositories still satisfy the interfaces
// they stand in for — otherwise a widened interface would turn into a nil
// service, and a nil service silently drops routes.
var (
	_ repository.CommentRepository            = nopCommentRepo{}
	_ repository.RatingRepository             = nopRatingRepo{}
	_ repository.InviteRepository             = nopInviteRepo{}
	_ repository.BonusRepository              = nopBonusRepo{}
	_ repository.PromotionRepository          = nopPromotionRepo{}
	_ repository.InviteDistributionRepository = nopInviteDistributionRepo{}
)

// nopHnRRepo is the HnR-tracking equivalent of nopPromotionRepo above:
// exists only so HnRService can be constructed for the router-walking tests,
// with every method deliberately inert. Real behavior is exercised in
// internal/service (fakeHnRRepo) and internal/repository/postgres, not here.
type nopHnRRepo struct{}

func (nopHnRRepo) ListRules(context.Context) ([]model.HnRRule, error) { return nil, nil }
func (nopHnRRepo) GetRuleForGroup(context.Context, int64) (*model.HnRRule, error) {
	return nil, sql.ErrNoRows
}
func (nopHnRRepo) UpsertRule(context.Context, *model.HnRRule) error { return nil }
func (nopHnRRepo) DeleteRule(context.Context, int64) error          { return sql.ErrNoRows }
func (nopHnRRepo) CreateIfNotExists(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}
func (nopHnRRepo) Accumulate(context.Context, int64, int64, int64, time.Duration, time.Time) error {
	return nil
}
func (nopHnRRepo) ListOpenForEvaluation(context.Context) ([]repository.HnREvalInput, error) {
	return nil, nil
}
func (nopHnRRepo) MarkBreached(context.Context, []int64, time.Time) (int64, error)  { return 0, nil }
func (nopHnRRepo) MarkSatisfied(context.Context, []int64, time.Time) (int64, error) { return 0, nil }
func (nopHnRRepo) MarkWaived(context.Context, []int64, time.Time) (int64, error)    { return 0, nil }
func (nopHnRRepo) PurgeResolved(context.Context, time.Time) (int64, error)          { return 0, nil }
func (nopHnRRepo) ListStages(context.Context) ([]model.HnRPenaltyStage, error)      { return nil, nil }
func (nopHnRRepo) UpsertStage(context.Context, *model.HnRPenaltyStage) error        { return nil }
func (nopHnRRepo) DeleteStage(context.Context, int) error                           { return sql.ErrNoRows }
func (nopHnRRepo) ActiveHnRCounts(context.Context) (map[int64]int, error)           { return nil, nil }
func (nopHnRRepo) UsersOnLadder(context.Context) ([]model.HnRUserState, error)      { return nil, nil }
func (nopHnRRepo) GetUserState(context.Context, int64) (*model.HnRUserState, error) {
	return nil, sql.ErrNoRows
}
func (nopHnRRepo) EnsureUserState(context.Context, int64, time.Time) error { return nil }
func (nopHnRRepo) CASUserStage(context.Context, int64, int, int, time.Time) (bool, error) {
	return false, nil
}
func (nopHnRRepo) SetLastNotifiedStage(context.Context, int64, int) error  { return nil }
func (nopHnRRepo) StartRun(context.Context, string, *int64) (int64, error) { return 0, nil }
func (nopHnRRepo) FinishRun(context.Context, int64, string, repository.HnRRunCounts, *string) error {
	return nil
}
func (nopHnRRepo) LastRun(context.Context) (*model.HnRRun, bool, error)          { return nil, false, nil }
func (nopHnRRepo) ListRuns(context.Context, int) ([]model.HnRRun, error)         { return nil, nil }
func (nopHnRRepo) ListForUser(context.Context, int64) ([]model.HnRRecord, error) { return nil, nil }
func (nopHnRRepo) GetForUser(context.Context, int64, int64) (*model.HnRRecord, error) {
	return nil, sql.ErrNoRows
}
func (nopHnRRepo) LiveSeedingTorrentIDs(context.Context, int64, []int64) (map[int64]bool, error) {
	return nil, nil
}
func (nopHnRRepo) GetRuleForUser(context.Context, int64) (*model.HnRRule, error) { return nil, nil }
func (nopHnRRepo) ClearRecord(context.Context, int64, int64, int64) (int64, error) {
	return 0, repository.ErrHnRRecordNotClearable
}
func (nopHnRRepo) AdminList(context.Context, repository.HnRAdminListOptions) ([]model.HnRRecord, int64, error) {
	return nil, 0, nil
}
func (nopHnRRepo) AggregateStats(context.Context) (repository.HnRAggregateStats, error) {
	return repository.HnRAggregateStats{}, nil
}
func (nopHnRRepo) TopOffenders(context.Context, int) ([]repository.HnROffender, error) {
	return nil, nil
}

var _ repository.HnRRepository = nopHnRRepo{}
