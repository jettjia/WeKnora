<template>
  <div class="models-tab">
    <!-- 卡片网格: 对齐知识库/智能体列表的卡片语言 -->
    <div v-if="loading" class="card-grid">
      <div v-for="n in 6" :key="'skel-' + n" class="kb-style-card is-skeleton">
        <div class="card-header"><t-skeleton animation="gradient" :row-col="[{ width: '60%', height: '20px' }]" /></div>
        <div class="card-content"><t-skeleton animation="gradient" :row-col="[{ width: '100%', height: '14px' }, { width: '80%', height: '14px' }]" /></div>
        <div class="card-bottom"><t-skeleton animation="gradient" :row-col="[[{ width: '28px', height: '28px', type: 'rect' }, { width: '28px', height: '28px', type: 'rect' }]]" /></div>
      </div>
    </div>
    <div v-else-if="visibleModels.length" class="card-grid">
      <div v-for="m in visibleModels" :key="m.id" class="kb-style-card model-card" @click="openEditor(m)">
        <button type="button" class="kb-favorite-star" :class="{ 'is-favorited': isFavorited(m) }"
          @click.stop="toggleFavorite(m, $event)">
          <t-icon :name="isFavorited(m) ? 'star-filled' : 'star'" size="14px" />
        </button>
        <div class="card-header">
          <span class="card-title" :title="m.name">
            <span class="card-title-text">{{ m.title || m.name }}</span>
            <span class="card-slug">{{ m.name }}</span>
          </span>
          <t-popup overlay-class-name="card-more-popup" trigger="click" destroy-on-close placement="bottom-right">
            <div class="more-wrap" @click.stop>
              <img class="more-icon" src="@/assets/img/more.png" alt="" />
            </div>
            <template #content>
              <div class="popup-menu" @click.stop>
                <div class="popup-menu-item" @click.stop="openEditor(m)">
                  <t-icon class="menu-icon" name="edit" />
                  <span>{{ t('semantic.model.editor.basic') }}</span>
                </div>
                <div class="popup-menu-item" @click.stop="openVersions(m)">
                  <t-icon class="menu-icon" name="history" />
                  <span>{{ t('semantic.model.versions') }}</span>
                </div>
                <div class="popup-menu-item" @click.stop="openShare(m)">
                  <t-icon class="menu-icon" name="share" />
                  <span>{{ t('semantic.model.share') }}</span>
                </div>
                <template v-if="canManageModel(m)">
                  <div class="popup-menu-item" @click.stop="publish(m)">
                    <t-icon class="menu-icon" name="cloud-upload" />
                    <span>{{ t('semantic.model.publish') }}</span>
                  </div>
                  <div v-if="m.status === 'published'" class="popup-menu-item" @click.stop="unpublish(m)">
                    <t-icon class="menu-icon" name="cloud-download" />
                    <span>{{ t('semantic.model.unpublish') }}</span>
                  </div>
                  <div class="popup-menu-item delete" @click.stop="confirmDelete(m)">
                    <t-icon class="menu-icon" name="delete" />
                    <span>{{ t('semantic.model.delete') }}</span>
                  </div>
                </template>
              </div>
            </template>
          </t-popup>
        </div>

        <div class="card-content">
          <div class="card-description">
            {{ m.description || t('semantic.model.noDescription') }}
          </div>
        </div>

        <div class="card-bottom">
          <div class="bottom-left">
            <t-tooltip :content="statusLabel(m.status)" placement="top">
              <div class="feature-badge" :class="m.status">
                <t-icon :name="m.status === 'published' ? 'check-circle' : m.status === 'publish_failed' ? 'error-circle' : 'edit-1'" size="14px" />
                <span class="badge-text">{{ statusLabel(m.status) }}</span>
              </div>
            </t-tooltip>
            <div class="feature-badge conn-badge">
              <t-icon name="server" size="14px" />
              <span class="badge-text">{{ connTitle(m.connection_id) }}</span>
            </div>
          </div>
          <div class="bottom-right">
            <span v-if="m.version" class="card-time">v{{ m.version }}</span>
            <span class="card-time">{{ shortTime(m.updated_at) }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 空状态: 对齐知识库列表 (搜索无结果时只给无结果文案, 不给空态引导和新建入口) -->
    <div v-else class="empty-state">
      <img v-if="!hasKeyword" class="empty-img" src="@/assets/img/upload.svg" alt="" />
      <span class="empty-txt">{{
        hasKeyword ? t('semantic.noResult')
        : spaceSelection === 'favorites' ? t('semantic.model.emptyFavorites')
        : spaceSelection === 'recents' ? t('semantic.model.emptyRecents')
        : t('semantic.model.empty')
      }}</span>
      <t-button v-if="!hasKeyword && spaceSelection === 'all' && canEdit && connections.length" class="empty-state-btn" @click="openCreate">
        <template #icon><t-icon name="add" /></template>
        {{ t('semantic.model.add') }}
      </t-button>
    </div>

    <!-- 发布说明 -->
    <PublishNoteDialog v-model:visible="noteVisible" @confirm="doPublish" />

    <!-- 新建对话框: 从表生成 / 空白 -->
    <t-dialog v-model:visible="createVisible" :header="t('semantic.model.add')" width="560px" @confirm="create">
      <t-form label-width="110px" label-align="right">
        <t-form-item :label="t('semantic.model.connection')">
          <t-select v-model="createForm.connection_id" @change="onConnChange">
            <t-option v-for="c in connections" :key="c.id" :value="c.id" :label="`${c.title || c.name} (${c.type})`" />
          </t-select>
        </t-form-item>
        <t-form-item :label="t('semantic.model.fromTable')">
          <t-cascader
            v-model="createForm.tableRef"
            :options="tableOptions"
            check-strictly
            clearable
            :placeholder="t('semantic.model.chooseTable')"
            :disabled="!createForm.connection_id"
          />
        </t-form-item>
        <t-form-item :label="t('semantic.model.blank')">
          <t-switch v-model="createForm.blank" />
        </t-form-item>
      </t-form>
      <div class="create-hint">{{ t('semantic.model.publishHint') }}</div>
    </t-dialog>

    <!-- 版本历史 -->
    <t-drawer v-model:visible="versionsVisible" :header="t('semantic.model.versions')" size="480px" :footer="false">
      <t-list v-if="versions.length">
        <t-list-item v-for="v in versions" :key="v.id">
          <div class="version-row">
            <div>
              <div class="version-title">v{{ v.version }}</div>
              <div class="version-note">{{ v.note || '-' }}</div>
              <div class="version-meta">{{ v.published_by }} · {{ v.published_at }}</div>
            </div>
            <t-button v-if="canManageModel(editing!)" size="small" variant="outline" @click="rollback(v)">
              {{ t('semantic.model.rollback') }}
            </t-button>
          </div>
        </t-list-item>
      </t-list>
      <t-empty v-else />
    </t-drawer>

    <ModelEditor
      v-model:visible="editorVisible"
      :model="editing"
      :connections="connections"
      :groups="groups"
      :can-edit="canEdit"
      :can-publish="canPublish"
      :current-user-id="currentUserId"
      @saved="onSaved"
      @published="emit('changed')"
    />
      <ShareModelDialog ref="shareDialogRef" :orgs="shareOrgs" />

</div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import {
  listTables,
  generateDraft,
  listModels,
  createModel,
  deleteModel,
  publishModel,
  unpublishModel,
  listVersions,
  rollbackModel,
  type ConnectionInfo,
  type DataGroup,
  type ModelVersion,
  type SemanticModel,
  type TableRef, shareModel, unshareModel, listModelShares } from './api'
import { listMyOrganizations } from '@/api/organization'
import ShareModelDialog from './ShareModelDialog.vue'
import ModelEditor from './ModelEditor.vue'
import PublishNoteDialog from './PublishNoteDialog.vue'
import { listFavorites, addFavorite, removeFavorite } from '@/api/user-favorites'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  connections: ConnectionInfo[]
  groups: DataGroup[]
  spaceSelection: string
  search?: string
  canEdit: boolean
  canPublish: boolean
  currentUserId: string
  cubeReady: boolean
}>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const { t } = useI18n()
const authStore = useAuthStore()

const models = ref<SemanticModel[]>([])
const loading = ref(true)
const createVisible = ref(false)
const createForm = ref<{ connection_id: string; tableRef: string[]; blank: boolean }>({
  connection_id: '',
  tableRef: [],
  blank: false
})
const tableOptions = ref<{ label: string; value: string; children?: { label: string; value: string }[] }[]>([])
const generating = ref(false)
const editorVisible = ref(false)
const editing = ref<SemanticModel | null>(null)
const noteVisible = ref(false)
const publishTarget = ref<SemanticModel | null>(null)
const versionsVisible = ref(false)
const versions = ref<ModelVersion[]>([])

// ---- 收藏 (复用平台 user_resource_favorites, type=semantic_model) ----
const favoriteIds = ref<Set<string>>(new Set())

async function loadFavorites() {
  try {
    const resp = await listFavorites('semantic_model')
    favoriteIds.value = new Set((resp.data || []).map(f => f.resource_id))
  } catch {
    favoriteIds.value = new Set()
  }
}
loadFavorites()

function isFavorited(m: SemanticModel) {
  return favoriteIds.value.has(m.id)
}

async function toggleFavorite(m: SemanticModel, ev: Event) {
  ev.stopPropagation()
  try {
    if (isFavorited(m)) {
      await removeFavorite('semantic_model', m.id)
      favoriteIds.value.delete(m.id)
    } else {
      await addFavorite('semantic_model', m.id)
      favoriteIds.value.add(m.id)
    }
    favoriteIds.value = new Set(favoriteIds.value)
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'favorite failed')
  }
}

// 最近查看: 本地记录 (与知识库 recents 同思路, 轻量实现)
const RECENT_KEY = 'semantic_recent_models'
const recentIds = ref<string[]>([])
try {
  recentIds.value = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
} catch { /* ignore */ }

function markRecent(m: SemanticModel) {
  recentIds.value = [m.id, ...recentIds.value.filter(id => id !== m.id)].slice(0, 10)
  localStorage.setItem(RECENT_KEY, JSON.stringify(recentIds.value))
}

// 侧栏视图过滤 (all | favorites | recents | mine) + 页头搜索框 (全视图生效)
const visibleModels = computed(() => {
  let base: SemanticModel[]
  if (props.spaceSelection === 'favorites') {
    base = models.value.filter(m => favoriteIds.value.has(m.id))
  } else if (props.spaceSelection === 'mine') {
    const uid = authStore.currentUserId
    base = models.value.filter(m => m.created_by === uid)
  } else if (props.spaceSelection === 'recents') {
    const order = new Map(recentIds.value.map((id, i) => [id, i]))
    base = models.value
      .filter(m => order.has(m.id))
      .sort((a, b) => (order.get(a.id) || 0) - (order.get(b.id) || 0))
  } else {
    base = models.value
  }
  const kw = (props.search || '').trim().toLowerCase()
  if (!kw) return base
  return base.filter(m => `${m.title || ''} ${m.name} ${m.description || ''}`.toLowerCase().includes(kw))
})
const hasKeyword = computed(() => !!(props.search || '').trim())

const count = computed(() => models.value.length)
const currentUserId = computed(() => props.currentUserId)

function canManageModel(m: SemanticModel) {
  return m.created_by === currentUserId.value || props.canPublish
}
const favoritesCount = computed(() => models.value.filter(m => favoriteIds.value.has(m.id)).length)
const mineCount = computed(() => models.value.filter(m => m.created_by === authStore.currentUserId).length)
defineExpose({ openCreate, count, favoritesCount, mineCount, models })

function statusLabel(s: string) {
  if (s === 'published') return t('semantic.model.statusPublished')
  if (s === 'publish_failed') return t('semantic.model.statusFailed')
  return t('semantic.model.statusDraft')
}
function connTitle(id: string) {
  const c = props.connections.find(x => x.id === id)
  return c ? c.title || c.name : '-'
}
function shortTime(ts: string) {
  return ts ? ts.slice(5, 16) : ''
}

async function load() {
  loading.value = true
  try {
    const resp = await listModels()
    models.value = resp.models || []
  } finally {
    loading.value = false
  }
}
load()

watch(
  () => props.connections,
  () => load(),
  { deep: true }
)

function openCreate() {
  createForm.value = { connection_id: props.connections[0]?.id || '', tableRef: [], blank: false }
  createVisible.value = true
  if (createForm.value.connection_id) onConnChange(createForm.value.connection_id)
}

async function onConnChange(connId: string) {
  if (!connId) return
  generating.value = true
  try {
    const resp = await listTables(connId)
    const bySchema: Record<string, TableRef[]> = {}
    for (const tb of resp.tables || []) {
      ;(bySchema[tb.schema] ||= []).push(tb)
    }
    tableOptions.value = Object.entries(bySchema).map(([schema, tables]) => ({
      label: schema,
      value: schema,
      children: tables.map(tb => ({ label: tb.name, value: tb.name }))
    }))
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'load tables failed')
  } finally {
    generating.value = false
  }
}

async function create() {
  if (!createForm.value.connection_id) {
    MessagePlugin.warning(t('semantic.model.chooseConn'))
    return
  }
  generating.value = true
  try {
    let draftYaml = ''
    if (!createForm.value.blank && createForm.value.tableRef.length >= 1) {
      const [schema, table] =
        createForm.value.tableRef.length === 2
          ? [createForm.value.tableRef[0], createForm.value.tableRef[1]]
          : ['', createForm.value.tableRef[0]]
      const resp = await generateDraft(createForm.value.connection_id, schema, table)
      draftYaml = resp.yaml
    } else {
      draftYaml = `cubes:
  - name: new_cube
    title: ''
    sql: SELECT * FROM schema.table
    data_source: ${props.connections.find(c => c.id === createForm.value.connection_id)?.name || ''}
    measures:
      - name: count
        type: count
    dimensions: []
`
    }
    const created = await createModel({
      connection_id: createForm.value.connection_id,
      kind: 'cube',
      draft_yaml: draftYaml,
      allowed_groups: []
    })
    models.value.push(created)
    createVisible.value = false
    editing.value = created
    editorVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'create failed')
  } finally {
    generating.value = false
  }
}

function openEditor(m: SemanticModel) {
  markRecent(m)
  editing.value = m
  editorVisible.value = true
}

function openVersions(m: SemanticModel) {
  editing.value = m
  listVersions(m.id).then(resp => {
    versions.value = resp.versions || []
    versionsVisible.value = true
  })
}

function publish(m: SemanticModel) {
  publishTarget.value = m
  noteVisible.value = true
}

async function doPublish(note: string) {
  if (!publishTarget.value) return
  noteVisible.value = false
  try {
    const resp = await publishModel(publishTarget.value.id, note)
    MessagePlugin.success(t('semantic.model.publishOk', { n: resp.version }))
    await load()
  } catch (e: any) {
    MessagePlugin.error(t('semantic.model.publishFail', { err: e?.response?.data?.error || '' }))
    await load()
  }
}

async function unpublish(m: SemanticModel) {
  try {
    await unpublishModel(m.id)
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'unpublish failed')
  }
}

const shareDialogRef = ref<InstanceType<typeof ShareModelDialog> | null>(null)
const shareOrgs = ref<{ id: string; name: string }[]>([])

async function openShare(m: SemanticModel) {
  if (!shareOrgs.value.length) {
    try {
      const resp = await listMyOrganizations()
      shareOrgs.value = (resp?.data?.organizations || []).map((o: any) => ({ id: o.id, name: o.name }))
    } catch {
      shareOrgs.value = []
    }
  }
  shareDialogRef.value?.open(m)
}

function confirmDelete(m: SemanticModel) {
  const dialog = DialogPlugin.confirm({
    header: t('semantic.model.delete'),
    body: t('semantic.model.deleteConfirm'),
    confirmBtn: { content: t('semantic.model.delete'), theme: 'danger' },
    onConfirm: async () => {
      try {
        await deleteModel(m.id)
        models.value = models.value.filter(x => x.id !== m.id)
        emit('changed')
      } catch (e: any) {
        MessagePlugin.error(e?.response?.data?.error || 'delete failed')
      }
      dialog.destroy()
    },
    onClose: () => dialog.destroy()
  })
}

async function rollback(v: ModelVersion) {
  if (!editing.value) return
  try {
    await rollbackModel(editing.value.id, v.id)
    MessagePlugin.success(t('semantic.model.publishOk', { n: v.version }))
    versionsVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'rollback failed')
  }
}

function onSaved(updated: SemanticModel) {
  models.value = models.value.map(m => (m.id === updated.id ? updated : m))
}
</script>

<style scoped lang="less">
@import (reference) '@/components/css/resource-card.less';
.create-hint {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.version-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.version-title {
  font-weight: 600;
}
.version-note,
.version-meta {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

/* ---- 卡片网格与卡片样式: 复用知识库列表的卡片语言 ---- */
.card-grid {
  .resource-card-grid();
}
.kb-style-card {
  .resource-card();
  .kb-favorite-star {
    .resource-favorite-button();
  }
}
.card-slug {
  flex-shrink: 0;
  font-size: var(--app-text-xs);
  color: var(--td-text-color-placeholder);
  font-family: 'SFMono-Regular', Consolas, Menlo, monospace;
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
  font-size: var(--app-text-sm);
  color: var(--td-text-color-placeholder);
}
.feature-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  height: 22px;
  border-radius: var(--app-radius-sm);
  padding: 0 6px;
  cursor: default;
  transition: background var(--app-motion-base) ease;
  font-size: var(--app-text-xs);
  font-weight: 500;
}
.feature-badge.published {
  background: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
  color: var(--td-brand-color-active);
}
.feature-badge.draft {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}
.feature-badge.publish_failed {
  background: color-mix(in srgb, var(--td-error-color) 8%, transparent);
  color: var(--td-error-color);
}
.feature-badge.conn-badge {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}

</style>
