export type CheckinRewardStatus = 'pending' | 'fulfilled' | 'retryable_failed' | 'failed'

export interface CheckinMilestone {
  days: number
  bonusAmount: number
}

export interface CheckinConfig {
  embedToken: string
  sub2apiSourceOrigin: string
  enabled: boolean
  dailyMin: number
  dailyMax: number
  dailyUserRewardCap: number
  timezone: string
  milestones: CheckinMilestone[]
  createdAt: string
  updatedAt: string
}

export interface CheckinConfigRequest {
  enabled: boolean
  dailyMin: number
  dailyMax: number
  dailyUserRewardCap: number
  timezone: string
  milestones: CheckinMilestone[]
}

export interface CheckinRecord {
  id: string
  email?: string
  maskedEmail?: string
  checkinDate: string
  streakDays: number
  baseReward: number
  milestoneReward: number
  totalReward: number
  rewardStatus: CheckinRewardStatus
  createdAt: string
}

export interface CheckinLeaderboardEntry {
  rank: number
  email?: string
  maskedEmail?: string
  totalDays: number
  currentStreak: number
  longestStreak: number
  totalRewards: number
  lastCheckinDate: string
  isCurrentUser?: boolean
}

export interface CheckinAdminOverview {
  config: CheckinConfig
  records: CheckinRecord[]
  leaderboard: CheckinLeaderboardEntry[]
  todayCount: number
  todayRewards: number
  totalUsers: number
  totalCheckins: number
  totalRewards: number
  recordsTotal: number
}

export type CheckinLeaderboardPeriod = 'today' | '7d' | '30d' | 'all'

export interface CheckinRecordsQuery {
  page: number
  pageSize: number
  dateFrom?: string
  dateTo?: string
  user?: string
}

export interface CheckinRecordsPage {
  items: CheckinRecord[]
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export interface CheckinLeaderboardResponse {
  period: CheckinLeaderboardPeriod
  entries: CheckinLeaderboardEntry[]
}

export interface CheckinEmbedStatus {
  enabled: boolean
  dailyMin: number
  dailyMax: number
  timezone: string
  milestones: CheckinMilestone[]
  checkedToday: boolean
  currentStreak: number
  longestStreak: number
  totalDays: number
  totalRewards: number
  userRank: number
  today?: CheckinRecord
  history: CheckinRecord[]
  leaderboard: CheckinLeaderboardEntry[]
}
