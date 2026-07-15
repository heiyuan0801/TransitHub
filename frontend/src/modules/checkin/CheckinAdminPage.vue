<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  CalendarCheck2,
  CalendarDays,
  Check,
  ChevronLeft,
  ChevronRight,
  Clipboard,
  Coins,
  Gift,
  Loader2,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Trash2,
  Trophy,
  Users,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import type {
  CheckinAdminOverview,
  CheckinConfigRequest,
  CheckinLeaderboardPeriod,
  CheckinRecordsPage,
} from './types'
import {
  getCheckinLeaderboard,
  getCheckinOverview,
  getCheckinRecords,
  rotateCheckinToken,
  saveCheckinConfig,
} from './api/checkin'

const { t, locale } = useI18n()
const overview = ref<CheckinAdminOverview | null>(null)
const loading = ref(true)
const saving = ref(false)
const rotating = ref(false)
const copied = ref(false)
const errorKey = ref('')
const saved = ref(false)
const recordsLoading = ref(false)
const leaderboardLoading = ref(false)
const leaderboardPeriod = ref<CheckinLeaderboardPeriod>('all')
const leaderboardPeriods: CheckinLeaderboardPeriod[] = ['today', '7d', '30d', 'all']
const recordsPage = ref<CheckinRecordsPage>({ items: [], page: 1, pageSize: 20, total: 0, totalPages: 0 })
const recordFilters = ref({ dateFrom: '', dateTo: '', user: '' })
const form = ref<CheckinConfigRequest>({
  enabled: true,
  dailyMin: 0.1,
  dailyMax: 1,
  dailyUserRewardCap: 10,
  timezone: 'Asia/Shanghai',
  milestones: [],
})

const embedUrl = computed(() => {
  const token = overview.value?.config.embedToken
  if (!token) return ''
  const url = new URL('/embed/checkin', window.location.origin)
  url.searchParams.set('embed_token', token)
  return url.toString()
})

const money = (value: number): string => new Intl.NumberFormat(locale.value, {
  minimumFractionDigits: 2,
  maximumFractionDigits: 6,
}).format(value)

const applyOverview = (value: CheckinAdminOverview) => {
  overview.value = value
  form.value = {
    enabled: value.config.enabled,
    dailyMin: value.config.dailyMin,
    dailyMax: value.config.dailyMax,
    dailyUserRewardCap: value.config.dailyUserRewardCap,
    timezone: value.config.timezone,
    milestones: value.config.milestones.map((item) => ({ ...item })),
  }
  recordsPage.value = { items: value.records, page: 1, pageSize: 20, total: value.recordsTotal, totalPages: Math.ceil(value.recordsTotal / 20) }
}

const load = async () => {
  loading.value = true
  errorKey.value = ''
  try {
    applyOverview(await getCheckinOverview())
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.checkin.errors.unknown'
  } finally {
    loading.value = false
  }
}

const addMilestone = () => {
  const last = form.value.milestones.at(-1)
  form.value.milestones.push({ days: last ? last.days + 7 : 7, bonusAmount: 1 })
}

const removeMilestone = (index: number) => {
  form.value.milestones.splice(index, 1)
}

const validate = (): boolean => {
  if (!Number.isFinite(form.value.dailyMin) || form.value.dailyMin <= 0 || form.value.dailyMax < form.value.dailyMin || form.value.dailyMax > form.value.dailyUserRewardCap) return false
  const days = new Set<number>()
  return form.value.milestones.every((item) => {
    if (!Number.isInteger(item.days) || item.days < 2 || item.bonusAmount <= 0 || days.has(item.days)) return false
    days.add(item.days)
    return true
  })
}

const loadRecords = async (page = 1) => {
  recordsLoading.value = true
  errorKey.value = ''
  try {
    recordsPage.value = await getCheckinRecords({ page, pageSize: recordsPage.value.pageSize, ...recordFilters.value })
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.checkin.errors.unknown'
  } finally {
    recordsLoading.value = false
  }
}

const resetRecordFilters = () => {
  recordFilters.value = { dateFrom: '', dateTo: '', user: '' }
  void loadRecords(1)
}

const selectLeaderboardPeriod = async (period: CheckinLeaderboardPeriod) => {
  leaderboardPeriod.value = period
  leaderboardLoading.value = true
  errorKey.value = ''
  try {
    const result = await getCheckinLeaderboard(period)
    if (overview.value) overview.value.leaderboard = result.entries
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.checkin.errors.unknown'
  } finally {
    leaderboardLoading.value = false
  }
}

const save = async () => {
  saved.value = false
  errorKey.value = ''
  if (!validate()) {
    errorKey.value = 'admin.checkin.errors.validation'
    return
  }
  saving.value = true
  try {
    const config = await saveCheckinConfig(form.value)
    if (overview.value) overview.value.config = config
    saved.value = true
    window.setTimeout(() => { saved.value = false }, 1800)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.checkin.errors.unknown'
  } finally {
    saving.value = false
  }
}

const copyEmbedUrl = async () => {
  try {
    await navigator.clipboard.writeText(embedUrl.value)
    copied.value = true
    window.setTimeout(() => { copied.value = false }, 1600)
  } catch {
    errorKey.value = 'admin.checkin.errors.copy'
  }
}

const rotateToken = async () => {
  if (!window.confirm(t('admin.checkin.embed.confirmRotate'))) return
  rotating.value = true
  errorKey.value = ''
  try {
    const config = await rotateCheckinToken()
    if (overview.value) overview.value.config = config
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.checkin.errors.unknown'
  } finally {
    rotating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto flex w-full max-w-6xl flex-col gap-5 p-4 sm:p-6">
    <header class="flex flex-col gap-3 border-b border-border/70 pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold tracking-normal text-foreground">{{ t('admin.checkin.title') }}</h1>
        <p class="mt-1 max-w-2xl text-sm text-muted-foreground">{{ t('admin.checkin.subtitle') }}</p>
      </div>
      <Button variant="secondary" class="shrink-0" :disabled="loading" @click="load">
        <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': loading }" />
        {{ t('admin.checkin.actions.refresh') }}
      </Button>
    </header>

    <div v-if="errorKey" class="flex gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <div v-if="loading && !overview" class="flex min-h-64 items-center justify-center text-sm text-muted-foreground">
      <Loader2 class="mr-2 h-5 w-5 animate-spin" />
      {{ t('admin.checkin.loading') }}
    </div>

    <template v-else-if="overview">
      <section class="grid gap-px overflow-hidden rounded-md border border-border/70 bg-border/70 sm:grid-cols-2 xl:grid-cols-5">
        <div class="bg-surface-elevated p-5">
          <div class="flex items-center gap-2 text-sm text-muted-foreground"><Users class="h-4 w-4" />{{ t('admin.checkin.metrics.todayUsers') }}</div>
          <p class="mt-2 text-3xl font-semibold tabular-nums">{{ overview.todayCount }}</p>
        </div>
        <div class="bg-surface-elevated p-5">
          <div class="flex items-center gap-2 text-sm text-muted-foreground"><Coins class="h-4 w-4" />{{ t('admin.checkin.metrics.todayRewards') }}</div>
          <p class="mt-2 text-3xl font-semibold tabular-nums">{{ money(overview.todayRewards) }}</p>
        </div>
        <div class="bg-surface-elevated p-5">
          <div class="flex items-center gap-2 text-sm text-muted-foreground"><Users class="h-4 w-4" />{{ t('admin.checkin.metrics.totalUsers') }}</div>
          <p class="mt-2 text-3xl font-semibold tabular-nums">{{ overview.totalUsers }}</p>
        </div>
        <div class="bg-surface-elevated p-5">
          <div class="flex items-center gap-2 text-sm text-muted-foreground"><CalendarDays class="h-4 w-4" />{{ t('admin.checkin.metrics.totalCheckins') }}</div>
          <p class="mt-2 text-3xl font-semibold tabular-nums">{{ overview.totalCheckins }}</p>
        </div>
        <div class="bg-surface-elevated p-5 sm:col-span-2 xl:col-span-1">
          <div class="flex items-center gap-2 text-sm text-muted-foreground"><Coins class="h-4 w-4" />{{ t('admin.checkin.metrics.totalRewards') }}</div>
          <p class="mt-2 text-3xl font-semibold tabular-nums">{{ money(overview.totalRewards) }}</p>
        </div>
      </section>

      <div class="grid gap-5 lg:grid-cols-[minmax(0,1.25fr)_minmax(20rem,0.75fr)]">
        <section class="rounded-md border border-border/70 bg-surface-elevated p-5">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="flex items-center gap-2 text-base font-semibold"><CalendarCheck2 class="h-4 w-4 text-primary" />{{ t('admin.checkin.config.title') }}</h2>
              <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.checkin.config.description') }}</p>
            </div>
            <label class="flex shrink-0 items-center gap-2 text-sm font-medium">
              <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-border text-primary focus:ring-primary" />
              {{ t('admin.checkin.fields.enabled') }}
            </label>
          </div>

          <div class="mt-5 grid gap-4 sm:grid-cols-3">
            <label class="grid gap-2 text-sm font-medium">
              {{ t('admin.checkin.fields.dailyMin') }}
              <input v-model.number="form.dailyMin" type="number" min="0.01" step="0.01" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/30" />
            </label>
            <label class="grid gap-2 text-sm font-medium">
              {{ t('admin.checkin.fields.dailyMax') }}
              <input v-model.number="form.dailyMax" type="number" min="0.01" step="0.01" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/30" />
            </label>
            <label class="grid gap-2 text-sm font-medium">
              {{ t('admin.checkin.fields.dailyUserRewardCap') }}
              <input v-model.number="form.dailyUserRewardCap" type="number" :min="form.dailyMax" step="0.01" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/30" />
            </label>
            <p class="text-xs text-muted-foreground sm:col-span-3">{{ t('admin.checkin.config.capDescription') }}</p>
            <label class="grid gap-2 text-sm font-medium sm:col-span-3">
              {{ t('admin.checkin.fields.timezone') }}
              <select v-model="form.timezone" class="h-10 rounded-md border border-border bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary">
                <option value="Asia/Shanghai">Asia/Shanghai</option>
                <option value="Asia/Hong_Kong">Asia/Hong_Kong</option>
                <option value="Asia/Tokyo">Asia/Tokyo</option>
                <option value="UTC">UTC</option>
              </select>
            </label>
          </div>

          <div class="mt-6 border-t border-border/70 pt-5">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h3 class="flex items-center gap-2 text-sm font-semibold"><Gift class="h-4 w-4 text-primary" />{{ t('admin.checkin.milestones.title') }}</h3>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.checkin.milestones.description') }}</p>
              </div>
              <Button variant="secondary" size="sm" @click="addMilestone"><Plus class="h-4 w-4" />{{ t('admin.checkin.actions.addMilestone') }}</Button>
            </div>

            <div v-if="form.milestones.length" class="mt-4 grid gap-3">
              <div v-for="(item, index) in form.milestones" :key="index" class="grid grid-cols-[1fr_1fr_2.5rem] items-end gap-3">
                <label class="grid gap-2 text-xs font-medium text-muted-foreground">
                  {{ t('admin.checkin.fields.days') }}
                  <input v-model.number="item.days" type="number" min="2" step="1" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm text-foreground outline-none focus:border-primary focus:ring-2 focus:ring-primary/30" />
                </label>
                <label class="grid gap-2 text-xs font-medium text-muted-foreground">
                  {{ t('admin.checkin.fields.bonus') }}
                  <input v-model.number="item.bonusAmount" type="number" min="0.01" step="0.01" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm text-foreground outline-none focus:border-primary focus:ring-2 focus:ring-primary/30" />
                </label>
                <Button variant="ghost" size="sm" class="h-10 w-10 px-0" :title="t('admin.checkin.actions.removeMilestone')" @click="removeMilestone(index)"><Trash2 class="h-4 w-4" /></Button>
              </div>
            </div>
            <p v-else class="mt-4 rounded-md border border-dashed border-border p-4 text-sm text-muted-foreground">{{ t('admin.checkin.milestones.empty') }}</p>
          </div>

          <div class="mt-6 flex justify-end">
            <Button :disabled="saving" @click="save">
              <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
              <Check v-else-if="saved" class="h-4 w-4" />
              <Save v-else class="h-4 w-4" />
              {{ t(saved ? 'admin.checkin.actions.saved' : 'admin.checkin.actions.save') }}
            </Button>
          </div>
        </section>

        <section class="rounded-md border border-border/70 bg-surface-elevated p-5">
          <h2 class="text-base font-semibold">{{ t('admin.checkin.embed.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.checkin.embed.description') }}</p>
          <dl class="mt-5 grid gap-4 text-sm">
            <div>
              <dt class="font-medium">{{ t('admin.checkin.embed.source') }}</dt>
              <dd class="mt-1 break-all text-muted-foreground">{{ overview.config.sub2apiSourceOrigin }}</dd>
            </div>
            <div>
              <dt class="font-medium">{{ t('admin.checkin.embed.url') }}</dt>
              <dd class="mt-2 rounded-md border border-border bg-background p-3 font-mono text-xs break-all">{{ embedUrl }}</dd>
            </div>
          </dl>
          <div class="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
            <Button variant="secondary" @click="copyEmbedUrl"><Check v-if="copied" class="h-4 w-4" /><Clipboard v-else class="h-4 w-4" />{{ t(copied ? 'admin.checkin.actions.copied' : 'admin.checkin.actions.copy') }}</Button>
            <Button variant="secondary" :disabled="rotating" @click="rotateToken"><Loader2 v-if="rotating" class="h-4 w-4 animate-spin" /><RotateCcw v-else class="h-4 w-4" />{{ t('admin.checkin.actions.rotate') }}</Button>
          </div>
        </section>
      </div>

      <section class="overflow-hidden rounded-md border border-border/70 bg-surface-elevated">
        <div class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="flex items-center gap-2 text-base font-semibold"><Trophy class="h-4 w-4 text-warning" />{{ t('admin.checkin.leaderboard.title') }}</h2>
            <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.checkin.leaderboard.description') }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-1 rounded-md border border-border bg-surface p-1" role="tablist">
            <button
              v-for="period in leaderboardPeriods"
              :key="period"
              type="button"
              role="tab"
              :aria-selected="leaderboardPeriod === period"
              class="h-8 rounded-sm px-3 text-xs font-medium transition-colors"
              :class="leaderboardPeriod === period ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground'"
              :disabled="leaderboardLoading"
              @click="selectLeaderboardPeriod(period)"
            >
              {{ t(`admin.checkin.leaderboard.period.${period}`) }}
            </button>
          </div>
        </div>
        <div v-if="!overview.leaderboard.length" class="border-t border-border/70 p-6 text-center text-sm text-muted-foreground">{{ t('admin.checkin.leaderboard.empty') }}</div>
        <div v-else class="overflow-x-auto border-t border-border/70">
          <table class="w-full min-w-[54rem] text-left text-sm">
            <thead class="bg-surface text-xs text-muted-foreground">
              <tr><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.rank') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.user') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.totalDays') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.currentStreak') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.longestStreak') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.totalRewards') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.leaderboard.lastCheckin') }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="entry in overview.leaderboard" :key="`${entry.rank}-${entry.maskedEmail}`" class="border-t border-border/60">
                <td class="px-5 py-3 font-semibold tabular-nums" :class="entry.rank <= 3 ? 'text-primary' : 'text-muted-foreground'">{{ entry.rank }}</td>
                <td class="px-5 py-3 font-medium">{{ entry.email || entry.maskedEmail || t('admin.checkin.records.anonymous') }}</td>
                <td class="px-5 py-3 tabular-nums">{{ entry.totalDays }}</td>
                <td class="px-5 py-3 tabular-nums">{{ entry.currentStreak }}</td>
                <td class="px-5 py-3 tabular-nums">{{ entry.longestStreak }}</td>
                <td class="px-5 py-3 tabular-nums">{{ money(entry.totalRewards) }}</td>
                <td class="px-5 py-3 tabular-nums text-muted-foreground">{{ entry.lastCheckinDate }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="overflow-hidden rounded-md border border-border/70 bg-surface-elevated">
        <div class="flex flex-col gap-4 px-5 py-4">
          <div class="flex items-center justify-between">
            <h2 class="text-base font-semibold">{{ t('admin.checkin.records.title') }}</h2>
            <span class="text-xs text-muted-foreground">{{ t('admin.checkin.records.count', { count: recordsPage.total }) }}</span>
          </div>
          <form class="grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(12rem,1fr)_10rem_10rem_auto_auto]" @submit.prevent="loadRecords(1)">
            <label class="grid gap-1.5 text-xs font-medium text-muted-foreground">
              {{ t('admin.checkin.records.userSearch') }}
              <input v-model="recordFilters.user" type="search" :placeholder="t('admin.checkin.records.userPlaceholder')" class="h-10 rounded-md border border-border bg-background px-3 text-sm text-foreground outline-none focus:ring-2 focus:ring-primary" />
            </label>
            <label class="grid gap-1.5 text-xs font-medium text-muted-foreground">
              {{ t('admin.checkin.records.dateFrom') }}
              <input v-model="recordFilters.dateFrom" type="date" class="h-10 rounded-md border border-border bg-background px-3 text-sm text-foreground outline-none focus:ring-2 focus:ring-primary" />
            </label>
            <label class="grid gap-1.5 text-xs font-medium text-muted-foreground">
              {{ t('admin.checkin.records.dateTo') }}
              <input v-model="recordFilters.dateTo" type="date" class="h-10 rounded-md border border-border bg-background px-3 text-sm text-foreground outline-none focus:ring-2 focus:ring-primary" />
            </label>
            <Button type="submit" class="self-end" :disabled="recordsLoading"><Loader2 v-if="recordsLoading" class="h-4 w-4 animate-spin" /><Search v-else class="h-4 w-4" />{{ t('admin.checkin.actions.search') }}</Button>
            <Button type="button" variant="secondary" class="self-end" :disabled="recordsLoading" @click="resetRecordFilters">{{ t('admin.checkin.actions.reset') }}</Button>
          </form>
        </div>
        <div v-if="!recordsPage.items.length" class="border-t border-border/70 p-6 text-center text-sm text-muted-foreground">{{ t('admin.checkin.records.empty') }}</div>
        <div v-else class="overflow-x-auto border-t border-border/70">
          <table class="w-full min-w-[44rem] text-left text-sm">
            <thead class="bg-surface text-xs text-muted-foreground"><tr><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.user') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.date') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.streak') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.reward') }}</th><th class="px-5 py-3 font-medium">{{ t('admin.checkin.fields.status') }}</th></tr></thead>
            <tbody>
              <tr v-for="record in recordsPage.items" :key="record.id" class="border-t border-border/60">
                <td class="px-5 py-3">{{ record.email || record.maskedEmail || t('admin.checkin.records.anonymous') }}</td>
                <td class="px-5 py-3 tabular-nums text-muted-foreground">{{ record.checkinDate }}</td>
                <td class="px-5 py-3">{{ t('admin.checkin.records.streakDays', { count: record.streakDays }) }}</td>
                <td class="px-5 py-3 font-medium tabular-nums">{{ money(record.totalReward) }}</td>
                <td class="px-5 py-3"><span class="rounded-sm border px-2 py-1 text-xs" :class="record.rewardStatus === 'fulfilled' ? 'border-emerald-400/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'border-destructive/30 bg-destructive/10 text-destructive'">{{ t(`admin.checkin.status.${record.rewardStatus}`) }}</span></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="recordsPage.totalPages > 1" class="flex items-center justify-between border-t border-border/70 px-5 py-3">
          <span class="text-xs text-muted-foreground">{{ t('admin.checkin.records.page', { page: recordsPage.page, total: recordsPage.totalPages }) }}</span>
          <div class="flex gap-2">
            <Button variant="secondary" size="sm" class="h-9 w-9 px-0" :title="t('admin.checkin.actions.previous')" :disabled="recordsLoading || recordsPage.page <= 1" @click="loadRecords(recordsPage.page - 1)"><ChevronLeft class="h-4 w-4" /></Button>
            <Button variant="secondary" size="sm" class="h-9 w-9 px-0" :title="t('admin.checkin.actions.next')" :disabled="recordsLoading || recordsPage.page >= recordsPage.totalPages" @click="loadRecords(recordsPage.page + 1)"><ChevronRight class="h-4 w-4" /></Button>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
