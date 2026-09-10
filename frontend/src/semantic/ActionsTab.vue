<template>
  <div class="actions-tab">
    <div v-if="modelValue.length" class="card-grid">
      <div v-for="a in modelValue" :key="a.id" class="kb-style-card action-card" @click="canManage && openEdit(a)">
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
                <div v-if="canManage" class="popup-menu-item" @click.stop="openEdit(a)">
                  <t-icon class="menu-icon" name="edit" />
                  <span>{{ t('semantic.action.edit') }}</span>
                </div>
                <div v-if="canManage" class="popup-menu-item" @click.stop="confirmDelete(a)">
                  <t-icon class="menu-icon" name="delete" />
                  <span>{{ t('semantic.action.delete') }}</span>
                </div>
              </div>
            </template>
          </t-popup>
        </div>

        <div class="card-content">
          <div class="card-description">{{ a.description || t('semantic.action.noDescription') }}</div>
          <div class="card-meta">
            <t-tag v-for="m in a.object_types" :key="m" size="small" variant="light-outline">{{ m }}</t-tag>
            <t-tag v-for="g in a.allowed_groups" :key="g" size="small" theme="warning" variant="light-outline">{{ g }}</t-tag>
          </div>
        </div>

        <div class="card-bottom">
          <div class="bottom-left">
            <t-tag size="small" theme="success" variant="light-outline" v-if="a.status === 'active'">{{ t('semantic.action.active') }}</t-tag>
            <t-tag size="small" theme="danger" variant="light-outline" v-else>{{ a.status }}</t-tag>
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
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { createAction, deleteAction, updateAction, type ActionInput, type ActionField, type SemanticAction, type DataGroup } from './api'
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

const form = ref<ActionInput & { input_schema: ActionField[] }>({
  name: '', title: '', description: '',
  object_types: [], input_schema: [],
  backend: { type: 'webhook' },
  allowed_groups: [], secret_value: '',
})
const webhookUrl = ref('')
const webhookMethod = ref('POST')
const webhookBody = ref('')
const webhookHeaders = ref<{ key: string; value: string }[]>([])

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

function openCreate() {
  editing.value = null
  nameError.value = ''
  form.value = {
    name: '', title: '', description: '',
    object_types: [], input_schema: [],
    backend: { type: 'webhook' },
    allowed_groups: [], secret_value: '',
  }
  webhookUrl.value = ''
  webhookMethod.value = 'POST'
  webhookBody.value = ''
  webhookHeaders.value = []
  dialogVisible.value = true
}

function openEdit(a: SemanticAction) {
  editing.value = a
  nameError.value = ''
  form.value = {
    name: a.name, title: a.title, description: a.description,
    object_types: [...(a.object_types || [])],
    input_schema: (a.input_schema || []).map(f => ({ ...f })),
    backend: { ...a.backend, type: 'webhook' },
    allowed_groups: [...(a.allowed_groups || [])],
    secret_value: '',
  }
  webhookUrl.value = a.backend?.url || ''
  webhookMethod.value = a.backend?.method || 'POST'
  webhookBody.value = a.backend?.body_template || ''
  webhookHeaders.value = Object.entries(a.backend?.headers || {}).map(([key, value]) => ({ key, value: String(value) }))
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
    for (const h of webhookHeaders.value) {
      if (h.key.trim()) headers[h.key.trim()] = h.value
    }
    const payload: ActionInput = {
      ...form.value,
      backend: {
        type: 'webhook',
        url: webhookUrl.value,
        method: webhookMethod.value,
        body_template: webhookBody.value,
        headers: Object.keys(headers).length > 0 ? headers : undefined,
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
.actions-tab .card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}
.kb-style-card.action-card {
  display: flex;
  flex-direction: column;
  padding: 16px 20px;
  border-radius: 8px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.kb-style-card.action-card:hover {
  border-color: var(--td-brand-color);
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.card-title {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.card-title-text {
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card-slug {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  font-family: 'SFMono-Regular', Consolas, Menlo, monospace;
}
.card-content {
  flex: 1;
  margin-bottom: 12px;
}
.card-description {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 20px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: 8px;
}
.card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.card-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.card-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
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
