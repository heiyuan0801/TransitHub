<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Clipboard, Code2, Eye, KeyRound, Loader2, Play, RefreshCw, Save } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  getConnectionHealthEmbedConfig,
  rotateConnectionHealthEmbedToken,
  testConnectionHealthEmbed,
  updateConnectionHealthEmbedConfig,
} from '../api/connectionHealth'
import type { ConnectionHealthEmbedConfig, ConnectionHealthEmbedTestResult } from '../types/connectionHealth'

const { t } = useI18n()

const embedConfig = ref<ConnectionHealthEmbedConfig | null>(null)
const refreshInterval = ref(30)
const customApiKey = ref('')
const saving = ref(false)
const rotating = ref(false)
const testing = ref(false)
const loading = ref(true)
const copied = ref<'url' | 'iframe' | ''>('')
const message = ref('')
const error = ref('')
const testResult = ref<ConnectionHealthEmbedTestResult | null>(null)

const errorMessage = (err: unknown, fallback: string) => {
  const key = err instanceof Error ? err.message : fallback
  // API 失败消息使用稳定的 i18n key；翻译不存在时保留原文，方便诊断未知错误。
  if (key.startsWith('admin.') || key.startsWith('auth.')) return t(key)
  return key
}

const normalizeConfig = (config: ConnectionHealthEmbedConfig): ConnectionHealthEmbedConfig => ({
  ...config,
  customCheckEnabled: Boolean(config.customCheckEnabled),
  customBaseUrl: config.customBaseUrl ?? '',
  customModel: config.customModel ?? '',
  customProviderFamily: config.customProviderFamily || 'openai',
  customApiKeyConfigured: Boolean(config.customApiKeyConfigured),
})

const loadConfig = async () => {
  loading.value = true
  error.value = ''
  try {
    const config = normalizeConfig(await getConnectionHealthEmbedConfig())
    embedConfig.value = config
    refreshInterval.value = config.refreshIntervalSeconds
  } catch (err) {
    error.value = errorMessage(err, t('admin.connectionHealth.embed.saveFailed'))
  } finally {
    loading.value = false
  }
}

const embedUrl = computed(() => {
  const token = embedConfig.value?.embedToken
  if (!token) return ''
  const path = `/embed/connection-health?embed_token=${encodeURIComponent(token)}`
  return typeof window === 'undefined' ? path : new URL(path, window.location.origin).toString()
})

const iframeCode = computed(() => embedUrl.value
  ? `<iframe src="${embedUrl.value}" title="在线模型检测" style="width:100%;min-height:520px;border:0" loading="lazy"></iframe>`
  : '')

const saveConfig = async (): Promise<boolean> => {
  if (!embedConfig.value || saving.value) return false
  saving.value = true
  message.value = ''
  error.value = ''
  try {
    embedConfig.value = normalizeConfig(await updateConnectionHealthEmbedConfig({
      enabled: embedConfig.value.enabled,
      // 空值代表不限制 frame-ancestors，允许在任意站点嵌入。
      allowedOrigin: '',
      refreshIntervalSeconds: refreshInterval.value,
      customCheckEnabled: embedConfig.value.customCheckEnabled,
      customBaseUrl: embedConfig.value.customBaseUrl,
      customApiKey: customApiKey.value,
      customModel: embedConfig.value.customModel,
      customProviderFamily: embedConfig.value.customProviderFamily,
    }))
    refreshInterval.value = embedConfig.value.refreshIntervalSeconds
    customApiKey.value = ''
    message.value = t('admin.connectionHealth.embed.saved')
    return true
  } catch (err) {
    error.value = errorMessage(err, t('admin.connectionHealth.embed.saveFailed'))
    return false
  } finally {
    saving.value = false
  }
}

const testConfig = async () => {
  if (testing.value || saving.value) return
  testing.value = true
  message.value = ''
  error.value = ''
  testResult.value = null
  try {
    // 测试前先保存当前表单，避免管理员修改 URL/模型后直接测试到旧配置。
    if (!await saveConfig()) return
    testResult.value = await testConnectionHealthEmbed()
    message.value = testResult.value.healthy
      ? t('admin.connectionHealth.embed.testPassed')
      : t('admin.connectionHealth.embed.testFailed')
  } catch (err) {
    error.value = errorMessage(err, t('admin.connectionHealth.embed.testFailed'))
  } finally {
    testing.value = false
  }
}

const rotateToken = async () => {
  if (rotating.value) return
  rotating.value = true
  message.value = ''
  error.value = ''
  try {
    embedConfig.value = normalizeConfig(await rotateConnectionHealthEmbedToken())
    message.value = t('admin.connectionHealth.embed.generated')
  } catch (err) {
    error.value = errorMessage(err, t('admin.connectionHealth.embed.saveFailed'))
  } finally {
    rotating.value = false
  }
}

const copyValue = async (value: string, kind: 'url' | 'iframe') => {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    copied.value = kind
    window.setTimeout(() => { if (copied.value === kind) copied.value = '' }, 1800)
  } catch {
    error.value = t('admin.connectionHealth.embed.copyFailed')
  }
}

onMounted(() => { void loadConfig() })
</script>

<template>
  <div class="space-y-5">
    <header class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div class="min-w-0">
        <div class="flex items-center gap-2 text-sm font-medium text-primary">
          <Code2 class="h-4 w-4" />
          {{ t('admin.menu.sub2apiFeatures') }}
        </div>
        <h1 class="mt-1 text-xl font-semibold text-foreground">{{ t('admin.connectionHealth.embed.standaloneTitle') }}</h1>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">{{ t('admin.connectionHealth.embed.standaloneDescription') }}</p>
      </div>
      <Button variant="secondary" size="sm" :disabled="loading" @click="loadConfig">
        <RefreshCw class="h-4 w-4" :class="loading ? 'animate-spin' : ''" />
        {{ t('admin.connectionHealth.refresh') }}
      </Button>
    </header>

    <div v-if="loading" class="rounded-lg border border-border/60 bg-card p-6 text-sm text-muted-foreground">
      <Loader2 class="mr-2 inline h-4 w-4 animate-spin" />{{ t('admin.connectionHealth.embed.loading') }}
    </div>

    <template v-else-if="embedConfig">
      <section class="rounded-lg border border-border/60 bg-card p-5 shadow-sm">
        <div class="flex items-start gap-3">
          <KeyRound class="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <div>
            <h2 class="font-semibold text-foreground">{{ t('admin.connectionHealth.embed.customTitle') }}</h2>
            <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t('admin.connectionHealth.embed.customDescription') }}</p>
          </div>
        </div>

        <div class="mt-5 grid gap-4 md:grid-cols-2">
          <label class="space-y-1.5 text-sm md:col-span-2">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.customBaseUrl') }}</span>
            <input v-model="embedConfig.customBaseUrl" type="url" :placeholder="t('admin.connectionHealth.embed.customBaseUrlPlaceholder')" class="h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20">
          </label>
          <label class="space-y-1.5 text-sm">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.customApiKey') }}</span>
            <input v-model="customApiKey" type="password" autocomplete="new-password" :placeholder="embedConfig.customApiKeyConfigured ? t('admin.connectionHealth.embed.customApiKeyPlaceholder') : t('admin.connectionHealth.embed.customApiKey')" class="h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20">
            <span class="text-xs text-muted-foreground">{{ embedConfig.customApiKeyConfigured ? t('admin.connectionHealth.embed.customConfigured') : t('admin.connectionHealth.embed.customNotConfigured') }}</span>
          </label>
          <label class="space-y-1.5 text-sm">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.customModel') }}</span>
            <input v-model="embedConfig.customModel" type="text" :placeholder="t('admin.connectionHealth.embed.customModelPlaceholder')" class="h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20">
          </label>
          <label class="space-y-1.5 text-sm">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.customProvider') }}</span>
            <select v-model="embedConfig.customProviderFamily" class="h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20">
              <option value="openai">OpenAI</option>
              <option value="gemini">Gemini</option>
              <option value="anthropic">Anthropic</option>
              <option value="custom">{{ t('admin.connectionHealth.providerLabels.custom') }}</option>
            </select>
          </label>
          <label class="flex items-center gap-3 self-end pb-2 text-sm">
            <input v-model="embedConfig.customCheckEnabled" type="checkbox" class="h-4 w-4 accent-primary">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.customEnabled') }}</span>
          </label>
        </div>
      </section>

      <section class="rounded-lg border border-border/60 bg-card p-5 shadow-sm">
        <div class="flex items-start gap-3">
          <Eye class="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <div>
            <h2 class="font-semibold text-foreground">{{ t('admin.connectionHealth.embed.standaloneSettings') }}</h2>
            <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t('admin.connectionHealth.embed.openAccessHint') }}</p>
          </div>
        </div>
        <div class="mt-5 grid gap-4 sm:grid-cols-2">
          <label class="flex items-center gap-3 text-sm">
            <input v-model="embedConfig.enabled" type="checkbox" class="h-4 w-4 accent-primary">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.enabled') }}</span>
          </label>
          <label class="space-y-1.5 text-sm">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.interval') }}</span>
            <input v-model.number="refreshInterval" type="number" min="10" max="3600" step="1" class="h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20">
            <span class="text-xs text-muted-foreground">{{ t('admin.connectionHealth.embed.intervalHint') }}</span>
          </label>
        </div>
        <div class="mt-5 flex flex-wrap items-center gap-2">
          <Button :disabled="saving" @click="saveConfig">
            <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
            <Save v-else class="h-4 w-4" />
            {{ saving ? t('admin.connectionHealth.embed.saving') : t('admin.connectionHealth.embed.save') }}
          </Button>
          <Button variant="secondary" :disabled="saving || testing" @click="testConfig">
            <Loader2 v-if="testing" class="h-4 w-4 animate-spin" />
            <Play v-else class="h-4 w-4" />
            {{ testing ? t('admin.connectionHealth.embed.testing') : t('admin.connectionHealth.embed.test') }}
          </Button>
          <Button variant="secondary" :disabled="rotating" @click="rotateToken">
            <Loader2 v-if="rotating" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.connectionHealth.embed.rotate') }}
          </Button>
        </div>
        <div v-if="testResult" class="mt-4 rounded-lg border border-border/60 bg-surface/60 p-4 text-sm">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="font-medium text-foreground">{{ t('admin.connectionHealth.embed.testResult') }} · {{ testResult.modelName }}</span>
            <span :class="testResult.healthy ? 'text-emerald-600 dark:text-emerald-300' : 'text-destructive'">
              {{ testResult.healthy ? t('admin.connectionHealth.embed.testPassed') : t('admin.connectionHealth.embed.testFailed') }}
            </span>
          </div>
          <div class="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground">
            <span v-if="testResult.firstByteLatencyMs != null">{{ t('admin.connectionHealth.embed.firstByte') }} {{ testResult.firstByteLatencyMs }} ms</span>
            <span>{{ t('admin.connectionHealth.embed.totalLatency') }} {{ testResult.latencyMs }} ms</span>
            <span v-if="testResult.qualityReason">{{ testResult.qualityReason }}</span>
          </div>
          <p v-if="!testResult.healthy && testResult.errorDetail" class="mt-2 break-all text-xs text-destructive">{{ testResult.errorDetail }}</p>
        </div>
      </section>

      <section class="rounded-lg border border-border/60 bg-card p-5 shadow-sm">
        <div class="flex items-start gap-3">
          <Code2 class="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <div>
            <h2 class="font-semibold text-foreground">{{ t('admin.connectionHealth.embed.generate') }}</h2>
            <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t('admin.connectionHealth.embed.generateHint') }}</p>
          </div>
        </div>
        <div class="mt-5 space-y-4">
          <div>
            <div class="mb-1.5 flex items-center justify-between gap-3 text-xs font-medium text-muted-foreground"><span>{{ t('admin.connectionHealth.embed.generatedUrl') }}</span><button type="button" class="text-primary hover:underline" @click="copyValue(embedUrl, 'url')">{{ copied === 'url' ? t('admin.connectionHealth.embed.copied') : t('admin.connectionHealth.embed.copyUrl') }}</button></div>
            <code class="block overflow-x-auto rounded-lg bg-surface px-3 py-3 text-xs text-foreground">{{ embedUrl }}</code>
          </div>
          <div>
            <div class="mb-1.5 flex items-center justify-between gap-3 text-xs font-medium text-muted-foreground"><span>{{ t('admin.connectionHealth.embed.generatedCode') }}</span><button type="button" class="inline-flex items-center gap-1 text-primary hover:underline" @click="copyValue(iframeCode, 'iframe')"><Check v-if="copied === 'iframe'" class="h-3.5 w-3.5" /><Clipboard v-else class="h-3.5 w-3.5" />{{ copied === 'iframe' ? t('admin.connectionHealth.embed.copied') : t('admin.connectionHealth.embed.copy') }}</button></div>
            <code class="block overflow-x-auto rounded-lg bg-surface px-3 py-3 text-xs leading-5 text-foreground">{{ iframeCode }}</code>
          </div>
        </div>
      </section>
    </template>

    <p v-if="message" class="rounded-lg bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-300">{{ message }}</p>
    <p v-if="error" class="rounded-lg bg-destructive/10 px-4 py-3 text-sm text-destructive">{{ error }}</p>
  </div>
</template>
