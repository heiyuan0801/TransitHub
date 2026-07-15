import type { CheckinEmbedStatus, CheckinRecord } from '@/modules/checkin/types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`
let sessionToken: string | null = null

interface EmbedSessionRequest {
  embedToken: string
  sub2apiToken: string
  srcHost: string
  srcUrl: string
  userId: string
}

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {}),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('embed.checkin.errors.network')
  }
  const payload = await response.json().catch(() => ({})) as T & { message?: string }
  if (!response.ok) throw new Error(payload.message ?? 'embed.checkin.errors.request')
  return payload
}

export const createCheckinSession = async (request: EmbedSessionRequest): Promise<void> => {
  const response = await requestJson<{ sessionToken: string }>('/embed/checkin/session', {
    method: 'POST',
    body: JSON.stringify(request),
  })
  sessionToken = response.sessionToken
}

export const getCheckinStatus = (): Promise<CheckinEmbedStatus> => requestJson('/embed/checkin/status')
export const claimCheckin = (): Promise<CheckinRecord> => requestJson('/embed/checkin', { method: 'POST' })
