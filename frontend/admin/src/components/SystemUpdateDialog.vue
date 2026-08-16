<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2, Copy, ExternalLink, Info, RefreshCw } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { adminAPI } from '@/api/admin'
import { renderReleaseNotes } from '@/utils/releaseNotes'
import { notifyError } from '@/utils/notify'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t } = useI18n()

interface UpdateCheckResult {
  current_version: string
  latest_version: string
  has_update: boolean
  release_url?: string
  release_name?: string
  release_notes?: string
  published_at?: string | null
  source?: string
}

const checkLoading = ref(false)
const checkResult = ref<UpdateCheckResult | null>(null)
const checkError = ref('')
const copied = ref(false)

// 本项目只发布容器镜像，升级路径固定为拉新镜像重建容器。
const dockerUpgradeCommand = 'docker compose pull && docker compose up -d'

const renderedReleaseNotes = computed(() => {
  const raw = checkResult.value?.release_notes
  if (!raw) return ''
  return renderReleaseNotes(raw)
})

function formatPublishedAt(raw: string | null | undefined) {
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleString()
}

function extractErrorMessage(err: any): string {
  return err?.response?.data?.msg || err?.message || ''
}

async function loadCheck() {
  checkLoading.value = true
  checkError.value = ''
  checkResult.value = null
  try {
    const res = await adminAPI.checkSystemUpdate()
    const payload = res.data?.data as UpdateCheckResult | undefined
    if (payload) {
      checkResult.value = payload
    } else {
      checkError.value = t('admin.updateCheck.failedGeneric')
    }
  } catch (err: any) {
    const status = err?.response?.status
    const code = err?.response?.data?.status_code
    const msg = extractErrorMessage(err)
    if (status === 429 || code === 429) {
      checkError.value = t('admin.updateCheck.rateLimited')
    } else if (msg) {
      checkError.value = t('admin.updateCheck.failed', { message: msg })
    } else {
      checkError.value = t('admin.updateCheck.failedGeneric')
    }
  } finally {
    checkLoading.value = false
  }
}

async function copyDockerCommand() {
  try {
    await navigator.clipboard.writeText(dockerUpgradeCommand)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    notifyError(t('admin.systemUpdate.copyFailed'))
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      copied.value = false
      void loadCheck()
    }
  },
)
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ t('admin.updateCheck.title') }}</DialogTitle>
      </DialogHeader>

      <div class="space-y-3 text-sm">
        <div v-if="checkLoading" class="flex items-center gap-2 py-4 text-muted-foreground">
          <RefreshCw class="h-4 w-4 animate-spin" />
          <span>{{ t('admin.updateCheck.checking') }}</span>
        </div>

        <div v-else-if="checkError" class="flex items-start gap-2 text-destructive">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ checkError }}</span>
        </div>

        <template v-else-if="checkResult">
          <div
            class="flex items-start gap-2 rounded-md border px-3 py-2"
            :class="checkResult.has_update ? 'border-amber-500/40 bg-amber-500/5 text-amber-600 dark:text-amber-400' : 'border-emerald-500/40 bg-emerald-500/5 text-emerald-600 dark:text-emerald-400'"
          >
            <component :is="checkResult.has_update ? AlertCircle : CheckCircle2" class="mt-0.5 h-4 w-4 shrink-0" />
            <span v-if="checkResult.has_update">
              {{ t('admin.updateCheck.hasUpdate', { version: checkResult.latest_version || t('admin.updateCheck.unknownVersion') }) }}
            </span>
            <span v-else>
              {{ t('admin.updateCheck.latest', { version: checkResult.current_version || t('admin.updateCheck.unknownVersion') }) }}
            </span>
          </div>

          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-muted-foreground">
            <dt>{{ t('admin.updateCheck.currentLabel') }}</dt>
            <dd class="text-foreground">{{ checkResult.current_version || t('admin.updateCheck.unknownVersion') }}</dd>
            <dt>{{ t('admin.updateCheck.latestLabel') }}</dt>
            <dd class="text-foreground">{{ checkResult.latest_version || t('admin.updateCheck.unknownVersion') }}</dd>
            <template v-if="checkResult.published_at">
              <dt>{{ t('admin.updateCheck.publishedLabel') }}</dt>
              <dd class="text-foreground">{{ formatPublishedAt(checkResult.published_at) }}</dd>
            </template>
          </dl>

          <div
            v-if="checkResult.release_notes"
            class="release-notes max-h-40 overflow-y-auto rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground"
            v-html="renderedReleaseNotes"
          ></div>

          <!-- 升级方式固定为重建容器 -->
          <div v-if="checkResult.has_update" class="space-y-2 rounded-md border bg-muted/40 px-3 py-2.5">
            <div class="flex items-start gap-2">
              <Info class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <span class="text-xs text-foreground">{{ t('admin.systemUpdate.dockerHint') }}</span>
            </div>
            <div class="flex items-center gap-2">
              <code class="flex-1 overflow-x-auto whitespace-nowrap rounded bg-background px-2 py-1.5 text-xs">
                {{ dockerUpgradeCommand }}
              </code>
              <Button size="icon-sm" variant="outline" :title="t('admin.systemUpdate.copy')" @click="copyDockerCommand">
                <CheckCircle2 v-if="copied" class="h-3.5 w-3.5 text-emerald-500" />
                <Copy v-else class="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        </template>
      </div>

      <DialogFooter class="gap-2 sm:gap-2">
        <a
          v-if="checkResult?.release_url"
          :href="checkResult.release_url"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex h-9 items-center justify-center gap-1.5 rounded-md border px-3 text-sm font-medium hover:bg-accent"
        >
          <ExternalLink class="h-4 w-4" />
          {{ t('admin.updateCheck.viewRelease') }}
        </a>

        <Button variant="outline" size="sm" @click="emit('update:open', false)">
          {{ t('admin.updateCheck.close') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.release-notes :deep(h1),
.release-notes :deep(h2),
.release-notes :deep(h3),
.release-notes :deep(h4) {
  color: hsl(var(--foreground));
  font-weight: 600;
  margin: 0.6em 0 0.3em;
  line-height: 1.3;
}
.release-notes :deep(h1) { font-size: 1rem; }
.release-notes :deep(h2) { font-size: 0.95rem; }
.release-notes :deep(h3) { font-size: 0.85rem; }
.release-notes :deep(h4) { font-size: 0.8rem; }
.release-notes :deep(p) { margin: 0.4em 0; }
.release-notes :deep(ul),
.release-notes :deep(ol) {
  padding-left: 1.25rem;
  margin: 0.4em 0;
}
.release-notes :deep(ul) { list-style: disc; }
.release-notes :deep(ol) { list-style: decimal; }
.release-notes :deep(li) { margin: 0.15em 0; }
.release-notes :deep(strong) { color: hsl(var(--foreground)); font-weight: 600; }
.release-notes :deep(em) { font-style: italic; }
.release-notes :deep(a) {
  color: hsl(var(--primary));
  text-decoration: underline;
  text-underline-offset: 2px;
}
.release-notes :deep(code) {
  background: hsl(var(--muted));
  padding: 0.1em 0.35em;
  border-radius: 0.25rem;
  font-size: 0.9em;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.release-notes :deep(pre) {
  background: hsl(var(--muted));
  padding: 0.6em 0.8em;
  border-radius: 0.375rem;
  overflow-x: auto;
  margin: 0.5em 0;
}
.release-notes :deep(pre code) {
  background: transparent;
  padding: 0;
}
.release-notes :deep(blockquote) {
  border-left: 3px solid hsl(var(--border));
  padding-left: 0.75rem;
  margin: 0.5em 0;
  color: hsl(var(--muted-foreground));
}
.release-notes :deep(hr) {
  border: 0;
  border-top: 1px solid hsl(var(--border));
  margin: 0.8em 0;
}
.release-notes :deep(img) { max-width: 100%; height: auto; }
.release-notes :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.5em 0;
}
.release-notes :deep(th),
.release-notes :deep(td) {
  border: 1px solid hsl(var(--border));
  padding: 0.3em 0.5em;
  text-align: left;
}
</style>
