package checkin

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/modules/lottery"
	"transithub/backend/internal/modules/upstream"
)

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

type AdminSessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

type store interface {
	GetConfigByWorkspace(context.Context, string, string) (*Config, error)
	GetConfigByToken(context.Context, string) (*Config, error)
	InsertConfig(context.Context, Config) error
	SaveConfig(context.Context, Config) error
	RotateEmbedToken(context.Context, string, string, string) error
	UpdateSourceOrigin(context.Context, string, string, string) error
	GetRecordForDate(context.Context, string, string, string, time.Time) (*Record, error)
	GetLatestRecord(context.Context, string, string, string) (*Record, error)
	InsertRecord(context.Context, Record) (*Record, bool, error)
	MarkReward(context.Context, string, string, string, string, string) error
	ListUserRecords(context.Context, string, string, string, int) ([]Record, error)
	ListWorkspaceRecords(context.Context, string, string, AdminRecordsQuery) ([]Record, int, error)
	TodaySummary(context.Context, string, string, time.Time) (int, float64, error)
	WorkspaceSummary(context.Context, string, string) (int, int, float64, error)
	ListLeaderboard(context.Context, string, string, int, string, string) ([]LeaderboardEntry, error)
	GetLeaderboardEntry(context.Context, string, string, string) (*LeaderboardEntry, error)
}

type sessionStore interface {
	Save(context.Context, string, EmbedSession) error
	Get(context.Context, string) (*EmbedSession, error)
	DeleteWorkspace(context.Context, string, string) error
}

type viewerClient interface {
	FetchCurrentUser(srcHost string, token string) (lottery.Sub2APIUser, error)
}

type rewardClient interface {
	Redeem(ctx context.Context, session upstream.Session, job lottery.RewardJob) lottery.RewardResult
}

const (
	maxRewardAmount   = 1_000_000_000
	pendingRetryAfter = 2 * time.Minute
)

type Service struct {
	repository          store
	sessions            sessionStore
	viewer              viewerClient
	rewards             rewardClient
	accounts            AdminAccountResolver
	adminSessions       AdminSessionProvider
	allowPrivateTargets bool
	now                 func() time.Time
	newToken            func() (string, error)
	randomReward        func(float64, float64) (float64, error)
}

func NewService(repository store, sessions sessionStore, viewer viewerClient, rewards rewardClient, adminSessions AdminSessionProvider) *Service {
	return &Service{repository: repository, sessions: sessions, viewer: viewer, rewards: rewards, adminSessions: adminSessions, now: time.Now, newToken: func() (string, error) { return randomHex(32) }, randomReward: secureRandomReward}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	if repository, ok := s.repository.(*Repository); ok {
		return repository.EnsureSchema(ctx)
	}
	return nil
}
func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) { s.accounts = accounts }
func (s *Service) SetAllowPrivateTargets(allow bool)                     { s.allowPrivateTargets = allow }

func (s *Service) GetConfig(ctx context.Context, userID string) (ConfigResponse, error) {
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return ConfigResponse{}, err
	}
	return configResponse(*config), nil
}

func (s *Service) UpdateConfig(ctx context.Context, userID string, req UpdateConfigRequest) (ConfigResponse, error) {
	if err := validateConfigRequest(req); err != nil {
		return ConfigResponse{}, err
	}
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return ConfigResponse{}, err
	}
	config.Enabled = req.Enabled
	config.DailyMin = roundMoney(req.DailyMin)
	config.DailyMax = roundMoney(req.DailyMax)
	config.DailyUserRewardCap = roundMoney(req.DailyUserRewardCap)
	config.Timezone = strings.TrimSpace(req.Timezone)
	config.Milestones = append([]Milestone(nil), req.Milestones...)
	sort.Slice(config.Milestones, func(i, j int) bool { return config.Milestones[i].Days < config.Milestones[j].Days })
	if err := s.repository.SaveConfig(ctx, *config); err != nil {
		return ConfigResponse{}, err
	}
	return s.GetConfig(ctx, userID)
}

func (s *Service) RotateEmbedToken(ctx context.Context, userID string) (ConfigResponse, error) {
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return ConfigResponse{}, err
	}
	token, err := s.newToken()
	if err != nil {
		return ConfigResponse{}, err
	}
	if err := s.repository.RotateEmbedToken(ctx, config.UserID, config.AdminAccountID, token); err != nil {
		return ConfigResponse{}, err
	}
	if err := s.sessions.DeleteWorkspace(ctx, config.UserID, config.AdminAccountID); err != nil {
		return ConfigResponse{}, err
	}
	return s.GetConfig(ctx, userID)
}

func (s *Service) AdminOverview(ctx context.Context, userID string) (AdminOverviewResponse, error) {
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return AdminOverviewResponse{}, err
	}
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return AdminOverviewResponse{}, requestError(ErrorValidation)
	}
	today := dateAt(s.now(), loc)
	records, recordsTotal, err := s.repository.ListWorkspaceRecords(ctx, config.UserID, config.AdminAccountID, AdminRecordsQuery{Page: 1, PageSize: 20})
	if err != nil {
		return AdminOverviewResponse{}, err
	}
	count, rewards, err := s.repository.TodaySummary(ctx, config.UserID, config.AdminAccountID, today)
	if err != nil {
		return AdminOverviewResponse{}, err
	}
	totalUsers, totalCheckins, totalRewards, err := s.repository.WorkspaceSummary(ctx, config.UserID, config.AdminAccountID)
	if err != nil {
		return AdminOverviewResponse{}, err
	}
	leaderboard, err := s.repository.ListLeaderboard(ctx, config.UserID, config.AdminAccountID, 50, "", today.Format(time.DateOnly))
	if err != nil {
		return AdminOverviewResponse{}, err
	}
	return AdminOverviewResponse{
		Config:        configResponse(*config),
		Records:       recordResponses(records, true),
		Leaderboard:   leaderboardResponses(leaderboard, today, "", true),
		TodayCount:    count,
		TodayRewards:  rewards,
		TotalUsers:    totalUsers,
		TotalCheckins: totalCheckins,
		TotalRewards:  totalRewards,
		RecordsTotal:  recordsTotal,
	}, nil
}

func (s *Service) AdminRecords(ctx context.Context, userID string, query AdminRecordsQuery) (AdminRecordsResponse, error) {
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return AdminRecordsResponse{}, err
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	if err := validateDateRange(query.DateFrom, query.DateTo); err != nil {
		return AdminRecordsResponse{}, err
	}
	query.UserQuery = strings.TrimSpace(query.UserQuery)
	if len(query.UserQuery) > 200 {
		return AdminRecordsResponse{}, requestError(ErrorValidation)
	}
	records, total, err := s.repository.ListWorkspaceRecords(ctx, config.UserID, config.AdminAccountID, query)
	if err != nil {
		return AdminRecordsResponse{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	return AdminRecordsResponse{Items: recordResponses(records, true), Page: query.Page, PageSize: query.PageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *Service) AdminLeaderboard(ctx context.Context, userID, period string) (AdminLeaderboardResponse, error) {
	config, err := s.requireConfig(ctx, userID)
	if err != nil {
		return AdminLeaderboardResponse{}, err
	}
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return AdminLeaderboardResponse{}, requestError(ErrorValidation)
	}
	today := dateAt(s.now(), loc)
	period, dateFrom, err := leaderboardPeriodRange(period, today)
	if err != nil {
		return AdminLeaderboardResponse{}, err
	}
	entries, err := s.repository.ListLeaderboard(ctx, config.UserID, config.AdminAccountID, 50, dateFrom, today.Format(time.DateOnly))
	if err != nil {
		return AdminLeaderboardResponse{}, err
	}
	return AdminLeaderboardResponse{Period: period, Entries: leaderboardResponses(entries, today, "", true)}, nil
}

func (s *Service) CreateEmbedSession(ctx context.Context, req CreateSessionRequest) (CreateSessionResponse, error) {
	viewerToken := strings.TrimSpace(req.Sub2apiToken)
	if viewerToken == "" {
		viewerToken = strings.TrimSpace(req.ViewerToken)
	}
	if strings.TrimSpace(req.EmbedToken) == "" || viewerToken == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedRequest)
	}
	config, err := s.repository.GetConfigByToken(ctx, strings.TrimSpace(req.EmbedToken))
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if config == nil {
		return CreateSessionResponse{}, requestError(ErrorEmbedConfigNotFound)
	}
	origin, err := lottery.NormalizeSrcHostForEmbed(req.SrcHost, s.allowPrivateTargets)
	if err != nil {
		return CreateSessionResponse{}, requestError(ErrorEmbedInvalidSrcHost)
	}
	if origin != config.Sub2apiSourceOrigin {
		return CreateSessionResponse{}, requestError(ErrorEmbedSrcHostMismatch)
	}
	if _, err := s.validateSourceBinding(ctx, config.UserID, config.AdminAccountID, origin); err != nil {
		return CreateSessionResponse{}, err
	}
	user, err := s.viewer.FetchCurrentUser(origin, viewerToken)
	if err != nil {
		if lottery.IsViewerUnauthorized(err) {
			return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiAuth)
		}
		return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiRequest)
	}
	if strings.TrimSpace(req.URLUserID) != "" && strings.TrimSpace(req.URLUserID) != user.ID {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserMismatch)
	}
	if !lottery.ViewerActive(user.Status) {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserInactive)
	}
	token, err := s.newToken()
	if err != nil {
		return CreateSessionResponse{}, err
	}
	session := EmbedSession{UserID: config.UserID, AdminAccountID: config.AdminAccountID, EmbedToken: config.EmbedToken, SrcHost: origin, SrcURL: strings.TrimSpace(req.SrcURL), Sub2apiUserID: user.ID, Sub2apiEmail: strings.TrimSpace(user.Email), Sub2apiEmailMasked: lottery.MaskEmail(user.Email), CreatedAt: s.now()}
	if err := s.sessions.Save(ctx, token, session); err != nil {
		return CreateSessionResponse{}, err
	}
	return CreateSessionResponse{SessionToken: token}, nil
}

func (s *Service) EmbedStatus(ctx context.Context, sessionToken string) (EmbedStatusResponse, error) {
	session, config, err := s.requireEmbedContext(ctx, sessionToken)
	if err != nil {
		return EmbedStatusResponse{}, err
	}
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return EmbedStatusResponse{}, requestError(ErrorValidation)
	}
	todayDate := dateAt(s.now(), loc)
	today, err := s.repository.GetRecordForDate(ctx, session.UserID, session.AdminAccountID, session.Sub2apiUserID, todayDate)
	if err != nil {
		return EmbedStatusResponse{}, err
	}
	history, err := s.repository.ListUserRecords(ctx, session.UserID, session.AdminAccountID, session.Sub2apiUserID, 30)
	if err != nil {
		return EmbedStatusResponse{}, err
	}
	leaderboard, err := s.repository.ListLeaderboard(ctx, session.UserID, session.AdminAccountID, 10, "", todayDate.Format(time.DateOnly))
	if err != nil {
		return EmbedStatusResponse{}, err
	}
	userSummary, err := s.repository.GetLeaderboardEntry(ctx, session.UserID, session.AdminAccountID, session.Sub2apiUserID)
	if err != nil {
		return EmbedStatusResponse{}, err
	}
	streak, longestStreak, totalDays, totalRewards, userRank := 0, 0, 0, 0.0, 0
	if userSummary != nil {
		streak = activeStreak(*userSummary, todayDate)
		longestStreak = userSummary.LongestStreak
		totalDays = userSummary.TotalDays
		totalRewards = userSummary.TotalRewards
		userRank = userSummary.Rank
	}
	var todayResponse *RecordResponse
	if today != nil {
		value := recordResponse(*today, false)
		todayResponse = &value
	}
	return EmbedStatusResponse{
		Enabled:       config.Enabled,
		DailyMin:      config.DailyMin,
		DailyMax:      config.DailyMax,
		Timezone:      config.Timezone,
		Milestones:    config.Milestones,
		CheckedToday:  today != nil && today.RewardStatus == RewardFulfilled,
		CurrentStreak: streak,
		LongestStreak: longestStreak,
		TotalDays:     totalDays,
		TotalRewards:  totalRewards,
		UserRank:      userRank,
		Today:         todayResponse,
		History:       recordResponses(history, false),
		Leaderboard:   leaderboardResponses(leaderboard, todayDate, session.Sub2apiUserID, false),
	}, nil
}

func (s *Service) CheckIn(ctx context.Context, sessionToken string) (RecordResponse, error) {
	session, config, err := s.requireEmbedContext(ctx, sessionToken)
	if err != nil {
		return RecordResponse{}, err
	}
	if !config.Enabled {
		return RecordResponse{}, requestError(ErrorEmbedDisabled)
	}
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return RecordResponse{}, requestError(ErrorValidation)
	}
	today := dateAt(s.now(), loc)
	existing, err := s.repository.GetRecordForDate(ctx, session.UserID, session.AdminAccountID, session.Sub2apiUserID, today)
	if err != nil {
		return RecordResponse{}, err
	}
	if existing != nil && existing.RewardStatus == RewardFulfilled {
		return recordResponse(*existing, false), requestError(ErrorEmbedAlreadyChecked)
	}
	if existing != nil && existing.RewardStatus == RewardPending && s.now().Sub(existing.UpdatedAt) < pendingRetryAfter {
		return RecordResponse{}, requestError(ErrorEmbedRewardFailed)
	}
	if existing != nil && existing.RewardStatus == RewardFailed {
		return RecordResponse{}, requestError(ErrorEmbedRewardFailed)
	}

	record := existing
	if record == nil {
		baseReward, err := s.randomReward(config.DailyMin, config.DailyMax)
		if err != nil {
			return RecordResponse{}, err
		}
		streak := 1
		latest, err := s.repository.GetLatestRecord(ctx, session.UserID, session.AdminAccountID, session.Sub2apiUserID)
		if err != nil {
			return RecordResponse{}, err
		}
		if latest != nil && dateAt(latest.CheckinDate, loc).Equal(today.AddDate(0, 0, -1)) {
			streak = latest.StreakDays + 1
		}
		bonus := milestoneBonus(config.Milestones, streak)
		if available := roundMoney(config.DailyUserRewardCap - baseReward); bonus > available {
			bonus = math.Max(0, available)
		}
		id, err := newID("cin")
		if err != nil {
			return RecordResponse{}, err
		}
		candidate := Record{ID: id, UserID: session.UserID, AdminAccountID: session.AdminAccountID, Sub2apiUserID: session.Sub2apiUserID, Email: session.Sub2apiEmail, MaskedEmail: session.Sub2apiEmailMasked, CheckinDate: today, StreakDays: streak, BaseReward: baseReward, MilestoneReward: bonus, TotalReward: roundMoney(baseReward + bonus), RewardStatus: RewardPending, IdempotencyKey: "checkin:" + id}
		var created bool
		record, created, err = s.repository.InsertRecord(ctx, candidate)
		if err != nil {
			return RecordResponse{}, err
		}
		if !created {
			if record != nil && record.RewardStatus == RewardFulfilled {
				return recordResponse(*record, false), requestError(ErrorEmbedAlreadyChecked)
			}
			return RecordResponse{}, requestError(ErrorEmbedRewardFailed)
		}
	}
	adminSession, err := s.validateSourceBinding(ctx, session.UserID, session.AdminAccountID, session.SrcHost)
	if err != nil {
		_ = s.repository.MarkReward(ctx, record.ID, RewardRetryableFailed, "", ErrorEmbedAdminSession, err.Error())
		return RecordResponse{}, err
	}
	job := lottery.RewardJob{ID: record.ID, CampaignID: "checkin", WinnerID: record.ID, UserID: record.UserID, AdminAccountID: record.AdminAccountID, IdempotencyKey: record.IdempotencyKey, Winner: lottery.Winner{Sub2apiUserID: record.Sub2apiUserID}, Prize: lottery.Prize{Type: lottery.PrizeTypeBalance, BalanceAmount: fmt.Sprintf("%.6f", record.TotalReward)}}
	result := s.rewards.Redeem(ctx, adminSession, job)
	status := RewardRetryableFailed
	if result.Status == lottery.RewardFulfilled {
		status = RewardFulfilled
	} else if result.Status == lottery.RewardFailed || result.Status == lottery.RewardManualAttention {
		status = RewardFailed
	}
	if err := s.repository.MarkReward(ctx, record.ID, status, result.RemoteRef, result.ErrorKey, result.Detail); err != nil {
		return RecordResponse{}, err
	}
	if status != RewardFulfilled {
		return RecordResponse{}, requestError(ErrorEmbedRewardFailed)
	}
	record.RewardStatus = status
	record.RemoteReference = result.RemoteRef
	return recordResponse(*record, false), nil
}

func (s *Service) FrameAncestorOrigin(ctx context.Context, embedToken string) (string, bool) {
	config, err := s.repository.GetConfigByToken(ctx, strings.TrimSpace(embedToken))
	if err != nil || config == nil {
		return "", false
	}
	if _, err := s.validateSourceBinding(ctx, config.UserID, config.AdminAccountID, config.Sub2apiSourceOrigin); err != nil {
		return "", false
	}
	return config.Sub2apiSourceOrigin, true
}

func (s *Service) requireConfig(ctx context.Context, userID string) (*Config, error) {
	if s.accounts == nil {
		return nil, requestError(ErrorNoCurrentAccount)
	}
	adminAccountID, err := s.accounts.RequireCurrentID(ctx, userID)
	if err != nil {
		return nil, requestError(ErrorNoCurrentAccount)
	}
	adminSession, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil || adminSession.Platform != upstream.PlatformSub2API {
		return nil, requestError(ErrorAdminOnly)
	}
	origin, err := lottery.NormalizeSrcHostForEmbed(adminSession.BaseURL, s.allowPrivateTargets)
	if err != nil {
		return nil, requestError(ErrorInvalidSourceOrigin)
	}
	config, err := s.repository.GetConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		token, err := s.newToken()
		if err != nil {
			return nil, err
		}
		config = &Config{UserID: userID, AdminAccountID: adminAccountID, EmbedToken: token, Sub2apiSourceOrigin: origin, Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 10, Timezone: DefaultTimezone, Milestones: []Milestone{{Days: 7, BonusAmount: 1}}}
		if err := s.repository.InsertConfig(ctx, *config); err != nil {
			return nil, err
		}
		if err := s.repository.SaveConfig(ctx, *config); err != nil {
			return nil, err
		}
		return s.repository.GetConfigByWorkspace(ctx, userID, adminAccountID)
	}
	if config.Sub2apiSourceOrigin != origin {
		if err := s.repository.UpdateSourceOrigin(ctx, userID, adminAccountID, origin); err != nil {
			return nil, err
		}
		_ = s.sessions.DeleteWorkspace(ctx, userID, adminAccountID)
		config.Sub2apiSourceOrigin = origin
	}
	return config, nil
}

func (s *Service) requireEmbedContext(ctx context.Context, token string) (*EmbedSession, *Config, error) {
	session, err := s.sessions.Get(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, requestError(ErrorEmbedSessionInvalid)
	}
	config, err := s.repository.GetConfigByWorkspace(ctx, session.UserID, session.AdminAccountID)
	if err != nil {
		return nil, nil, err
	}
	if config == nil || config.EmbedToken != session.EmbedToken {
		return nil, nil, requestError(ErrorEmbedSessionInvalid)
	}
	if _, err := s.validateSourceBinding(ctx, session.UserID, session.AdminAccountID, session.SrcHost); err != nil {
		return nil, nil, err
	}
	return session, config, nil
}

func (s *Service) validateSourceBinding(ctx context.Context, userID, adminAccountID, origin string) (upstream.Session, error) {
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, requestError(ErrorEmbedAdminSession)
	}
	current, err := lottery.NormalizeSrcHostForEmbed(session.BaseURL, s.allowPrivateTargets)
	if err != nil || session.Platform != upstream.PlatformSub2API || current != origin {
		return upstream.Session{}, requestError(ErrorEmbedSourceBinding)
	}
	return session, nil
}

func validateConfigRequest(req UpdateConfigRequest) error {
	if req.DailyMin <= 0 || req.DailyMax < req.DailyMin || req.DailyMax > req.DailyUserRewardCap || req.DailyUserRewardCap > maxRewardAmount || math.IsNaN(req.DailyMin) || math.IsNaN(req.DailyMax) || math.IsNaN(req.DailyUserRewardCap) || math.IsInf(req.DailyMin, 0) || math.IsInf(req.DailyMax, 0) || math.IsInf(req.DailyUserRewardCap, 0) {
		return requestError(ErrorValidation)
	}
	if _, err := time.LoadLocation(strings.TrimSpace(req.Timezone)); err != nil {
		return requestError(ErrorValidation)
	}
	seen := map[int]struct{}{}
	for _, item := range req.Milestones {
		if item.Days < 2 || item.Days > 3650 || item.BonusAmount <= 0 || item.BonusAmount > maxRewardAmount || math.IsNaN(item.BonusAmount) || math.IsInf(item.BonusAmount, 0) {
			return requestError(ErrorValidation)
		}
		if _, ok := seen[item.Days]; ok {
			return requestError(ErrorValidation)
		}
		seen[item.Days] = struct{}{}
	}
	return nil
}

func milestoneBonus(items []Milestone, streak int) float64 {
	for _, item := range items {
		if item.Days == streak {
			return roundMoney(item.BonusAmount)
		}
	}
	return 0
}

func validateDateRange(dateFrom, dateTo string) error {
	var from, to time.Time
	var err error
	if dateFrom != "" {
		from, err = time.Parse(time.DateOnly, dateFrom)
		if err != nil {
			return requestError(ErrorValidation)
		}
	}
	if dateTo != "" {
		to, err = time.Parse(time.DateOnly, dateTo)
		if err != nil {
			return requestError(ErrorValidation)
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return requestError(ErrorValidation)
	}
	return nil
}

func leaderboardPeriodRange(period string, today time.Time) (string, string, error) {
	switch strings.TrimSpace(period) {
	case "", "all":
		return "all", "", nil
	case "today":
		return "today", today.Format(time.DateOnly), nil
	case "7d":
		return "7d", today.AddDate(0, 0, -6).Format(time.DateOnly), nil
	case "30d":
		return "30d", today.AddDate(0, 0, -29).Format(time.DateOnly), nil
	default:
		return "", "", requestError(ErrorValidation)
	}
}

func secureRandomReward(min, max float64) (float64, error) {
	minUnits, maxUnits := int64(math.Round(min*100)), int64(math.Round(max*100))
	if minUnits <= 0 || maxUnits < minUnits {
		return 0, requestError(ErrorValidation)
	}
	span := uint64(maxUnits - minUnits + 1)
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return float64(minUnits+int64(binary.BigEndian.Uint64(buf[:])%span)) / 100, nil
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func newID(prefix string) (string, error) {
	token, err := randomHex(16)
	if err != nil {
		return "", err
	}
	return prefix + "_" + token, nil
}
func roundMoney(value float64) float64 { return math.Round(value*1_000_000) / 1_000_000 }
func dateAt(value time.Time, loc *time.Location) time.Time {
	local := value.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
func activeStreak(entry LeaderboardEntry, today time.Time) int {
	lastDate := dateAt(entry.LastCheckinDate, time.UTC)
	if lastDate.Equal(today) || lastDate.Equal(today.AddDate(0, 0, -1)) {
		return entry.LatestStreak
	}
	return 0
}

func configResponse(config Config) ConfigResponse {
	return ConfigResponse{EmbedToken: config.EmbedToken, Sub2apiSourceOrigin: config.Sub2apiSourceOrigin, Enabled: config.Enabled, DailyMin: config.DailyMin, DailyMax: config.DailyMax, DailyUserRewardCap: config.DailyUserRewardCap, Timezone: config.Timezone, Milestones: config.Milestones, CreatedAt: config.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: config.UpdatedAt.UTC().Format(time.RFC3339)}
}
func recordResponse(record Record, includeEmail bool) RecordResponse {
	email, masked := "", ""
	if includeEmail {
		email = record.Email
		masked = record.MaskedEmail
	}
	return RecordResponse{ID: record.ID, Email: email, MaskedEmail: masked, CheckinDate: record.CheckinDate.Format(time.DateOnly), StreakDays: record.StreakDays, BaseReward: record.BaseReward, MilestoneReward: record.MilestoneReward, TotalReward: record.TotalReward, RewardStatus: record.RewardStatus, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339)}
}
func recordResponses(records []Record, includeEmail bool) []RecordResponse {
	items := make([]RecordResponse, 0, len(records))
	for _, record := range records {
		items = append(items, recordResponse(record, includeEmail))
	}
	return items
}

func leaderboardResponses(entries []LeaderboardEntry, today time.Time, currentUserID string, includeEmail bool) []LeaderboardEntryResponse {
	items := make([]LeaderboardEntryResponse, 0, len(entries))
	for _, entry := range entries {
		email, masked := "", ""
		if includeEmail {
			email = entry.Email
			masked = entry.MaskedEmail
		}
		items = append(items, LeaderboardEntryResponse{
			Rank:            entry.Rank,
			Email:           email,
			MaskedEmail:     masked,
			TotalDays:       entry.TotalDays,
			CurrentStreak:   activeStreak(entry, today),
			LongestStreak:   entry.LongestStreak,
			TotalRewards:    entry.TotalRewards,
			LastCheckinDate: entry.LastCheckinDate.Format(time.DateOnly),
			IsCurrentUser:   currentUserID != "" && entry.Sub2apiUserID == currentUserID,
		})
	}
	return items
}
