<template>
  <div class="actions-tab">
    <div v-if="modelValue.length" class="card-grid">
      <div v-for="a in modelValue" :key="a.id" class="kb-style-card conn-card" @click="canManage && openEdit(a)">
        <div class="card-header">
          <span class="card-title" :title="a.name">
            <span class="card-title-text">{{ a.title || a.name }}</span>
            <span class="card-slug">{{ a.name }}</span>
          </span>
          <t-popup overlay-class-name="card-more-popup" trigger="click" destroy-on-close placement="bottom-right">
            <div class="more-wrap" @click.stop>
              <img class="more-icon" src="@/assets/img/more.png" alt="" />
            </div>
            <template #content>
              <div class="popup-menu" @click.stop>
                <div v-if="canManage" class="popup-menu-item" @click.stop="copyJSON(a)">
                  <t-icon class="menu-icon" name="file-copy" />
                  <span>{{ t('semantic.action.import.copyJson') }}</span>
                </div>
                <div v-if="canManage" class="popup-menu-item" @click.stop="openEdit(a)">
                  <t-icon class="menu-icon" name="edit" />
                  <span>{{ t('semantic.action.edit') }}</span>
                </div>
                <div v-if="canManage" class="popup-menu-item delete" @click.stop="confirmDelete(a)">
                  <t-icon class="menu-icon" name="delete" />
                  <span>{{ t('semantic.action.delete') }}</span>
                </div>
              </div>
            </template>
          </t-popup>
        </div>

        <div class="card-content">
          <div class="card-description">
            {{ a.description || a.object_types.join(' · ') }}
          </div>
        </div>

        <div class="card-bottom">
          <div class="bottom-left">
            <t-tooltip :content="badgeTooltip(a)" placement="top">
              <div class="feature-badge type-badge">
                <t-icon name="layers" size="14px" />
                <span class="badge-text">{{ a.object_types[0] || 'webhook' }}</span>
              </div>
            </t-tooltip>
            <div class="feature-badge status-badge" :class="a.status">
              <t-icon :name="a.status === 'active' ? 'check-circle' : 'error-circle'" size="14px" />
              <span class="badge-text">{{ a.status }}</span>
            </div>
          </div>
          <div class="bottom-right">
            <span class="card-time">{{ shortTime(a.updated_at) }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="empty-state">
      <img class="empty-img" src="@/assets/img/upload.svg" alt="" />
      <span class="empty-txt">{{ t('semantic.action.empty') }}</span>
      <t-button v-if="canManage" class="empty-state-btn" @click="openCreate">
        <template #icon><t-icon name="add" /></template>
        {{ t('semantic.action.add') }}
      </t-button>
    </div>

    <!-- 编辑器: 对齐知识库数据源编辑的 SettingDrawer 设计 -->
    <SettingDrawer
      v-model:visible="dialogVisible"
      :title="editing ? t('semantic.action.edit') : t('semantic.action.add')"
      :description="t('semantic.action.drawerDesc')"
      icon="play-circle"
      width="560px"
      storage-key="setting-drawer:width:semantic-action-editor"
      :confirm-loading="saving"
      @confirm="save"
    >
      <!--
        从 JSON 导入: 粘贴动作配置 JSON, 纯前端解析后填回表单。
        不自动提交; 用户检查后再点保存。跨环境 (开发/测试/生产) 迁移用。
      -->
      <section class="setting-drawer__section code-import">
        <button type="button" class="code-import__toggle" @click="importOpen = !importOpen">
          <t-icon :name="importOpen ? 'chevron-down' : 'chevron-right'" />
          <span>{{ t('semantic.action.import.toggle') }}</span>
        </button>
        <div v-if="importOpen" class="code-import__body">
          <p class="form-desc">{{ t('semantic.action.import.hint') }}</p>
          <p v-if="editing" class="form-desc code-import__warn">
            {{ t('semantic.action.import.editOverwriteHint') }}
          </p>
          <t-textarea
            v-model="importText"
            :autosize="{ minRows: 6, maxRows: 14 }"
            :placeholder="importPlaceholder"
            class="code-import__textarea"
          />
          <p v-if="importError" class="code-import__error">{{ importError }}</p>
          <div class="code-import__actions">
            <t-button theme="primary" variant="outline" @click="handleImport">
              {{ t('semantic.action.import.parse') }}
            </t-button>
          </div>
        </div>
      </section>

      <!-- 基础信息 -->
      <section class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ t('semantic.action.sectionBasic') }}</h4>

        <div v-if="!editing" class="form-item">
          <label class="form-label required">{{ t('semantic.action.name') }}</label>
          <t-input v-model="form.name" :status="nameError ? 'error' : undefined"
            placeholder="create_ticket" autocomplete="off" spellcheck="false" />
          <p class="form-desc">{{ nameError || t('semantic.action.nameHint') }}</p>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.title') }}</label>
          <t-input v-model="form.title" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.desc') }}</label>
          <t-textarea v-model="form.description" :autosize="{ minRows: 2, maxRows: 4 }"
            :placeholder="t('semantic.action.descHint')" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.objectTypes') }}</label>
          <t-select v-model="form.object_types" multiple filterable :placeholder="t('semantic.action.objectTypesHint')">
            <t-option v-for="m in availableModels" :key="m" :value="m" :label="m" />
          </t-select>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.allowedGroups') }}</label>
          <t-select v-model="form.allowed_groups" multiple filterable :placeholder="t('semantic.action.groupsHint')">
            <t-option v-for="g in groups" :key="g.name" :value="g.name" :label="g.title || g.name" />
          </t-select>
        </div>
      </section>

      <!-- Webhook 配置 -->
      <section class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ t('semantic.action.sectionWebhook') }}</h4>

        <div class="form-item">
          <label class="form-label required">{{ t('semantic.action.webhookUrl') }}</label>
          <t-input v-model="webhookUrl" placeholder="https://erp.internal/api/tickets" autocomplete="off" spellcheck="false" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.webhookMethod') }}</label>
          <t-select v-model="webhookMethod" style="width: 160px" :options="methodOptions" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.inputSchema') }}</label>
          <div v-if="form.input_schema.length" class="custom-headers-list">
            <div v-for="(f, i) in form.input_schema" :key="i" class="field-row">
              <t-input v-model="f.column" :placeholder="t('semantic.action.fieldColumnPlaceholder')" class="field-key" autocomplete="off" spellcheck="false" />
              <t-select v-model="f.type" class="field-type" :options="fieldTypeOptions" />
              <t-checkbox v-model="f.required" class="field-required">{{ t('semantic.action.required') }}</t-checkbox>
              <t-input v-model="f.description" :placeholder="t('semantic.action.fieldDescPlaceholder')" class="flex-1" />
              <t-button variant="text" shape="square" size="small" class="custom-header-remove" @click="removeField(i)">
                <t-icon name="close" />
              </t-button>
            </div>
          </div>
          <t-button variant="text" size="small" theme="primary" @click="addField">
            <template #icon><t-icon name="add" /></template>
            {{ t('semantic.action.addField') }}
          </t-button>
          <p class="form-desc">{{ t('semantic.action.inputSchemaHint') }}</p>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.headers') }}</label>
          <div v-if="webhookHeaders.length" class="custom-headers-list">
            <div v-for="(h, i) in webhookHeaders" :key="i" class="custom-header-row">
              <t-input v-model="h.key" :placeholder="t('semantic.action.headerKeyPlaceholder')" class="custom-header-key" autocomplete="off" spellcheck="false" />
              <t-input v-model="h.value" :placeholder="t('semantic.action.headerValuePlaceholder', { token: secretToken })" class="custom-header-value" autocomplete="off" spellcheck="false" />
              <t-button variant="text" shape="square" size="small" class="custom-header-remove" @click="removeHeader(i)">
                <t-icon name="close" />
              </t-button>
            </div>
          </div>
          <t-button variant="text" size="small" theme="primary" @click="addHeader">
            <template #icon><t-icon name="add" /></template>
            {{ t('semantic.action.addHeader') }}
          </t-button>
          <p class="form-desc">{{ t('semantic.action.headersHint', { token: secretToken }) }}</p>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.secret') }}</label>
          <t-input v-model="form.secret_value" type="password"
            :placeholder="editing ? t('semantic.action.secretKeepHint') : t('semantic.action.secretHint', { token: secretToken })" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('semantic.action.bodyTemplate') }}</label>
          <t-textarea v-model="webhookBody" :autosize="{ minRows: 2, maxRows: 4 }"
            :placeholder="t('semantic.action.bodyTemplatePlaceholder', { example: bodyExample })" />
          <p class="form-desc">{{ t('semantic.action.bodyTemplateHint') }}</p>
        </div>
      </section>
    </SettingDrawer>
  </div>
</template>

<script setup lang="ts">
import { ref, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { createAction, deleteAction, updateAction, type ActionInput, type ActionField, type Precondition, type SemanticAction, type DataGroup } from './api'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'

const props = defineProps<{
  modelValue: SemanticAction[]
  canManage: boolean
  availableModels: string[]
  groups: DataGroup[]
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: SemanticAction[]): void }>()

const { t } = useI18n()
const slugPattern = /^[a-z][a-z0-9_]{0,62}$/

const dialogVisible = ref(false)
const editing = ref<SemanticAction | null>(null)
const saving = ref(false)
const nameError = ref('')

const form = ref<ActionInput & { input_schema: ActionField[]; preconditions: Precondition[] }>({
  name: '', title: '', description: '',
  object_types: [], input_schema: [], preconditions: [],
  backend: { type: 'webhook' },
  allowed_groups: [], secret_value: '',
})
const webhookUrl = ref('')
const webhookMethod = ref('POST')
const webhookBody = ref('')
const webhookHeaders = ref<{ key: string; value: string }[]>([])
const webhookSuccessStatus = ref('')

// ---- 从 JSON 导入 (纯前端解析, 照 MCP 服务弹窗的代码导入模式) ----
const importOpen = ref(false)
const importText = ref('')
const importError = ref('')

// 占位示例不走 i18n: vue-i18n 会把大括号当插值编译, 直接用常量。
const importPlaceholder = `{
  "name": "create_ticket",
  "title": "创建工单",
  "description": "为异常批次创建处理工单",
  "object_types": ["orders"],
  "allowed_groups": ["sales"],
  "input_schema": [
    { "column": "part_sn", "type": "string", "required": true, "description": "产品 SN" }
  ],
  "preconditions": [
    { "description": "批次存在", "query": { "measures": ["orders.count"], "filters": [{ "member": "orders.batch_no", "operator": "equals", "values": ["{{.batch_no}}"] }] }, "expect": "rows_gt_0" }
  ],
  "backend": {
    "type": "webhook",
    "url": "https://erp.example.com/api/tickets",
    "method": "POST",
    "headers": { "X-Auth": "{{secret}}" },
    "body_template": "{\\"part_sn\\":\\"{{.part_sn}}\\"}"
  }
}`

// 把导入的 JSON 对象映射到表单。宽松取值: 字段缺失保持表单现状, 类型不对的忽略。
function applyActionJSON(obj: Record<string, unknown>): boolean {
  if (String(obj.backend?.type ?? 'webhook') !== 'webhook') {
    importError.value = t('semantic.action.import.errors.unsupportedBackend')
    return false
  }
  const strArr = (v: unknown): string[] =>
    Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []

  if (typeof obj.name === 'string' && obj.name) form.value.name = obj.name
  if (typeof obj.title === 'string') form.value.title = obj.title
  if (typeof obj.description === 'string') form.value.description = obj.description
  if (Array.isArray(obj.object_types)) form.value.object_types = strArr(obj.object_types)
  if (Array.isArray(obj.allowed_groups)) form.value.allowed_groups = strArr(obj.allowed_groups)

  if (Array.isArray(obj.input_schema)) {
    form.value.input_schema = (obj.input_schema as Record<string, unknown>[])
      .filter(f => f && typeof f === 'object' && typeof f.column === 'string' && f.column)
      .map(f => ({
        column: f.column as string,
        type: typeof f.type === 'string' && f.type ? f.type : 'string',
        required: f.required === true,
        description: typeof f.description === 'string' ? f.description : '',
        ...(Array.isArray(f.enum) ? { enum: strArr(f.enum) } : {}),
        ...(f.default !== undefined ? { default: f.default } : {}),
      }))
  }

  // 前置条件整段透传 (表单没有编辑区, 但导出/导入必须携带, 否则跨环境丢失)
  if (Array.isArray(obj.preconditions)) {
    form.value.preconditions = (obj.preconditions as Record<string, unknown>[])
      .filter(p => p && typeof p === 'object')
      .map(p => ({
        description: typeof p.description === 'string' ? p.description : '',
        query: p.query ?? {},
        expect: typeof p.expect === 'string' ? p.expect : 'rows_gt_0',
      }))
  }

  const b = (obj.backend ?? {}) as Record<string, unknown>
  if (typeof b.url === 'string') webhookUrl.value = b.url
  if (typeof b.method === 'string' && methodOptions.some(m => m.value === b.method.toUpperCase())) {
    webhookMethod.value = b.method.toUpperCase()
  }
  if (typeof b.body_template === 'string') webhookBody.value = b.body_template
  if (b.success_status === undefined || b.success_status === null || (Array.isArray(b.success_status) && b.success_status.length === 0)) {
    webhookSuccessStatus.value = ''
  } else {
    webhookSuccessStatus.value = JSON.stringify(b.success_status)
  }
  if (b.headers && typeof b.headers === 'object' && !Array.isArray(b.headers)) {
    webhookHeaders.value = Object.entries(b.headers as Record<string, unknown>)
      .map(([key, value]) => ({ key, value: typeof value === 'string' ? value : String(value ?? '') }))
  }
  if (typeof obj.secret_value === 'string' && obj.secret_value) form.value.secret_value = obj.secret_value
  return true
}

function handleImport() {
  importError.value = ''
  const raw = importText.value.trim()
  if (!raw) {
    importError.value = t('semantic.action.import.errors.empty')
    return
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    importError.value = t('semantic.action.import.errors.invalidJson')
    return
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    importError.value = t('semantic.action.import.errors.noAction')
    return
  }
  if (!applyActionJSON(parsed as Record<string, unknown>)) return
  MessagePlugin.success(t('semantic.action.import.toasts.filled') as string)
}

// ---- 复制 JSON (跨环境迁移: 开发 → 测试 → 生产) ----

// 动作的可迁移配置 = 业务字段 + backend; 服务端状态 (id/租户/时间/密钥) 不导出。
// webhook 认证令牌从不返回 (响应里只有 {{secret}} 占位符), 复制件可安全传递。
function actionToExportJSON(a: SemanticAction): Record<string, unknown> {
  const out: Record<string, unknown> = {
    name: a.name,
    title: a.title,
    description: a.description,
    object_types: a.object_types || [],
    allowed_groups: a.allowed_groups || [],
    input_schema: a.input_schema || [],
  }
  if (a.preconditions?.length) out.preconditions = a.preconditions
  const b: Record<string, unknown> = { type: 'webhook' }
  if (a.backend?.url) b.url = a.backend.url
  if (a.backend?.method) b.method = a.backend.method
  if (a.backend?.headers && Object.keys(a.backend.headers).length) b.headers = a.backend.headers
  if (a.backend?.body_template) b.body_template = a.backend.body_template
  if (a.backend?.success_status?.length) b.success_status = a.backend.success_status
  out.backend = b
  return out
}

// navigator.clipboard 仅在安全上下文可用; 内网 http 部署走 execCommand 降级。
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch { /* fall through to execCommand */ }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  let ok = false
  try { ok = document.execCommand('copy') } catch { ok = false }
  document.body.removeChild(ta)
  return ok
}

async function copyJSON(a: SemanticAction) {
  const text = JSON.stringify(actionToExportJSON(a), null, 2)
  const ok = await copyText(text)
  if (ok) {
    MessagePlugin.success(t('semantic.action.import.toasts.copied') as string)
  } else {
    // 剪贴板彻底不可用时展示出来让用户手动复制
    DialogPlugin.alert({
      header: t('semantic.action.import.copyJson'),
      body: () => h('pre', { style: 'max-height: 320px; overflow: auto; font-size: 12px; white-space: pre-wrap;' }, text),
    })
  }
}

const fieldTypeOptions = [
  { value: 'string', label: 'string' },
  { value: 'int', label: 'int' },
  { value: 'float', label: 'float' },
  { value: 'bool', label: 'bool' },
  { value: 'date', label: 'date' },
  { value: 'json', label: 'json' },
]
const methodOptions = [
  { value: 'GET', label: 'GET' },
  { value: 'POST', label: 'POST' },
  { value: 'PUT', label: 'PUT' },
  { value: 'PATCH', label: 'PATCH' },
]

// 模板语法示例作为参数传入 i18n (值不会被 vue-i18n 编译器解析, 避免大括号
// 被当成插值占位符导致 Message compilation error)
const secretToken = '{{secret}}'
const bodyExample = '{"part_sn":"{{.part_sn}}"}'

function shortTime(ts: string): string {
  if (!ts) return ''
  return ts.slice(0, 16).replace('T', ' ')
}

// 类型徽章的悬浮提示: 关联模型 + 允许的数据组
function badgeTooltip(a: SemanticAction): string {
  const models = a.object_types?.length ? `${t('semantic.action.objectTypes')}: ${a.object_types.join(', ')}` : ''
  const groups = a.allowed_groups?.length ? `${t('semantic.action.allowedGroups')}: ${a.allowed_groups.join(', ')}` : ''
  return [models, groups].filter(Boolean).join('\n')
}

function openCreate() {
  editing.value = null
  nameError.value = ''
  form.value = {
    name: '', title: '', description: '',
    object_types: [], input_schema: [], preconditions: [],
    backend: { type: 'webhook' },
    allowed_groups: [], secret_value: '',
  }
  webhookUrl.value = ''
  webhookMethod.value = 'POST'
  webhookBody.value = ''
  webhookHeaders.value = []
  webhookSuccessStatus.value = ''
  importOpen.value = false
  importText.value = ''
  importError.value = ''
  dialogVisible.value = true
}

function openEdit(a: SemanticAction) {
  editing.value = a
  nameError.value = ''
  form.value = {
    name: a.name, title: a.title, description: a.description,
    object_types: [...(a.object_types || [])],
    input_schema: (a.input_schema || []).map(f => ({ ...f })),
    preconditions: (a.preconditions || []).map(p => ({ ...p })),
    backend: { ...a.backend, type: 'webhook' },
    allowed_groups: [...(a.allowed_groups || [])],
    secret_value: '',
  }
  webhookUrl.value = a.backend?.url || ''
  webhookMethod.value = a.backend?.method || 'POST'
  webhookBody.value = a.backend?.body_template || ''
  webhookHeaders.value = Object.entries(a.backend?.headers || {}).map(([key, value]) => ({ key, value: String(value) }))
  webhookSuccessStatus.value = a.backend?.success_status?.length ? JSON.stringify(a.backend.success_status) : ''
  importOpen.value = false
  importText.value = ''
  importError.value = ''
  dialogVisible.value = true
}

function addField() {
  if (!form.value.input_schema) form.value.input_schema = []
  form.value.input_schema.push({ column: '', type: 'string', required: false, description: '' })
}

function removeField(idx: number) {
  if (!form.value.input_schema) return
  form.value.input_schema.splice(idx, 1)
}

function addHeader() {
  webhookHeaders.value.push({ key: '', value: '' })
}

function removeHeader(idx: number) {
  webhookHeaders.value.splice(idx, 1)
}

async function save() {
  if (!editing.value && !slugPattern.test(form.value.name || '')) {
    nameError.value = t('semantic.action.nameError')
    return
  }
  saving.value = true
  try {
    const headers: Record<string, string> = {}
    for (const h2 of webhookHeaders.value) {
      if (h2.key.trim()) headers[h2.key.trim()] = h2.value
    }
    let successStatus: number[] | undefined
    if (webhookSuccessStatus.value.trim()) {
      try {
        const parsed = JSON.parse(webhookSuccessStatus.value)
        if (Array.isArray(parsed)) successStatus = parsed.map(Number).filter(n => !Number.isNaN(n))
      } catch { /* 非法值忽略, 保持默认 */ }
    }
    const payload: ActionInput = {
      ...form.value,
      preconditions: form.value.preconditions,
      backend: {
        type: 'webhook',
        url: webhookUrl.value,
        method: webhookMethod.value,
        body_template: webhookBody.value,
        headers: Object.keys(headers).length > 0 ? headers : undefined,
        ...(successStatus?.length ? { success_status: successStatus } : {}),
      },
    }
    let updated: SemanticAction
    if (editing.value) {
      updated = (await updateAction(editing.value.id, payload)) as SemanticAction
    } else {
      updated = (await createAction(payload)) as SemanticAction
    }
    const list = editing.value
      ? props.modelValue.map(a => a.id === updated.id ? updated : a)
      : [...props.modelValue, updated]
    emit('update:modelValue', list)
    dialogVisible.value = false
    MessagePlugin.success(t('semantic.action.saved'))
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'save failed')
  } finally {
    saving.value = false
  }
}

function confirmDelete(a: SemanticAction) {
  const dlg = DialogPlugin.confirm({
    header: t('semantic.action.delete'),
    body: `${a.title || a.name} — ${t('semantic.action.deleteConfirm')}`,
    theme: 'warning',
    onConfirm: async () => {
      try {
        await deleteAction(a.id)
        emit('update:modelValue', props.modelValue.filter(x => x.id !== a.id))
        MessagePlugin.success(t('semantic.action.deleted'))
      } catch (e: any) {
        MessagePlugin.error(e?.response?.data?.error || 'delete failed')
      }
      dlg.destroy()
    },
  })
}

defineExpose({ openCreate })
</script>

<style scoped>
/* ---- 卡片: 照抄 ConnectionsTab (数据源) 的卡片语言 ---- */
.actions-tab .card-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

@media (min-width: 1250px) {
  .actions-tab .card-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1600px) {
  .actions-tab .card-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

.kb-style-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  background: var(--td-bg-color-container);
  position: relative;
  cursor: pointer;
  transition: all 0.25s ease;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  height: 136px;
  min-height: 136px;
}

.kb-style-card:hover {
  border-color: var(--td-brand-color);
  box-shadow: 0 4px 12px rgba(7, 192, 95, 0.12);
}

.kb-style-card::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  width: 60px;
  height: 60px;
  background: linear-gradient(135deg, rgba(7, 192, 95, 0.08) 0%, transparent 100%);
  border-radius: 0 12px 0 100%;
  pointer-events: none;
  z-index: 0;
}

.card-header,
.card-content,
.card-bottom {
  position: relative;
  z-index: 1;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.card-title {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}

.card-title-text {
  font-size: 16px;
  font-weight: 500;
  line-height: 22px;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-slug {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  font-family: 'SFMono-Regular', Consolas, Menlo, monospace;
}

.more-wrap {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  flex-shrink: 0;
}

.more-wrap:hover {
  background: var(--td-bg-color-secondarycontainer);
}

.more-wrap .more-icon {
  width: 16px;
  height: 16px;
}

.card-content {
  flex: 1;
  min-height: 0;
  margin-bottom: 8px;
  overflow: hidden;
}

.card-description {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  overflow: hidden;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  font-weight: 400;
  line-height: 18px;
}

.card-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: auto;
  padding-top: 8px;
  border-top: 0.5px solid var(--td-component-stroke);
}

.bottom-left {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.bottom-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.card-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.feature-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  height: 22px;
  border-radius: 5px;
  padding: 0 6px;
  cursor: default;
  transition: background 0.2s ease;
  font-size: 11px;
  font-weight: 500;
}

.feature-badge.type-badge {
  background: rgba(0, 82, 217, 0.08);
  color: var(--td-brand-color);
  max-width: 160px;
}

.feature-badge.type-badge .badge-text {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.feature-badge.status-badge.active {
  background: rgba(7, 192, 95, 0.08);
  color: var(--td-brand-color-active);
}

.feature-badge.status-badge.error,
.feature-badge.status-badge.inactive {
  background: rgba(227, 77, 89, 0.08);
  color: var(--td-error-color);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 0;
}
.empty-state .empty-img {
  width: 120px;
  opacity: 0.8;
}
.empty-state .empty-txt {
  font-size: 15px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}
.empty-state-btn {
  width: fit-content;
}
.field-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.field-key {
  flex: 0 0 30%;
}
.field-type {
  width: 96px;
  flex-shrink: 0;
}
.field-required {
  flex-shrink: 0;
}
.field-required :deep(.t-checkbox__label) {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.flex-1 {
  flex: 1;
  min-width: 0;
}
.popup-menu {
  min-width: 120px;
}
.popup-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  font-size: 13px;
}
.popup-menu-item:hover {
  background: var(--td-bg-color-container-hover);
}
.popup-menu-item.delete {
  color: var(--td-error-color);
}
.menu-icon {
  font-size: 16px;
}
/* ---- 从 JSON 导入区: 照抄 MCP 服务弹窗的 code-import 设计 ---- */
.code-import__toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  border: none;
  background: none;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}
.code-import__toggle:hover {
  color: var(--td-brand-color);
}
.code-import__body {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.code-import__warn {
  color: var(--td-warning-color);
}
.code-import__textarea :deep(textarea) {
  font-family: var(--td-font-family-mono, monospace);
  font-size: 12px;
}
.code-import__error {
  margin: 0;
  font-size: 12px;
  color: var(--td-error-color);
}
.code-import__actions {
  display: flex;
  justify-content: flex-end;
}
/* ---- 表单样式: 对齐知识库 DataSourceEditorDialog 的 form-item 体系 ---- */
.form-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
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
.custom-headers-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}
.custom-header-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.custom-header-key {
  flex: 0 0 38%;
}
.custom-header-value {
  flex: 1;
  min-width: 0;
}
.custom-header-remove {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--td-text-color-placeholder);
  border-radius: 6px;
  transition: all 0.18s ease;
}
.custom-header-remove:hover {
  background: var(--td-error-color-light);
  color: var(--td-error-color);
}
</style>
