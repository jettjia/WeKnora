<template>
  <SettingDrawer
    v-model:visible="visible"
    :title="editing ? t('automation.drawer.edit') : t('automation.drawer.add')"
    :description="t('automation.drawer.desc')"
    icon="control-platform"
    width="560px"
    storage-key="setting-drawer:width:automation-editor"
    :confirm-loading="saving"
    @confirm="save"
  >
    <!-- 基础信息 -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('automation.drawer.sectionBasic') }}</h4>

      <div v-if="!editing" class="form-item">
        <label class="form-label required">{{ t('automation.drawer.name') }}</label>
        <t-input v-model="form.name" :status="nameError ? 'error' : undefined"
          placeholder="daily_report" autocomplete="off" spellcheck="false" />
        <p class="form-desc">{{ nameError || t('automation.drawer.nameHint') }}</p>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.title') }}</label>
        <t-input v-model="form.title" />
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.description') }}</label>
        <t-textarea v-model="form.description" :autosize="{ minRows: 2, maxRows: 4 }" />
      </div>

      <div class="form-item">
        <label class="form-label required">{{ t('automation.drawer.agent') }}</label>
        <t-select v-model="form.agent_id" filterable :placeholder="t('automation.drawer.selectAgent')">
          <t-option v-for="a in agents" :key="a.id" :value="a.id" :label="a.name" />
        </t-select>
        <p class="form-desc">{{ t('automation.drawer.agentHint') }}</p>
      </div>
    </section>

    <!-- 执行内容 -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('automation.drawer.sectionContent') }}</h4>
      <div class="form-item">
        <label class="form-label required">{{ t('automation.drawer.query') }}</label>
        <t-textarea v-model="form.query_template" :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder="{{date}} ..." autocomplete="off" spellcheck="false" />
        <p class="form-desc">{{ t('automation.drawer.queryHint') }}</p>
      </div>
    </section>

    <!-- 执行计划 -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('automation.drawer.sectionSchedule') }}</h4>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.schedulePresets.daily') }}</label>
        <div class="preset-row">
          <t-button v-for="(label, key) in presetButtons" :key="key" variant="outline" size="small"
            :class="{ active: presetKey === key }" @click="applyPreset(key)">
            {{ label }}
          </t-button>
        </div>
      </div>

      <div class="form-item">
        <label class="form-label required">{{ t('automation.drawer.schedule') }}</label>
        <t-input v-model="form.schedule_cron" placeholder="0 9 * * *" autocomplete="off" spellcheck="false"
          :status="cronError ? 'error' : undefined" @change="onCronChange" />
        <p class="form-desc">{{ cronError || t('automation.drawer.scheduleHint') }}</p>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.timezone') }}</label>
        <t-input v-model="form.schedule_tz" placeholder="Asia/Shanghai" autocomplete="off" spellcheck="false" />
      </div>

      <div v-if="nextRunsPreview.length" class="form-item">
        <label class="form-label">{{ t('automation.drawer.nextRuns') }}</label>
        <div class="next-runs">
          <div v-for="(r, i) in nextRunsPreview" :key="i" class="next-run-row">{{ r }}</div>
        </div>
      </div>

      <div class="form-item form-item--flat">
        <t-checkbox v-model="form.enabled">{{ t('automation.drawer.enabled') }}</t-checkbox>
      </div>
    </section>

    <!-- 高级 -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('automation.drawer.sectionAdvanced') }}</h4>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.overlap') }}</label>
        <t-select v-model="form.overlap_policy">
          <t-option value="skip" :label="t('automation.drawer.overlapSkip')" />
          <t-option value="queue" :label="t('automation.drawer.overlapQueue')" />
        </t-select>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('automation.drawer.timeout') }}</label>
        <t-input-number v-model="form.timeout_minutes" :min="1" :max="720" theme="normal" style="width: 160px" />
        <p class="form-desc">{{ t('automation.drawer.timeoutHint') }}</p>
      </div>
    </section>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { createAutomation, updateAutomation, type Automation, type AutomationInput } from './api'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'

const props = defineProps<{ agents: { id: string; name: string }[] }>()
const emit = defineEmits<{ (e: 'saved', a: Automation): void }>()

const { t } = useI18n()
const slugPattern = /^[a-z][a-z0-9_]{0,62}$/

const visible = ref(false)
const editing = ref<Automation | null>(null)
const saving = ref(false)
const nameError = ref('')
const cronError = ref('')

const form = ref<AutomationInput>({
  name: '', title: '', description: '', agent_id: '', query_template: '',
  schedule_cron: '0 9 * * *', schedule_tz: 'Asia/Shanghai',
  enabled: true, overlap_policy: 'skip', timeout_minutes: 15,
})

const presetKey = ref<string>('daily')
const presetButtons = computed(() => ({
  daily: t('automation.drawer.schedulePresets.daily'),
  weekly: t('automation.drawer.schedulePresets.weekly'),
  hourly: t('automation.drawer.schedulePresets.hourly'),
  custom: t('automation.drawer.schedulePresets.custom'),
}))

const PRESET_CRONS: Record<string, string> = {
  daily: '0 9 * * *',
  weekly: '0 9 * * 1',
  hourly: '0 * * * *',
}

function applyPreset(key: string) {
  presetKey.value = key
  if (PRESET_CRONS[key]) form.value.schedule_cron = PRESET_CRONS[key]
}

function onCronChange(v: unknown) {
  const value = String(v ?? '').trim()
  presetKey.value = PRESET_CRONS[value] ? value === '0 9 * * 1' ? 'weekly' : value === '0 * * * *' ? 'hourly' : 'daily' : 'custom'
  cronError.value = ''
}

// 客户端粗校验 cron 形状; 服务端保存时 robfig 解析兜底
const cronShape = /^(\S+\s+){4}\S+$/

// 未来 3 次执行预览: 仅在 cron 形状合法时做本地近似展示, 精确预览以列表页
// 后端返回的 next_runs 为准
const nextRunsPreview = computed(() => {
  const expr = (form.value.schedule_cron || '').trim()
  if (!cronShape.test(expr)) return []
  const minutes = expr.split(/\s+/)
  // 常见 presets 的快速预览 (daily/hourly/weekly); 其余交给列表页后端预览
  const tz = form.value.schedule_tz || 'Asia/Shanghai'
  const now = new Date()
  const fmt = (d: Date) => `${tz} ${d.toISOString().slice(0, 16).replace('T', ' ')} (UTC)`
  const out: string[] = []
  try {
    if (expr === '0 9 * * *') {
      for (let i = 1; i <= 3; i++) {
        const d = new Date(now); d.setHours(9, 0, 0, 0)
        if (d <= now) d.setDate(d.getDate() + i); else d.setDate(d.getDate() + i - 1)
        out.push(fmt(d))
      }
    } else if (expr === '0 * * * *') {
      for (let i = 1; i <= 3; i++) {
        const d = new Date(now); d.setMinutes(0, 0, 0); d.setHours(d.getHours() + i)
        out.push(fmt(d))
      }
    } else if (expr === '0 9 * * 1') {
      for (let i = 1; i <= 7 && out.length < 3; i++) {
        const d = new Date(now); d.setDate(d.getDate() + i)
        if (d.getDay() === 1) { d.setHours(9, 0, 0, 0); out.push(fmt(d)) }
      }
    }
  } catch { /* 预览失败静默 */ }
  return out
})

function openCreate() {
  editing.value = null
  nameError.value = ''
  cronError.value = ''
  form.value = {
    name: '', title: '', description: '', agent_id: props.agents[0]?.id || '',
    query_template: '', schedule_cron: '0 9 * * *', schedule_tz: 'Asia/Shanghai',
    enabled: true, overlap_policy: 'skip', timeout_minutes: 15,
  }
  presetKey.value = 'daily'
  visible.value = true
}

function openEdit(a: Automation) {
  editing.value = a
  nameError.value = ''
  cronError.value = ''
  form.value = {
    name: a.name, title: a.title, description: a.description,
    agent_id: a.agent_id, query_template: a.query_template,
    schedule_cron: a.schedule_cron, schedule_tz: a.schedule_tz,
    enabled: a.enabled, overlap_policy: a.overlap_policy, timeout_minutes: a.timeout_minutes,
  }
  presetKey.value = PRESET_CRONS[a.schedule_cron] ? (a.schedule_cron === '0 9 * * 1' ? 'weekly' : a.schedule_cron === '0 * * * *' ? 'hourly' : 'daily') : 'custom'
  visible.value = true
}

async function save() {
  if (!editing.value && !slugPattern.test(form.value.name || '')) {
    nameError.value = t('automation.drawer.nameError')
    return
  }
  const expr = (form.value.schedule_cron || '').trim()
  if (!cronShape.test(expr)) {
    cronError.value = t('automation.drawer.cronInvalid')
    return
  }
  saving.value = true
  try {
    let updated: Automation
    if (editing.value) {
      updated = await updateAutomation(editing.value.id, form.value)
    } else {
      updated = await createAutomation(form.value)
    }
    emit('saved', updated)
    visible.value = false
    MessagePlugin.success(t('automation.drawer.saved'))
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'save failed')
  } finally {
    saving.value = false
  }
}

defineExpose({ openCreate, openEdit })
</script>

<style scoped>
.form-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.form-item--flat {
  margin-top: 4px;
}
.form-label {
  display: block;
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 6px;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}
.form-label.required::before {
  content: '*';
  color: var(--td-error-color);
  margin-right: 4px;
  font-weight: 500;
  line-height: 1;
}
.form-desc {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);
}
.preset-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.preset-row .t-button.active {
  --td-button-outline-color: var(--td-brand-color);
  color: var(--td-brand-color);
  border-color: var(--td-brand-color);
}
.next-runs {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  padding: 8px 10px;
}
</style>
