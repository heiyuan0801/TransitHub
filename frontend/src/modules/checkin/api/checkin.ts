import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'
import type {
  CheckinAdminOverview,
  CheckinConfig,
  CheckinConfigRequest,
  CheckinLeaderboardPeriod,
  CheckinLeaderboardResponse,
  CheckinRecordsPage,
  CheckinRecordsQuery,
} from '../types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getAccessToken() ?? ''}`,
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('admin.checkin.errors.network')
  }
  const payload = await response.json().catch(() => ({})) as T & { message?: string }
  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.checkin.errors.request')
  }
  return payload
}

export const getCheckinOverview = (): Promise<CheckinAdminOverview> => requestJson('/checkin/overview')

export const getCheckinRecords = (query: CheckinRecordsQuery): Promise<CheckinRecordsPage> => {
  const params = new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize) })
  if (query.dateFrom) params.set('dateFrom', query.dateFrom)
  if (query.dateTo) params.set('dateTo', query.dateTo)
  if (query.user?.trim()) params.set('user', query.user.trim())
  return requestJson(`/checkin/records?${params.toString()}`)
}

export const getCheckinLeaderboard = (period: CheckinLeaderboardPeriod): Promise<CheckinLeaderboardResponse> => (
  requestJson(`/checkin/leaderboard?period=${encodeURIComponent(period)}`)
)

export const saveCheckinConfig = (request: CheckinConfigRequest): Promise<CheckinConfig> => (
  requestJson('/checkin/config', { method: 'PUT', body: JSON.stringify(request) })
)

export const rotateCheckinToken = (): Promise<CheckinConfig> => (
  requestJson('/checkin/config/rotate-token', { method: 'POST' })
)
