<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, AlertTriangle, Brain, CheckCircle2, Clock3, RefreshCw, XCircle } from 'lucide-vue-next'

type Model = {
  connectionId: string
  groupName: string
  modelName: string
  state: string
  firstByteLatencyMs?: number | null
  latencyMs?: number | null
  lastProbeAt?: string | null
  errorKey?: string
  qualityStatus?: 'not_degraded' | 'degraded' | 'unknown' | string
  qualityScore?: number
  qualityReason?: string
  previewHtml?: string
}
type LogEntry = {
  id: string
  modelName: string
  result: string
  healthy: boolean
  firstByteLatencyMs?: number | null
  latencyMs: number
  errorKey?: string
  errorDetail?: string
  qualityStatus?: string
  qualityScore?: number
  qualityReason?: string
  previewHtml?: string
  probedAt: string
}
type Response = { generatedAt: string; refreshIntervalSeconds: number; models: Model[]; logs?: LogEntry[] }

const route = useRoute()
const loading = ref(true)
const error = ref('')
const data = ref<Response>({ generatedAt: '', refreshIntervalSeconds: 30, models: [], logs: [] })
let timer: number | undefined
const apiBase = String(import.meta.env.VITE_API_BASE_URL ?? '/api').replace(/\/$/, '')

const token = computed(() => {
  const raw = route.query.embed_token
  return Array.isArray(raw) ? String(raw[0] ?? '') : String(raw ?? '')
})
const orderedModels = computed(() => data.value.models)
const logs = computed(() => data.value.logs ?? [])
const previewLogs = computed(() => logs.value.filter((entry) => entry.healthy && Boolean(entry.previewHtml)))
const healthyCount = computed(() => orderedModels.value.filter((model) => model.state === 'healthy').length)
const notDegradedCount = computed(() => orderedModels.value.filter((model) => model.qualityStatus === 'not_degraded').length)
const degradedCount = computed(() => orderedModels.value.filter((model) => model.qualityStatus === 'degraded').length)
const stateLabel = (state: string) => ({ healthy: '正常', degraded: '降级', suspended: '暂停', observing: '观察中', recovering: '恢复中', disabled: '已禁用' }[state] ?? state)
const stateClass = (state: string) => state === 'healthy' ? 'text-emerald-700' : state === 'degraded' || state === 'observing' || state === 'recovering' ? 'text-amber-700' : 'text-rose-700'
const errorReason = (errorKey?: string) => ({
  network_fluctuation: '网络波动或请求超时（单次请求上限 2 分钟）',
  rate_limited: '上游触发限流，请稍后重试',
  server_error: '上游服务异常',
  auth: 'API Key 无效、过期或没有权限',
  model_not_found: '模型不存在或当前渠道不支持该模型',
  invalid_response: '上游返回格式无法解析',
  unsupported: '当前检测方式暂不支持该模型',
  not_probed: '等待后端首次检测',
}[errorKey ?? ''] ?? '')
const qualityLabel = (status?: string, state?: string) => status === 'not_degraded' ? '不降智' : status === 'degraded' ? '降智' : state && state !== 'healthy' ? stateLabel(state) : '待判断'
const qualityClass = (status?: string, state?: string) => status === 'not_degraded' ? 'bg-emerald-700 text-white' : status === 'degraded' ? 'bg-rose-700 text-white' : state && state !== 'healthy' ? 'bg-amber-700 text-white' : 'bg-slate-700 text-slate-200'
const formatDuration = (milliseconds?: number | null) => {
  if (milliseconds == null) return '-'
  if (milliseconds >= 60_000) return `${Math.floor(milliseconds / 60_000)}m ${Math.floor((milliseconds % 60_000) / 1000)}s`
  if (milliseconds >= 1000) return `${(milliseconds / 1000).toFixed(1)}s`
  return `${milliseconds}ms`
}
const formatTime = (value?: string | null) => value ? new Date(value).toLocaleString() : '暂无检测时间'
const healthIcon = (state: string) => state === 'healthy' ? CheckCircle2 : XCircle
const logLabel = (entry: LogEntry) => entry.healthy ? '成功' : '失败'
const logClass = (entry: LogEntry) => entry.healthy ? 'text-emerald-700' : 'text-rose-700'

const load = async () => {
  if (!token.value) { error.value = '缺少内嵌令牌'; loading.value = false; return }
  loading.value = true
  try {
    const response = await fetch(`${apiBase}/embed/connection-health?embed_token=${encodeURIComponent(token.value)}`, { headers: { Accept: 'application/json' } })
    // 某些代理在 204/网关错误时会返回空 body，不能直接调用 response.json()。
    const raw = await response.text()
    let payload: Partial<Response> & { message?: string } = {}
    if (raw.trim()) {
      try { payload = JSON.parse(raw) as Partial<Response> & { message?: string } } catch {
        throw new Error('模型检测返回格式无效')
      }
    }
    if (!response.ok) throw new Error(payload.message || (response.status === 401 ? '内嵌令牌无效或看板未启用' : '模型检测暂不可用'))
    data.value = {
      generatedAt: String(payload.generatedAt ?? ''),
      refreshIntervalSeconds: Number(payload.refreshIntervalSeconds ?? 30),
      models: Array.isArray(payload.models) ? payload.models as Model[] : [],
      logs: Array.isArray(payload.logs) ? payload.logs as LogEntry[] : [],
    }
    error.value = ''
    if (timer) window.clearInterval(timer)
    timer = window.setInterval(() => void load(), Math.max(10, data.value.refreshIntervalSeconds || 30) * 1000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '模型检测暂不可用'
  } finally { loading.value = false }
}

onMounted(() => { void load() })
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <main class="min-h-dvh bg-[#eef2ea] px-4 py-6 text-[#213b35] sm:px-8">
    <div class="mx-auto max-w-7xl">
      <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.24em] text-[#2d7a5d]"><Activity class="h-4 w-4" /> Online Model Check</p>
          <h1 class="text-2xl font-semibold tracking-tight sm:text-3xl">模型在线检测</h1>
          <p class="mt-1 text-sm text-[#668078]">按检测顺序展示分组和模型状态，悬停卡片查看完整时间与判定原因。</p>
        </div>
        <button type="button" class="inline-flex items-center gap-2 rounded-full border border-[#c7d6c9] bg-white/70 px-4 py-2 text-sm text-[#31584b] shadow-sm transition hover:bg-white" :disabled="loading" @click="load"><RefreshCw class="h-4 w-4" :class="loading ? 'animate-spin' : ''" />刷新</button>
      </header>

      <p v-if="error" class="mb-5 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>

      <section class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <div class="rounded-2xl border border-[#d9e4d7] bg-white/75 p-4 shadow-sm"><p class="text-xs text-[#789088]">检测模型</p><p class="mt-2 text-2xl font-semibold">{{ orderedModels.length }}</p></div>
        <div class="rounded-2xl border border-[#d9e4d7] bg-white/75 p-4 shadow-sm"><p class="text-xs text-[#789088]">当前正常</p><p class="mt-2 text-2xl font-semibold text-[#27815d]">{{ healthyCount }}</p></div>
        <div class="rounded-2xl border border-[#d9e4d7] bg-white/75 p-4 shadow-sm"><p class="text-xs text-[#789088]">不降智</p><p class="mt-2 text-2xl font-semibold text-[#27815d]">{{ notDegradedCount }}</p></div>
        <div class="rounded-2xl border border-[#d9e4d7] bg-white/75 p-4 shadow-sm"><p class="text-xs text-[#789088]">降智</p><p class="mt-2 text-2xl font-semibold text-[#b23a4c]">{{ degradedCount }}</p></div>
        <div class="rounded-2xl border border-[#d9e4d7] bg-white/75 p-4 shadow-sm"><p class="text-xs text-[#789088]">自动刷新</p><p class="mt-2 text-2xl font-semibold">{{ data.refreshIntervalSeconds }}<span class="ml-1 text-sm font-normal text-[#789088]">秒</span></p></div>
      </section>

      <section v-if="loading && orderedModels.length === 0" class="rounded-3xl border border-[#d9e4d7] bg-white/75 p-12 text-center text-sm text-[#789088] shadow-sm">正在读取后端检测结果...</section>
      <section v-else-if="orderedModels.length === 0" class="rounded-3xl border border-[#d9e4d7] bg-white/75 p-12 text-center text-sm text-[#789088] shadow-sm">暂无后端检测结果，请先在后台配置并启用检测。</section>
      <section v-else class="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
        <article v-for="(model, index) in orderedModels" :key="`${model.connectionId}:${model.modelName}`" class="group relative overflow-hidden rounded-[1.75rem] border border-[#d9e4d7] bg-white shadow-[0_18px_40px_rgba(61,93,72,0.12)] transition duration-300 hover:-translate-y-1 hover:shadow-[0_24px_55px_rgba(61,93,72,0.2)]" :title="`${model.modelName} · ${formatTime(model.lastProbeAt)} · ${model.qualityReason || errorReason(model.errorKey) || '暂无判定原因'}`">
          <div class="pointer-events-none absolute -right-12 top-5 z-10 w-40 rotate-45 py-2 text-center text-xs font-bold tracking-wide shadow-sm" :class="qualityClass(model.qualityStatus, model.state)"><Brain class="mr-1 inline h-3.5 w-3.5" />{{ qualityLabel(model.qualityStatus, model.state) }}</div>
          <div class="relative h-32 overflow-hidden bg-[#e8f0e4] p-5">
            <div class="absolute -right-8 -top-16 h-36 w-36 rotate-45 bg-[#2d7a5d]/10" />
            <p class="text-[10px] font-semibold uppercase tracking-[0.22em] text-[#7d998b]">CHECK {{ String(index + 1).padStart(2, '0') }}</p>
            <div class="mt-4 flex items-center gap-3"><span class="flex h-9 w-9 items-center justify-center rounded-xl bg-[#d2e5d5] text-[#2d7a5d]"><Activity class="h-5 w-5" /></span><div class="min-w-0"><h2 class="truncate pr-20 text-lg font-semibold text-[#213b35]">{{ model.modelName }}</h2><p class="truncate text-xs text-[#6d877b]">{{ model.groupName || '未命名分组' }}</p></div></div>
          </div>
          <div v-if="model.previewHtml" class="border-b border-[#e5ebe3] bg-[#dfe8dc] p-3">
            <div class="overflow-hidden rounded-2xl border border-[#cbd9c9] bg-white shadow-inner">
              <iframe
                class="h-48 w-full bg-white sm:h-56"
                :srcdoc="model.previewHtml"
                title="模型生成 HTML 预览"
                sandbox="allow-scripts"
                referrerpolicy="no-referrer"
              />
            </div>
            <p class="mt-2 text-center text-[11px] text-[#789088]">模型生成结果 · 已在隔离预览窗口中运行</p>
          </div>
          <div class="space-y-4 p-5">
            <div class="flex items-center justify-between gap-3"><span class="flex items-center gap-2 text-sm font-medium" :class="stateClass(model.state)"><component :is="healthIcon(model.state)" class="h-4 w-4" />{{ stateLabel(model.state) }}</span><span v-if="model.latencyMs != null" class="flex items-center gap-1 text-right text-xs text-[#789088]"><Clock3 class="h-4 w-4 shrink-0" /><span v-if="model.firstByteLatencyMs != null">首字 {{ formatDuration(model.firstByteLatencyMs) }} · 总耗时 {{ formatDuration(model.latencyMs) }}</span><span v-else>{{ formatDuration(model.latencyMs) }}</span></span></div>
            <div class="grid grid-cols-2 gap-2 rounded-xl bg-[#f2f6ef] p-3 text-xs"><div><p class="text-[#8aa095]">检测时间</p><p class="mt-1 truncate font-medium text-[#31584b]">{{ formatTime(model.lastProbeAt) }}</p></div><div><p class="text-[#8aa095]">质量分</p><p class="mt-1 font-medium text-[#31584b]">{{ model.qualityStatus === 'unknown' || !model.qualityStatus ? '-' : `${model.qualityScore ?? 0}/100` }}</p></div></div>
            <div class="flex items-start gap-2 text-xs leading-5" :class="model.errorKey && model.errorKey !== 'ok' ? 'text-[#a34a27]' : 'text-[#6d877b]'"><AlertTriangle v-if="model.errorKey && model.errorKey !== 'ok'" class="mt-0.5 h-4 w-4 shrink-0 text-[#b23a4c]" /><CheckCircle2 v-else class="mt-0.5 h-4 w-4 shrink-0 text-[#27815d]" /><span>{{ model.qualityReason || errorReason(model.errorKey) || '尚未进行质量判定' }}</span></div>
          </div>
          <div class="pointer-events-none absolute inset-x-4 bottom-4 translate-y-2 rounded-xl bg-[#213b35] px-3 py-2 text-xs leading-5 text-white opacity-0 shadow-xl transition duration-200 group-hover:translate-y-0 group-hover:opacity-100">{{ model.qualityReason || '暂无判定原因' }} · {{ formatTime(model.lastProbeAt) }}</div>
        </article>
      </section>

      <section v-if="previewLogs.length" class="mt-7">
        <div class="mb-3 flex flex-wrap items-end justify-between gap-2"><div><h2 class="font-semibold text-[#31584b]">历史生成结果</h2><p class="mt-1 text-xs text-[#8aa095]">展示最近成功检测返回的 HTML，均在隔离窗口中运行。</p></div><span class="text-xs text-[#8aa095]">{{ previewLogs.length }} 个结果</span></div>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <article v-for="entry in previewLogs" :key="`preview:${entry.id}`" class="overflow-hidden rounded-3xl border border-[#d9e4d7] bg-white shadow-sm">
            <div class="bg-[#dfe8dc] p-2"><div class="overflow-hidden rounded-2xl border border-[#cbd9c9] bg-white"><iframe class="h-44 w-full bg-white sm:h-48" :srcdoc="entry.previewHtml" :title="`${entry.modelName} HTML 生成结果`" sandbox="allow-scripts" referrerpolicy="no-referrer" /></div></div>
            <div class="flex items-center justify-between gap-2 px-4 py-3 text-xs"><div class="min-w-0"><p class="truncate font-medium text-[#31584b]">{{ entry.modelName }} · <span :class="logClass(entry)">{{ entry.qualityStatus === 'not_degraded' ? '不降智' : entry.qualityStatus === 'degraded' ? '降智' : logLabel(entry) }}</span></p><p class="mt-1 truncate text-[#8aa095]">{{ formatTime(entry.probedAt) }}</p></div><span class="shrink-0 text-[#789088]">{{ formatDuration(entry.latencyMs) }}</span></div>
          </article>
        </div>
      </section>

      <section v-if="logs.length" class="mt-7 overflow-hidden rounded-3xl border border-[#d9e4d7] bg-white/80 shadow-sm">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-[#e4ece1] px-5 py-4">
          <div><h2 class="font-semibold text-[#31584b]">检测日志</h2><p class="mt-1 text-xs text-[#8aa095]">日志由后端按配置间隔生成，刷新页面不会重复请求上游。</p></div>
          <span class="text-xs text-[#8aa095]">最近 {{ logs.length }} 条</span>
        </div>
        <div class="divide-y divide-[#edf1eb]">
          <div v-for="entry in logs" :key="entry.id" class="flex flex-wrap items-center justify-between gap-3 px-5 py-3 text-sm">
            <div class="flex min-w-0 items-center gap-3"><span class="h-2.5 w-2.5 shrink-0 rounded-full" :class="entry.healthy ? 'bg-emerald-600' : 'bg-rose-600'" /><div class="min-w-0"><p class="truncate font-medium text-[#31584b]">{{ entry.modelName }} <span class="ml-1 text-xs" :class="logClass(entry)">{{ logLabel(entry) }}</span></p><p class="truncate text-xs text-[#8aa095]">{{ entry.qualityReason || errorReason(entry.errorKey) || entry.errorDetail || '暂无附加信息' }}</p></div></div>
            <div class="flex shrink-0 items-center gap-3 text-right text-xs text-[#789088]"><span v-if="entry.latencyMs">{{ entry.firstByteLatencyMs ? `首字 ${formatDuration(entry.firstByteLatencyMs)} · ` : '' }}总耗时 {{ formatDuration(entry.latencyMs) }}</span><time :datetime="entry.probedAt">{{ formatTime(entry.probedAt) }}</time></div>
          </div>
        </div>
      </section>
      <p class="mt-5 text-right text-xs text-[#8aa095]">每 {{ data.refreshIntervalSeconds }} 秒自动刷新结果 · 看板更新时间：{{ data.generatedAt ? new Date(data.generatedAt).toLocaleString() : '-' }}</p>
    </div>
  </main>
</template>
