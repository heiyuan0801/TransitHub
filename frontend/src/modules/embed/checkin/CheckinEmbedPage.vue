<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  CalendarCheck2,
  Check,
  Flame,
  Gift,
  Loader2,
  RefreshCw,
} from 'lucide-vue-next'
import type { CheckinEmbedStatus } from '@/modules/checkin/types'
import { claimCheckin, createCheckinSession, getCheckinStatus } from './api'

const route = useRoute()
const { t, locale } = useI18n()
const pageState = ref<'loading' | 'ready' | 'error'>('loading')
const status = ref<CheckinEmbedStatus | null>(null)
const errorKey = ref('')
const actionErrorKey = ref('')
const claiming = ref(false)
const refreshing = ref(false)

const queryString = (value: unknown): string => Array.isArray(value)
  ? (typeof value[0] === 'string' ? value[0] : '')
  : (typeof value === 'string' ? value : '')

const applyTheme = (theme: string) => {
  if (theme === 'dark') document.documentElement.classList.add('dark')
  if (theme === 'light') document.documentElement.classList.remove('dark')
}

const stripTokenFromUrl = () => {
  const params = new URLSearchParams(window.location.search)
  params.delete('token')
  const query = params.toString()
  window.history.replaceState(window.history.state, '', query ? `${window.location.pathname}?${query}` : window.location.pathname)
}

const nextMilestone = computed(() => status.value?.milestones.find((item) => item.days > (status.value?.currentStreak ?? 0)) ?? null)
const canCheckIn = computed(() => Boolean(status.value?.enabled && !status.value?.checkedToday && !claiming.value))
const recentHistory = computed(() => status.value?.history.slice(0, 10) ?? [])
const checkinByDate = computed(() => new Map((status.value?.history ?? []).map((record) => [record.checkinDate, record])))

const dateKeyInTimezone = (date: Date, timezone: string): string => {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(date)
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]))
  return `${value.year}-${value.month}-${value.day}`
}

const calendarDays = computed(() => {
  if (!status.value) return []
  const todayKey = status.value.today?.checkinDate ?? dateKeyInTimezone(new Date(), status.value.timezone)
  const [year, month, day] = todayKey.split('-').map(Number)
  const today = new Date(Date.UTC(year, month - 1, day))
  return Array.from({ length: 30 }, (_, index) => {
    const date = new Date(today)
    date.setUTCDate(today.getUTCDate() - (29 - index))
    const key = date.toISOString().slice(0, 10)
    return { key, label: `${String(date.getUTCMonth() + 1).padStart(2, '0')}/${String(date.getUTCDate()).padStart(2, '0')}`, record: checkinByDate.value.get(key), isToday: key === todayKey }
  })
})

const loadStatus = async () => {
  refreshing.value = true
  actionErrorKey.value = ''
  try {
    status.value = await getCheckinStatus()
  } catch (error) {
    actionErrorKey.value = error instanceof Error ? error.message : 'embed.checkin.errors.request'
  } finally {
    refreshing.value = false
  }
}

const checkIn = async () => {
  if (!canCheckIn.value) return
  claiming.value = true
  actionErrorKey.value = ''
  try {
    await claimCheckin()
    await loadStatus()
  } catch (error) {
    const key = error instanceof Error ? error.message : 'embed.checkin.errors.request'
    if (key === 'embed.checkin.errors.alreadyChecked') await loadStatus()
    else actionErrorKey.value = key
  } finally {
    claiming.value = false
  }
}

onMounted(async () => {
  applyTheme(queryString(route.query.theme))
  locale.value = queryString(route.query.lang).toLowerCase().startsWith('en') ? 'en-US' : 'zh-CN'
  const embedToken = queryString(route.query.embed_token)
  const sub2apiToken = queryString(route.query.token)
  const srcHost = queryString(route.query.src_host)
  const srcUrl = queryString(route.query.src_url)
  const userId = queryString(route.query.user_id)
  if (sub2apiToken) stripTokenFromUrl()
  if (!embedToken || !sub2apiToken || !srcHost) {
    errorKey.value = 'embed.checkin.errors.missingParams'
    pageState.value = 'error'
    return
  }
  try {
    await createCheckinSession({ embedToken, sub2apiToken, srcHost, srcUrl, userId })
    status.value = await getCheckinStatus()
    pageState.value = 'ready'
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'embed.checkin.errors.request'
    pageState.value = 'error'
  }
})
</script>

<template>
  <main class="min-h-dvh bg-background px-4 py-5 text-foreground sm:px-6 sm:py-8">
    <div class="mx-auto flex w-full max-w-5xl flex-col gap-5">
      <header class="flex items-start justify-between gap-4 border-b border-border/70 pb-5">
        <div>
          <h1 class="flex items-center gap-2 text-2xl font-semibold tracking-normal sm:text-3xl"><CalendarCheck2 class="h-7 w-7 text-primary" />{{ t('embed.checkin.title') }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('embed.checkin.subtitle') }}</p>
        </div>
        <button v-if="pageState === 'ready'" type="button" class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-border bg-surface-elevated transition-colors hover:bg-surface focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-60" :disabled="refreshing" :title="t('embed.checkin.actions.refresh')" @click="loadStatus">
          <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': refreshing }" />
        </button>
      </header>

      <section v-if="pageState === 'loading'" class="flex min-h-72 items-center justify-center rounded-md border border-border/70 bg-surface-elevated text-sm text-muted-foreground">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" />{{ t('embed.checkin.loading') }}
      </section>

      <section v-else-if="pageState === 'error'" class="flex gap-3 rounded-md border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive" role="alert">
        <AlertCircle class="mt-0.5 h-5 w-5 shrink-0" />
        <div><h2 class="font-semibold">{{ t('embed.checkin.errors.title') }}</h2><p class="mt-1">{{ t(errorKey) }}</p></div>
      </section>

      <template v-else-if="status">
        <div v-if="actionErrorKey" class="flex gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" role="alert"><AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />{{ t(actionErrorKey) }}</div>

        <section class="grid overflow-hidden rounded-md border border-border/70 bg-surface-elevated lg:grid-cols-[1.2fr_0.8fr]">
          <div class="flex min-h-72 flex-col items-center justify-center px-6 py-8 text-center lg:border-r lg:border-border/70">
            <div class="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Check v-if="status.checkedToday" class="h-8 w-8" />
              <Gift v-else class="h-8 w-8" />
            </div>
            <h2 class="mt-5 text-xl font-semibold">{{ t(status.checkedToday ? 'embed.checkin.today.done' : 'embed.checkin.today.ready') }}</h2>
            <button type="button" class="mt-6 inline-flex h-11 min-w-40 items-center justify-center gap-2 rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-transform active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 focus:ring-offset-background disabled:cursor-not-allowed disabled:opacity-60" :disabled="!canCheckIn" @click="checkIn">
              <Loader2 v-if="claiming" class="h-4 w-4 animate-spin" />
              <Check v-else-if="status.checkedToday" class="h-4 w-4" />
              <CalendarCheck2 v-else class="h-4 w-4" />
              {{ t(status.checkedToday ? 'embed.checkin.actions.checked' : 'embed.checkin.actions.checkin') }}
            </button>
          </div>

          <div class="grid content-center gap-6 border-t border-border/70 p-6 lg:border-t-0">
            <div>
              <div class="flex items-center gap-2 text-sm text-muted-foreground"><Flame class="h-4 w-4 text-warning" />{{ t('embed.checkin.streak.label') }}</div>
              <p class="mt-2 text-4xl font-semibold tabular-nums">{{ status.currentStreak }}<span class="ml-1 text-base font-normal text-muted-foreground">{{ t('embed.checkin.streak.days') }}</span></p>
            </div>
            <div v-if="nextMilestone" class="border-t border-border/70 pt-5">
              <div class="flex items-center gap-2 text-sm font-medium"><Gift class="h-4 w-4 text-primary" />{{ t('embed.checkin.milestone.next') }}</div>
              <p class="mt-2 text-sm text-muted-foreground">{{ t('embed.checkin.milestone.description', { days: nextMilestone.days }) }}</p>
            </div>
            <p v-else class="border-t border-border/70 pt-5 text-sm text-muted-foreground">{{ t('embed.checkin.milestone.completed') }}</p>
          </div>
        </section>

        <section class="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-border/70 bg-border/70">
          <div class="bg-surface-elevated p-4 sm:p-5">
            <p class="text-xs text-muted-foreground">{{ t('embed.checkin.stats.totalDays') }}</p>
            <p class="mt-2 text-2xl font-semibold tabular-nums">{{ status.totalDays }}</p>
          </div>
          <div class="bg-surface-elevated p-4 sm:p-5">
            <p class="text-xs text-muted-foreground">{{ t('embed.checkin.stats.longestStreak') }}</p>
            <p class="mt-2 text-2xl font-semibold tabular-nums">{{ status.longestStreak }}</p>
          </div>
        </section>

        <section class="rounded-md border border-border/70 bg-surface-elevated p-5">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="font-semibold">{{ t('embed.checkin.calendar.title') }}</h2>
              <p class="mt-1 text-xs text-muted-foreground">{{ t('embed.checkin.calendar.description') }}</p>
            </div>
            <CalendarCheck2 class="h-5 w-5 shrink-0 text-primary" />
          </div>
          <div class="mt-4 grid grid-cols-6 gap-2 sm:grid-cols-10">
            <div
              v-for="item in calendarDays"
              :key="item.key"
              class="flex aspect-square min-w-0 flex-col items-center justify-center rounded-md border text-[10px] tabular-nums sm:text-xs"
              :class="item.record ? 'border-primary bg-primary text-primary-foreground' : item.isToday ? 'border-primary bg-primary/5 text-primary' : 'border-border/70 bg-surface text-muted-foreground'"
              :title="item.record ? t('embed.checkin.calendar.checkedTitle', { date: item.key }) : t('embed.checkin.calendar.emptyTitle', { date: item.key })"
            >
              <Check v-if="item.record" class="mb-0.5 h-3 w-3" />
              <span>{{ item.label }}</span>
            </div>
          </div>
        </section>

        <section class="rounded-md border border-border/70 bg-surface-elevated">
          <div class="flex items-center justify-between px-5 py-4"><h2 class="font-semibold">{{ t('embed.checkin.history.title') }}</h2><span class="text-xs text-muted-foreground">{{ t('embed.checkin.history.count', { count: recentHistory.length }) }}</span></div>
          <div v-if="!recentHistory.length" class="border-t border-border/70 p-6 text-center text-sm text-muted-foreground">{{ t('embed.checkin.history.empty') }}</div>
          <div v-else class="grid border-t border-border/70 sm:grid-cols-2">
            <div v-for="record in recentHistory" :key="record.id" class="border-b border-border/60 px-5 py-4 odd:sm:border-r">
              <div><p class="font-medium tabular-nums">{{ record.checkinDate }}</p><p class="mt-1 text-xs text-muted-foreground">{{ t('embed.checkin.history.streak', { count: record.streakDays }) }}</p></div>
            </div>
          </div>
        </section>
      </template>
    </div>
  </main>
</template>
