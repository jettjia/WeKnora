<template>
  <div class="semantic-studio-container">
    <div class="semantic-studio">
      <!-- 页头: 对齐知识库列表页; 新建/类型切换/审计集中在右上角 (单模块聚焦) -->
      <div class="header">
        <div class="header-title">
          <div class="title-row">
            <h2><ResourceIcon type="semantic" :size="24" /> {{ t('semantic.menu') }}</h2>
            <t-tag v-if="info?.cube_ready" theme="success" variant="light-outline" size="small">
              {{ t('semantic.info.cubeReady') }}
            </t-tag>
            <t-tooltip v-else :content="info?.cube_error || t('semantic.info.cubeDown')">
              <t-tag theme="danger" variant="light-outline" size="small">
                {{ t('semantic.info.cubeDown') }}
              </t-tag>
            </t-tooltip>
          </div>
          <p class="header-subtitle">{{ t('semantic.info.subtitle') }}</p>
        </div>
        <div class="header-actions">
          <t-tooltip v-if="canAddActive" :content="activeTypeMeta.addLabel" placement="bottom">
            <t-button variant="text" theme="default" size="small" class="header-action-btn"
              :disabled="addBlocked" @click="addActive">
              <template #icon><t-icon name="add" size="16px" /></template>
              {{ activeTypeMeta.addLabel }}
            </t-button>
          </t-tooltip>
          <t-popup trigger="click" placement="bottom-right" destroy-on-close>
            <t-button variant="outline" theme="default" size="small" class="type-switcher">
              <template #icon><t-icon :name="activeTypeMeta.icon" size="15px" /></template>
              <span class="type-switcher-label">{{ activeTypeMeta.label }}</span>
              <template #suffix><t-icon name="chevron-down" size="14px" /></template>
            </t-button>
            <template #content>
              <div class="type-menu">
                <button v-for="opt in typeOptions" :key="opt.key" type="button" class="type-menu-item"
                  :class="{ 'is-active': opt.key === activeType }" @click="switchType(opt.key)">
                  <span class="type-menu-icon"><t-icon :name="opt.icon" size="17px" /></span>
                  <span class="type-menu-text">
                    <span class="type-menu-name">
                      {{ opt.label }}
                      <span class="type-menu-count">{{ opt.count }}</span>
                    </span>
                    <span class="type-menu-desc">{{ opt.desc }}</span>
                  </span>
                  <t-icon v-if="opt.key === activeType" name="check" size="16px" class="type-menu-check" />
                </button>
              </div>
            </template>
          </t-popup>
          <t-button v-if="isAdmin" variant="text" theme="default" size="small" class="audit-btn"
            @click="openAudit">
            <template #icon><t-icon name="history" size="15px" /></template>
            {{ t('semantic.audit.title') }}
          </t-button>
        </div>
      </div>

      <ResourceListToolbar
        v-model="toolbarScope"
        v-model:query="keyword"
        :hide-scopes="authStore.isLiteMode"
        :count-all="scopeCounts.all"
        :count-mine="scopeCounts.mine"
        :count-favorites="scopeCounts.favorites"
        :count-recents="scopeCounts.recents"
        :disabled-scopes="scopeCounts.disabled"
      />

      <div class="semantic-studio-main">
      <t-alert v-if="connectionsLoaded && !connections.length" theme="info" class="conn-hint">
        {{ t('semantic.info.noConnection') }}
      </t-alert>

      <!-- 单模块聚焦: 一次只呈现一个资源类型, 右上角切换; v-show 保留各列表状态 -->
      <ModelsTab
        v-show="activeType === 'models'"
        ref="modelsTabRef"
        :connections="connections"
        :groups="groups"
        :space-selection="spaceSelection"
        :search="keyword"
        :can-edit="canEdit"
        :can-publish="isAdmin"
        :current-user-id="authStore.currentUserId"
        :cube-ready="!!info?.cube_ready"
      />
      <ConnectionsTab v-show="activeType === 'connections'" ref="connectionsTabRef" v-model="connections"
        :can-manage="isAdmin" :search="keyword" @changed="reloadGroups" />
      <ActionsTab v-show="activeType === 'actions'" ref="actionsTabRef" v-model="actions" :can-manage="isAdmin"
        :available-models="modelNames" :groups="groups" :search="keyword" />
      <GroupsTab v-show="activeType === 'groups'" ref="groupsTabRef" v-model="groups" :can-manage="isAdmin"
        :search="keyword" />
      </div>

      <!-- 审计日志抽屉 -->
      <t-drawer v-model:visible="auditVisible" :header="t('semantic.audit.title')" size="560px" :footer="false">
        <t-table row-key="id" :data="auditLogs" :columns="auditColumns" size="small" max-height="70vh">
          <template #action="{ row }"><code class="audit-action">{{ row.action }}</code></template>
          <template #time="{ row }"><span class="audit-time">{{ row.created_at }}</span></template>
        </t-table>
      </t-drawer>

    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import {
  getModuleInfo, listConnections, listGroups, listAudits, listActions,
  type AuditLogEntry, type ConnectionInfo, type DataGroup, type ModuleInfo, type SemanticAction
} from './api'
import ResourceIcon from '@/components/icons/ResourceIcon.vue'
import ResourceListToolbar from '@/components/ResourceListToolbar.vue'
import ModelsTab from './ModelsTab.vue'
import ConnectionsTab from './ConnectionsTab.vue'
import GroupsTab from './GroupsTab.vue'
import ActionsTab from './ActionsTab.vue'

const { t } = useI18n()
const authStore = useAuthStore()

const info = ref<ModuleInfo | null>(null)
const connections = ref<ConnectionInfo[]>([])
const connectionsLoaded = ref(false)
const groups = ref<DataGroup[]>([])
const actions = ref<SemanticAction[]>([])

const spaceSelection = ref('all')
const keyword = ref('')

const modelsTabRef = ref<InstanceType<typeof ModelsTab> | null>(null)
const connectionsTabRef = ref<InstanceType<typeof ConnectionsTab> | null>(null)
const groupsTabRef = ref<InstanceType<typeof GroupsTab> | null>(null)
const actionsTabRef = ref<InstanceType<typeof ActionsTab> | null>(null)

const isAdmin = computed(() => authStore.hasRole('admin'))
const canEdit = computed(() => authStore.hasRole('contributor'))

// 审计日志 (admin)
const auditVisible = ref(false)
const auditLogs = ref<AuditLogEntry[]>([])

const auditColumns = computed(() => [
  { colKey: 'action', title: t('semantic.audit.action'), width: 170 },
  { colKey: 'target', title: t('semantic.audit.target'), minWidth: 140 },
  { colKey: 'user_id', title: t('semantic.audit.user'), width: 100, ellipsis: true },
  { colKey: 'time', title: t('semantic.audit.time'), width: 160 }
])

function openAudit() {
  auditVisible.value = true
  listAudits().then(resp => { auditLogs.value = resp.audit_logs || [] }).catch(() => { auditLogs.value = [] })
}

const modelCount = computed(() => modelsTabRef.value?.count ?? 0)
const favoriteCount = computed(() => modelsTabRef.value?.favoritesCount ?? 0)
const mineCount = computed(() => modelsTabRef.value?.mineCount ?? 0)
const recentCount = computed(() => Math.min(modelCount.value, 10))

const modelNames = computed(() => modelsTabRef.value?.models?.map((m: { name: string }) => m.name) || [])

// ---- 右上角类型切换: 单模块聚焦, 首页一次只呈现一个资源类型 ----
type SemanticTypeKey = 'models' | 'connections' | 'actions' | 'groups'
const activeType = ref<SemanticTypeKey>('models')

const typeOptions = computed<{ key: SemanticTypeKey; icon: string; label: string; desc: string; count: number; addLabel: string }[]>(() => [
  { key: 'models', icon: 'layers', label: t('semantic.tabs.models'), desc: t('semantic.typeDesc.models'),
    count: modelCount.value, addLabel: t('semantic.model.add') },
  { key: 'connections', icon: 'server', label: t('semantic.tabs.connections'), desc: t('semantic.typeDesc.connections'),
    count: connections.value.length, addLabel: t('semantic.conn.add') },
  { key: 'actions', icon: 'play-circle', label: t('semantic.tabs.actions'), desc: t('semantic.typeDesc.actions'),
    count: actions.value.length, addLabel: t('semantic.action.add') },
  { key: 'groups', icon: 'usergroup', label: t('semantic.tabs.groups'), desc: t('semantic.typeDesc.groups'),
    count: groups.value.length, addLabel: t('semantic.group.add') },
])
const activeTypeMeta = computed(() => typeOptions.value.find(o => o.key === activeType.value) ?? typeOptions.value[0])

function switchType(key: SemanticTypeKey) {
  // 各类型完全隔离: scope 选择只作用于模型 (收藏/最近/本空间是模型的能力),
  // 其他类型固定「全部」且对应 scope 置灰, 切换不串数据
  activeType.value = key
}

// scope 选择只驱动模型列表; 其他类型下工具栏固定显示「全部」
const toolbarScope = computed({
  get: () => (activeType.value === 'models' ? spaceSelection.value : 'all'),
  set: (v: string) => { spaceSelection.value = v }
})

// scope 计数跟随当前类型; 收藏/最近/本空间仅模型支持, 其他类型不显示计数
// (disabled) 并置灰, 与模型视图完全隔离
const scopeCounts = computed<{ all: number; mine?: number; favorites?: number; recents?: number; disabled: string[] }>(() => {
  if (activeType.value === 'models') {
    return { all: modelCount.value, mine: mineCount.value, favorites: favoriteCount.value, recents: recentCount.value, disabled: [] }
  }
  const totals: Record<SemanticTypeKey, number> = {
    models: modelCount.value,
    connections: connections.value.length,
    actions: actions.value.length,
    groups: groups.value.length
  }
  return { all: totals[activeType.value], disabled: ['favorites', 'recents', 'mine'] }
})

const canAddActive = computed(() =>
  activeType.value === 'models' ? canEdit.value && spaceSelection.value === 'all' : isAdmin.value
)
// 模型依赖数据源: 一个连接都没有时, 新建模型无从谈起
const addBlocked = computed(() => activeType.value === 'models' && !connections.value.length)

function addActive() {
  const target = {
    models: modelsTabRef,
    connections: connectionsTabRef,
    actions: actionsTabRef,
    groups: groupsTabRef
  }[activeType.value]
  target?.value?.openCreate()
}

async function loadConnections() {
  try {
    const resp = await listConnections()
    connections.value = resp.connections || []
  } finally {
    connectionsLoaded.value = true
  }
}

async function reloadGroups() {
  try {
    const resp = await listGroups()
    groups.value = resp.groups || []
  } catch {
    groups.value = []
  }
}

async function loadActions() {
  try {
    const resp = await listActions()
    actions.value = resp.actions || []
  } catch {
    actions.value = []
  }
}

onMounted(async () => {
  try {
    info.value = await getModuleInfo()
  } catch {
    info.value = null
  }
  loadConnections()
  reloadGroups()
  loadActions()
})
</script>

<style scoped lang="less">
@import (reference) '@/components/css/resource-card.less';
.semantic-studio-main {
  .resource-list-main();
}
.semantic-studio-container {
  height: 100%;
  display: flex;
  min-width: 0;
  min-height: 0;
  box-sizing: border-box;
}

.semantic-studio {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 20px 28px 0 20px;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.header-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

/* 新建按钮: 同知识库页头 header-action-btn 的口径 (resource-card.less .resource-list-header) */
.header-action-btn {
  width: auto;
  min-height: 32px;
  padding: 0 12px;
  gap: 6px;
  box-shadow: none;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);

  &:hover:not(:disabled) {
    background: var(--td-bg-color-container-hover);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

.audit-btn {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  color: var(--td-text-color-secondary);
}

/* ---- 右上角类型切换 (间距走 t-button 的 icon/suffix 插槽规则, 这里只管外观) ---- */
.type-switcher {
  border-radius: 6px;
}

.type-switcher-label {
  font-weight: 500;
}

.type-menu {
  width: 280px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  box-sizing: border-box;
}

.type-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s ease;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.is-active {
    background: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
  }
}

.type-menu-icon {
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}

.type-menu-item.is-active .type-menu-icon {
  color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
}

.type-menu-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.type-menu-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  line-height: 18px;
  color: var(--td-text-color-primary);
}

.type-menu-count {
  padding: 0 6px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 11px;
  line-height: 16px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.type-menu-item.is-active .type-menu-count {
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  color: var(--td-brand-color);
}

.type-menu-desc {
  font-size: 12px;
  line-height: 16px;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.type-menu-check {
  flex-shrink: 0;
  color: var(--td-brand-color);
}

.audit-action {
  font-size: 12px;
  font-family: 'SFMono-Regular', Consolas, Menlo, monospace;
  color: var(--td-text-color-secondary);
}

.audit-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.header-title {
  display: flex;
  flex-direction: column;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-title h2 {
  margin: 0;
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

.header-subtitle {
  margin: 4px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.conn-hint {
  margin: 12px 0 4px;
}
</style>
