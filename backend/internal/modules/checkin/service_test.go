package checkin

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"transithub/backend/internal/modules/lottery"
	"transithub/backend/internal/modules/upstream"
)

type fakeStore struct {
	config      *Config
	records     []Record
	leaderboard []LeaderboardEntry
}

func (f *fakeStore) GetConfigByWorkspace(context.Context, string, string) (*Config, error) {
	return f.config, nil
}
func (f *fakeStore) GetConfigByToken(context.Context, string) (*Config, error) { return f.config, nil }
func (f *fakeStore) InsertConfig(context.Context, Config) error                { return nil }
func (f *fakeStore) SaveConfig(_ context.Context, config Config) error {
	f.config = &config
	return nil
}
func (f *fakeStore) RotateEmbedToken(context.Context, string, string, string) error   { return nil }
func (f *fakeStore) UpdateSourceOrigin(context.Context, string, string, string) error { return nil }
func (f *fakeStore) GetRecordForDate(_ context.Context, _, _, _ string, date time.Time) (*Record, error) {
	for i := range f.records {
		if f.records[i].CheckinDate.Equal(date) {
			return &f.records[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) GetLatestRecord(context.Context, string, string, string) (*Record, error) {
	var latest *Record
	for i := range f.records {
		if f.records[i].RewardStatus != RewardFulfilled {
			continue
		}
		if latest == nil || f.records[i].CheckinDate.After(latest.CheckinDate) {
			latest = &f.records[i]
		}
	}
	return latest, nil
}
func (f *fakeStore) InsertRecord(_ context.Context, record Record) (*Record, bool, error) {
	f.records = append(f.records, record)
	return &f.records[len(f.records)-1], true, nil
}
func (f *fakeStore) MarkReward(_ context.Context, id, status, remoteRef, errorKey, detail string) error {
	for i := range f.records {
		if f.records[i].ID == id {
			f.records[i].RewardStatus = status
			f.records[i].RemoteReference = remoteRef
			f.records[i].ErrorKey = errorKey
			f.records[i].ErrorDetail = detail
		}
	}
	return nil
}
func (f *fakeStore) ListUserRecords(context.Context, string, string, string, int) ([]Record, error) {
	return f.records, nil
}
func (f *fakeStore) ListWorkspaceRecords(context.Context, string, string, AdminRecordsQuery) ([]Record, int, error) {
	return f.records, len(f.records), nil
}
func (f *fakeStore) TodaySummary(context.Context, string, string, time.Time) (int, float64, error) {
	return 0, 0, nil
}
func (f *fakeStore) WorkspaceSummary(context.Context, string, string) (int, int, float64, error) {
	return len(f.leaderboard), len(f.records), 0, nil
}
func (f *fakeStore) ListLeaderboard(context.Context, string, string, int, string, string) ([]LeaderboardEntry, error) {
	return f.leaderboard, nil
}
func (f *fakeStore) GetLeaderboardEntry(_ context.Context, _, _, sub2apiUserID string) (*LeaderboardEntry, error) {
	for i := range f.leaderboard {
		if f.leaderboard[i].Sub2apiUserID == sub2apiUserID {
			return &f.leaderboard[i], nil
		}
	}
	return nil, nil
}

type fakeSessions struct{ session *EmbedSession }

func (f *fakeSessions) Save(context.Context, string, EmbedSession) error      { return nil }
func (f *fakeSessions) Get(context.Context, string) (*EmbedSession, error)    { return f.session, nil }
func (f *fakeSessions) DeleteWorkspace(context.Context, string, string) error { return nil }

type fakeAdminSessions struct{}

func (fakeAdminSessions) RequireSession(context.Context, string, string) (upstream.Session, error) {
	return upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://sub.example.com", AccessToken: "admin", TokenType: "Bearer"}, nil
}

type fakeViewer struct{}

func (fakeViewer) FetchCurrentUser(string, string) (lottery.Sub2APIUser, error) {
	return lottery.Sub2APIUser{}, nil
}

type fakeRewards struct {
	calls int
	job   lottery.RewardJob
}

func (f *fakeRewards) Redeem(_ context.Context, _ upstream.Session, job lottery.RewardJob) lottery.RewardResult {
	f.calls++
	f.job = job
	return lottery.RewardResult{Status: lottery.RewardFulfilled, RemoteRef: "reward-1"}
}

func TestCheckInAddsMilestoneBonusAndDeliversOnce(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	store := &fakeStore{
		config:  &Config{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", Sub2apiSourceOrigin: "https://sub.example.com", Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 10, Timezone: DefaultTimezone, Milestones: []Milestone{{Days: 7, BonusAmount: 2}}},
		records: []Record{{ID: "previous", CheckinDate: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), StreakDays: 6, RewardStatus: RewardFulfilled}},
	}
	rewards := &fakeRewards{}
	service := NewService(store, &fakeSessions{session: &EmbedSession{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", SrcHost: "https://sub.example.com", Sub2apiUserID: "42", Sub2apiEmailMasked: "u***@example.com"}}, fakeViewer{}, rewards, fakeAdminSessions{})
	service.now = func() time.Time { return now }
	service.randomReward = func(float64, float64) (float64, error) { return 0.4, nil }

	result, err := service.CheckIn(context.Background(), "session")
	if err != nil {
		t.Fatalf("CheckIn() error = %v", err)
	}
	if result.StreakDays != 7 || result.BaseReward != 0.4 || result.MilestoneReward != 2 || result.TotalReward != 2.4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if rewards.calls != 1 || rewards.job.Prize.BalanceAmount != "2.400000" || rewards.job.Winner.Sub2apiUserID != "42" {
		t.Fatalf("unexpected reward call: calls=%d job=%+v", rewards.calls, rewards.job)
	}
}

func TestCheckInCapsCombinedDailyReward(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	store := &fakeStore{
		config:  &Config{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", Sub2apiSourceOrigin: "https://sub.example.com", Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 1.5, Timezone: DefaultTimezone, Milestones: []Milestone{{Days: 7, BonusAmount: 2}}},
		records: []Record{{ID: "previous", CheckinDate: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), StreakDays: 6, RewardStatus: RewardFulfilled}},
	}
	rewards := &fakeRewards{}
	service := NewService(store, &fakeSessions{session: &EmbedSession{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", SrcHost: "https://sub.example.com", Sub2apiUserID: "42"}}, fakeViewer{}, rewards, fakeAdminSessions{})
	service.now = func() time.Time { return now }
	service.randomReward = func(float64, float64) (float64, error) { return 0.8, nil }

	result, err := service.CheckIn(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseReward != 0.8 || result.MilestoneReward != 0.7 || result.TotalReward != 1.5 {
		t.Fatalf("daily cap not enforced: %+v", result)
	}
	if rewards.job.Prize.BalanceAmount != "1.500000" {
		t.Fatalf("unexpected delivered amount: %s", rewards.job.Prize.BalanceAmount)
	}
}

func TestCheckInRejectsSecondRewardForSameDay(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	store := &fakeStore{
		config:  &Config{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", Sub2apiSourceOrigin: "https://sub.example.com", Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 10, Timezone: DefaultTimezone},
		records: []Record{{ID: "today", CheckinDate: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), StreakDays: 1, TotalReward: 0.5, RewardStatus: RewardFulfilled}},
	}
	rewards := &fakeRewards{}
	service := NewService(store, &fakeSessions{session: &EmbedSession{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", SrcHost: "https://sub.example.com", Sub2apiUserID: "42"}}, fakeViewer{}, rewards, fakeAdminSessions{})
	service.now = func() time.Time { return now }

	_, err := service.CheckIn(context.Background(), "session")
	if !errors.Is(err, requestError(ErrorEmbedAlreadyChecked)) {
		t.Fatalf("error = %v, want already checked", err)
	}
	if rewards.calls != 0 {
		t.Fatalf("reward calls = %d, want 0", rewards.calls)
	}
}

func TestValidateConfigRequestRejectsDuplicateMilestoneDays(t *testing.T) {
	req := UpdateConfigRequest{Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 10, Timezone: DefaultTimezone, Milestones: []Milestone{{Days: 7, BonusAmount: 1}, {Days: 7, BonusAmount: 2}}}
	if !errors.Is(validateConfigRequest(req), requestError(ErrorValidation)) {
		t.Fatal("duplicate milestone days should be rejected")
	}
}

func TestSecureRandomRewardStaysInsideConfiguredRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		value, err := secureRandomReward(0.1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if value < 0.1 || value > 1 {
			t.Fatalf("reward %v outside range", value)
		}
		if math.Abs(value*100-math.Round(value*100)) > 1e-9 {
			t.Fatalf("reward %v has more than two decimals", value)
		}
	}
}

func TestEmbedStatusIncludesUserStatsAndLeaderboard(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	store := &fakeStore{
		config:      &Config{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", Sub2apiSourceOrigin: "https://sub.example.com", Enabled: true, DailyMin: 0.1, DailyMax: 1, DailyUserRewardCap: 10, Timezone: DefaultTimezone},
		leaderboard: []LeaderboardEntry{{Rank: 2, Sub2apiUserID: "42", MaskedEmail: "u***@example.com", TotalDays: 18, LatestStreak: 6, LongestStreak: 9, TotalRewards: 12.4, LastCheckinDate: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)}},
	}
	service := NewService(store, &fakeSessions{session: &EmbedSession{UserID: "owner", AdminAccountID: "workspace", EmbedToken: "embed", SrcHost: "https://sub.example.com", Sub2apiUserID: "42"}}, fakeViewer{}, &fakeRewards{}, fakeAdminSessions{})
	service.now = func() time.Time { return now }

	status, err := service.EmbedStatus(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if status.TotalDays != 18 || status.CurrentStreak != 6 || status.LongestStreak != 9 || status.UserRank != 2 || status.TotalRewards != 12.4 {
		t.Fatalf("unexpected user summary: %+v", status)
	}
	if len(status.Leaderboard) != 1 || !status.Leaderboard[0].IsCurrentUser {
		t.Fatalf("unexpected leaderboard: %+v", status.Leaderboard)
	}
	if status.Leaderboard[0].Email != "" || status.Leaderboard[0].MaskedEmail != "" {
		t.Fatalf("embed leaderboard exposed email: %+v", status.Leaderboard[0])
	}
}

func TestLeaderboardPeriodRangeIncludesCurrentDay(t *testing.T) {
	today := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	tests := map[string]string{"today": "2026-07-15", "7d": "2026-07-09", "30d": "2026-06-16", "all": ""}
	for period, want := range tests {
		_, got, err := leaderboardPeriodRange(period, today)
		if err != nil || got != want {
			t.Fatalf("period %s: got %q, %v; want %q", period, got, err, want)
		}
	}
}
