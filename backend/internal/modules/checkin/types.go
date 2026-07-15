package checkin

import "time"

const (
	ErrorRequest             = "admin.checkin.errors.request"
	ErrorUnknown             = "admin.checkin.errors.unknown"
	ErrorNoCurrentAccount    = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorAdminOnly           = "admin.dashboard.adminAuth.errors.adminOnly"
	ErrorInvalidSourceOrigin = "admin.checkin.errors.invalidSourceOrigin"
	ErrorValidation          = "admin.checkin.errors.validation"

	ErrorEmbedRequest         = "embed.checkin.errors.request"
	ErrorEmbedConfigNotFound  = "embed.checkin.errors.configNotFound"
	ErrorEmbedInvalidSrcHost  = "embed.checkin.errors.invalidSrcHost"
	ErrorEmbedSrcHostMismatch = "embed.checkin.errors.srcHostMismatch"
	ErrorEmbedSub2apiAuth     = "embed.checkin.errors.sub2apiAuth"
	ErrorEmbedSub2apiRequest  = "embed.checkin.errors.sub2apiRequest"
	ErrorEmbedUserMismatch    = "embed.checkin.errors.userMismatch"
	ErrorEmbedUserInactive    = "embed.checkin.errors.userInactive"
	ErrorEmbedSessionInvalid  = "embed.checkin.errors.sessionInvalid"
	ErrorEmbedAdminSession    = "embed.checkin.errors.adminSession"
	ErrorEmbedSourceBinding   = "embed.checkin.errors.sourceBinding"
	ErrorEmbedDisabled        = "embed.checkin.errors.disabled"
	ErrorEmbedAlreadyChecked  = "embed.checkin.errors.alreadyChecked"
	ErrorEmbedRewardFailed    = "embed.checkin.errors.rewardFailed"
)

const (
	RewardPending         = "pending"
	RewardFulfilled       = "fulfilled"
	RewardRetryableFailed = "retryable_failed"
	RewardFailed          = "failed"
	DefaultTimezone       = "Asia/Shanghai"
)

type requestError string

func (e requestError) Error() string { return string(e) }

type Milestone struct {
	Days        int     `json:"days"`
	BonusAmount float64 `json:"bonusAmount"`
}

type Config struct {
	UserID              string
	AdminAccountID      string
	EmbedToken          string
	Sub2apiSourceOrigin string
	Enabled             bool
	DailyMin            float64
	DailyMax            float64
	DailyUserRewardCap  float64
	Timezone            string
	Milestones          []Milestone
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Record struct {
	ID              string
	UserID          string
	AdminAccountID  string
	Sub2apiUserID   string
	Email           string
	MaskedEmail     string
	CheckinDate     time.Time
	StreakDays      int
	BaseReward      float64
	MilestoneReward float64
	TotalReward     float64
	RewardStatus    string
	AttemptCount    int
	IdempotencyKey  string
	RemoteReference string
	ErrorKey        string
	ErrorDetail     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	FulfilledAt     *time.Time
}

type LeaderboardEntry struct {
	Rank            int
	Sub2apiUserID   string
	Email           string
	MaskedEmail     string
	TotalDays       int
	LatestStreak    int
	LongestStreak   int
	TotalRewards    float64
	LastCheckinDate time.Time
}

type EmbedSession struct {
	UserID             string
	AdminAccountID     string
	EmbedToken         string
	SrcHost            string
	SrcURL             string
	Sub2apiUserID      string
	Sub2apiEmail       string
	Sub2apiEmailMasked string
	CreatedAt          time.Time
}

type UpdateConfigRequest struct {
	Enabled            bool        `json:"enabled"`
	DailyMin           float64     `json:"dailyMin"`
	DailyMax           float64     `json:"dailyMax"`
	DailyUserRewardCap float64     `json:"dailyUserRewardCap"`
	Timezone           string      `json:"timezone"`
	Milestones         []Milestone `json:"milestones"`
}

type CreateSessionRequest struct {
	EmbedToken   string `json:"embedToken"`
	Sub2apiToken string `json:"sub2apiToken"`
	ViewerToken  string `json:"viewerToken"`
	SrcHost      string `json:"srcHost"`
	SrcURL       string `json:"srcUrl"`
	URLUserID    string `json:"userId"`
}

type CreateSessionResponse struct {
	SessionToken string `json:"sessionToken"`
}

type ConfigResponse struct {
	EmbedToken          string      `json:"embedToken"`
	Sub2apiSourceOrigin string      `json:"sub2apiSourceOrigin"`
	Enabled             bool        `json:"enabled"`
	DailyMin            float64     `json:"dailyMin"`
	DailyMax            float64     `json:"dailyMax"`
	DailyUserRewardCap  float64     `json:"dailyUserRewardCap"`
	Timezone            string      `json:"timezone"`
	Milestones          []Milestone `json:"milestones"`
	CreatedAt           string      `json:"createdAt"`
	UpdatedAt           string      `json:"updatedAt"`
}

type RecordResponse struct {
	ID              string  `json:"id"`
	Email           string  `json:"email,omitempty"`
	MaskedEmail     string  `json:"maskedEmail,omitempty"`
	CheckinDate     string  `json:"checkinDate"`
	StreakDays      int     `json:"streakDays"`
	BaseReward      float64 `json:"baseReward"`
	MilestoneReward float64 `json:"milestoneReward"`
	TotalReward     float64 `json:"totalReward"`
	RewardStatus    string  `json:"rewardStatus"`
	CreatedAt       string  `json:"createdAt"`
}

type AdminOverviewResponse struct {
	Config        ConfigResponse             `json:"config"`
	Records       []RecordResponse           `json:"records"`
	Leaderboard   []LeaderboardEntryResponse `json:"leaderboard"`
	TodayCount    int                        `json:"todayCount"`
	TodayRewards  float64                    `json:"todayRewards"`
	TotalUsers    int                        `json:"totalUsers"`
	TotalCheckins int                        `json:"totalCheckins"`
	TotalRewards  float64                    `json:"totalRewards"`
	RecordsTotal  int                        `json:"recordsTotal"`
}

type AdminRecordsQuery struct {
	Page      int
	PageSize  int
	DateFrom  string
	DateTo    string
	UserQuery string
}

type AdminRecordsResponse struct {
	Items      []RecordResponse `json:"items"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	Total      int              `json:"total"`
	TotalPages int              `json:"totalPages"`
}

type AdminLeaderboardResponse struct {
	Period  string                     `json:"period"`
	Entries []LeaderboardEntryResponse `json:"entries"`
}

type LeaderboardEntryResponse struct {
	Rank            int     `json:"rank"`
	Email           string  `json:"email,omitempty"`
	MaskedEmail     string  `json:"maskedEmail,omitempty"`
	TotalDays       int     `json:"totalDays"`
	CurrentStreak   int     `json:"currentStreak"`
	LongestStreak   int     `json:"longestStreak"`
	TotalRewards    float64 `json:"totalRewards"`
	LastCheckinDate string  `json:"lastCheckinDate"`
	IsCurrentUser   bool    `json:"isCurrentUser,omitempty"`
}

type EmbedStatusResponse struct {
	Enabled       bool                       `json:"enabled"`
	DailyMin      float64                    `json:"dailyMin"`
	DailyMax      float64                    `json:"dailyMax"`
	Timezone      string                     `json:"timezone"`
	Milestones    []Milestone                `json:"milestones"`
	CheckedToday  bool                       `json:"checkedToday"`
	CurrentStreak int                        `json:"currentStreak"`
	LongestStreak int                        `json:"longestStreak"`
	TotalDays     int                        `json:"totalDays"`
	TotalRewards  float64                    `json:"totalRewards"`
	UserRank      int                        `json:"userRank"`
	Today         *RecordResponse            `json:"today,omitempty"`
	History       []RecordResponse           `json:"history"`
	Leaderboard   []LeaderboardEntryResponse `json:"leaderboard"`
}
