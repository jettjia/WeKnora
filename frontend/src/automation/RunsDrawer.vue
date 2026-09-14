<template>
  <t-drawer v-model:visible="visible" :header="runsHeader" size="640px" :footer="false">
    <t-table
      v-if="runs.length"
      row-key="id"
      :data="runs"
      :columns="columns"
      size="small"
      max-height="calc(100vh - 160px)"
    >
      <template #status="{ row }">
        <div class="feature-badge status-badge" :class="statusClass(row.status)">
          <t-icon :name="statusIcon(row.status)" size="14px" />
          <span>{{ statusLabel(row.status) }}</span>
        </div>
      </template>
      <template #trigger="{ row }">{{ t(`automation.run.trigger.${row.trigger_type}`) }}</template>
      <template #started="{ row }">{{ fmtTime(row.started_at) }}</template>
      <template #duration="{ row }">{{ fmtDuration(row.duration_ms) }}</template>
      <template #output="{ row }">
        <t-tooltip :content="row.output_summary || row.error || '-'" placement="top" :show-arrow="false">
          <div class="output-cell">{{ row.output_summary || row.error || '-' }}</div>
        </t-tooltip>
      </template>
      <template #session="{ row }">
        <t-button v-if="row.session_id" variant="text" size="small" theme="primary" @click="openSession(row)">
          {{ t('automation.run.viewSession') }}
        </t-button>
      </template>
    </t-table>
    <div v-else class="empty">{{ t('automation.run.empty') }}</div>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { listAutomationRuns, type Automation, type AutomationRun } from './api'

const props = defineProps<{ automation: Automation | null }>()
const visible = defineModel<boolean>('visible', { default: false })

const { t } = useI18n()
const router = useRouter()
const runs = ref<AutomationRun[]>([])

const runsHeader = computed(() =>
  `${t('automation.run.history')} · ${props.automation?.title || props.automation?.name || ''}`)

watch(
  () => [visible.value, props.automation?.id],
  () => {
    if (visible.value && props.automation) {
      listAutomationRuns(props.automation.id, 100)
        .then(resp => { runs.value = resp.runs || [] })
        .catch(() => { runs.value = [] })
    }
  },
  { immediate: true },
)

const columns = computed(() => [
  { colKey: 'status', title: t('automation.run.colStatus'), width: 110 },
  { colKey: 'trigger', title: t('automation.run.colTrigger'), width: 80 },
  { colKey: 'started', title: t('automation.run.colStarted'), width: 140 },
  { colKey: 'duration', title: t('automation.run.colDuration'), width: 90 },
  { colKey: 'output', title: t('automation.run.colOutput'), minWidth: 200 },
  { colKey: 'session', title: '', width: 90 },
])

function statusClass(s: string): string {
  if (s === 'success') return 'success'
  if (s === 'running' || s === 'pending') return 'running'
  if (s === 'skipped') return 'idle'
  return 'error'
}
function statusIcon(s: string): string {
  if (s === 'success') return 'check-circle'
  if (s === 'running' || s === 'pending') return 'loading'
  if (s === 'skipped') return 'pause-circle'
  if (s === 'timeout') return 'time'
  return 'error-circle'
}
function statusLabel(s: string): string {
  return t(`automation.run.status.${s}`)
}
function fmtTime(ts?: string | null): string {
  return ts ? String(ts).slice(0, 19).replace('T', ' ') : '-'
}
function fmtDuration(ms: number): string {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${s % 60}s`
}
function openSession(row: AutomationRun) {
  router.push(`/platform/chat/${row.session_id}`)
}
</script>

<style scoped>
.feature-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: 22px;
  border-radius: 5px;
  padding: 0 6px;
  font-size: 11px;
  font-weight: 500;
}
.feature-badge.success {
  background: rgba(7, 192, 95, 0.08);
  color: var(--td-brand-color-active);
}
.feature-badge.running {
  background: rgba(0, 82, 217, 0.08);
  color: var(--td-brand-color);
}
.feature-badge.error {
  background: rgba(227, 77, 89, 0.08);
  color: var(--td-error-color);
}
.feature-badge.idle {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-placeholder);
}
.output-cell {
  max-width: 260px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.empty {
  padding: 64px 0;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}
</style>
