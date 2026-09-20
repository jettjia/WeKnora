<template>
  <div class="automations-container">
    <div class="automations-page">
      <div class="header">
        <div class="header-title">
          <div class="title-row">
            <h2><ResourceIcon type="automation" :size="24" /> {{ t('automation.list.title') }}</h2>
            <t-tooltip v-if="canManage" :content="t('automation.list.add')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn" @click="openCreate">
                <template #icon><t-icon name="add" size="16px" /></template>
                {{ t('automation.list.add') }}
              </t-button>
            </t-tooltip>
          </div>
          <p class="header-subtitle">{{ t('automation.list.subtitle') }}</p>
        </div>
      </div>

      <ResourceListToolbar
        v-model="spaceSelection"
        v-model:query="keyword"
        :hide-scopes="authStore.isLiteMode"
        :count-all="automations.length"
        :count-mine="mineCount"
        :count-favorites="favoritesCount"
        :count-recents="recentsCount"
      />

      <div class="automations-main">
      <div v-if="loading" class="card-grid">
        <div v-for="n in 6" :key="'skel-' + n" class="kb-style-card is-skeleton">
          <div class="card-header"><t-skeleton animation="gradient" :row-col="[{ width: '60%', height: '20px' }]" /></div>
          <div class="card-content"><t-skeleton animation="gradient" :row-col="[{ width: '100%', height: '14px' }, { width: '80%', height: '14px' }]" /></div>
          <div class="card-bottom"><t-skeleton animation="gradient" :row-col="[[{ width: '28px', height: '28px', type: 'rect' }, { width: '28px', height: '28px', type: 'rect' }]]" /></div>
        </div>
      </div>
      <div v-else-if="filteredAutomations.length" class="card-grid">
        <div v-for="a in filteredAutomations" :key="a.id" class="kb-style-card" @click="openEdit(a)">
          <button type="button" class="kb-favorite-star" :class="{ 'is-favorited': isFavorited(a) }"
            :aria-label="t('listSpaceSidebar.favorites')" :aria-pressed="isFavorited(a)"
            @click.stop="toggleFavoriteAutomation(a)">
            <t-icon :name="isFavorited(a) ? 'star-filled' : 'star'" size="14px" />
          </button>
          <div class="card-header">
            <span class="card-title" :title="a.title || a.name">
              <span class="card-title-text">{{ a.title || a.name }}</span>
              <span class="card-slug">{{ a.name }}</span>
            </span>
            <t-popup overlay-class-name="card-more-popup" trigger="click" destroy-on-close placement="bottom-right">
              <div class="more-wrap" @click.stop>
                <img class="more-icon" src="@/assets/img/more.png" alt="" />
              </div>
              <template #content>
                  <div class="popup-menu" @click.stop>
                    <div v-if="canManage" class="popup-menu-item" @click.stop="runNow(a)">
                      <t-icon class="menu-icon" name="play-circle" />
                      <span>{{ t('automation.run.runNow') }}</span>
                    </div>
                    <div class="popup-menu-item" @click.stop="openRuns(a)">
                      <t-icon class="menu-icon" name="history" />
                      <span>{{ t('automation.run.history') }}</span>
                    </div>
                    <div v-if="canManage" class="popup-menu-item" @click.stop="openEdit(a)">
                      <t-icon class="menu-icon" name="edit" />
                      <span>{{ t('automation.drawer.edit') }}</span>
                    </div>
                    <div v-if="canManage" class="popup-menu-item delete" @click.stop="confirmDelete(a)">
                      <t-icon class="menu-icon" name="delete" />
                      <span>{{ t('automation.drawer.delete') }}</span>
                    </div>
                  </div>
                </template>
            </t-popup>
          </div>

          <div class="card-content">
            <div class="card-description" :title="a.description || a.query_template">{{ a.description || a.query_template }}</div>
          </div>

          <div class="card-bottom">
            <div class="bottom-left">
              <t-switch :value="a.enabled" size="small" @click.stop @change="(v: unknown) => toggleEnabled(a, !!v)" />
              <div class="feature-badge status-badge" :class="lastStatusClass(a)">
                <t-icon :name="lastStatusIcon(a)" size="14px" />
                <span class="badge-text">{{ lastStatusLabel(a) }}</span>
              </div>
            </div>
            <div class="bottom-right">
              <span class="card-time">{{ nextRunLabel(a) }}</span>
            </div>
          </div>
        </div>
      </div>

      <EmptyState v-else-if="keyword.trim()" icon="search" :title="t('common.noResult')">
        <t-button variant="outline" @click="keyword = ''">{{ t('common.clear') }}</t-button>
      </EmptyState>
      <EmptyState v-else :image="uploadImg" :title="emptyText">
        <t-button v-if="canManage && spaceSelection === 'all'" class="empty-state-btn" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('automation.list.add') }}
        </t-button>
      </EmptyState>
      </div>

      <AutomationDrawer ref="drawerRef" :agents="agentOptions" @saved="reload" />
      <RunsDrawer v-model:visible="runsVisible" :automation="runsTarget" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { listAutomations, deleteAutomation, updateAutomation, runAutomationNow, type Automation } from '@/automation/api'
import { listAgents } from '@/api/agent'
import AutomationDrawer from '@/automation/AutomationDrawer.vue'
import RunsDrawer from '@/automation/RunsDrawer.vue'
import EmptyState from '@/components/EmptyState.vue'
import ResourceIcon from '@/components/icons/ResourceIcon.vue'
import ResourceListToolbar from '@/components/ResourceListToolbar.vue'
import uploadImg from '@/assets/img/upload.svg'
import { useResourcePins } from '@/composables/useResourcePins'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()

const automations = ref<Automation[]>([])
const loading = ref(true)
const agentOptions = ref<{ id: string; name: string }[]>([])
const drawerRef = ref<InstanceType<typeof AutomationDrawer> | null>(null)
const runsVisible = ref(false)
const runsTarget = ref<Automation | null>(null)

// 收藏 (服务端) + 最近 (本地), 类型 automation — 与知识库/智能体同款机制
const pins = useResourcePins()
const spaceSelection = ref('all')
const keyword = ref('')

const canManage = computed(() => authStore.hasRole('contributor'))

const favoritesCount = computed(() => pins.favorites.value.filter(e => e.type === 'automation').length)
const recentsCount = computed(() => pins.recents.value.filter(e => e.type === 'automation').length)
const mineCount = computed(() => automations.value.filter(a => a.created_by === authStore.currentUserId).length)
const isFavorited = (a: Automation) => pins.isFavorite('automation', a.id)

async function toggleFavoriteAutomation(a: Automation) {
  try {
    await pins.toggleFavorite('automation', a.id)
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'favorite failed')
  }
}

// 顶部工具栏视图过滤: 全部 / 收藏 / 最近 / 本空间(我创建的)
const selectionFiltered = computed(() => {
  const sel = spaceSelection.value
  if (sel === 'favorites') {
    const ids = new Set(pins.favorites.value.filter(e => e.type === 'automation').map(e => e.id))
    return automations.value.filter(a => ids.has(a.id))
  }
  if (sel === 'recents') {
    const ordered = pins.recents.value.filter(e => e.type === 'automation').map(e => e.id)
    const byId = new Map(automations.value.map(a => [a.id, a]))
    return ordered.map(id => byId.get(id)).filter(Boolean) as Automation[]
  }
  if (sel === 'mine') {
    return automations.value.filter(a => a.created_by === authStore.currentUserId)
  }
  return automations.value
})

function applyKeyword(list: Automation[]) {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter(a => `${a.title || ''} ${a.name} ${a.description || ''}`.toLowerCase().includes(kw))
}
const filteredAutomations = computed(() => applyKeyword(selectionFiltered.value))

const emptyText = computed(() => {
  if (spaceSelection.value === 'favorites') return t('automation.list.emptyFavorites')
  if (spaceSelection.value === 'recents') return t('automation.list.emptyRecents')
  if (spaceSelection.value === 'mine') return t('automation.list.emptyMine')
  return t('automation.list.empty')
})

async function reload() {
  loading.value = true
  try {
    const resp = await listAutomations()
    automations.value = resp.automations || []
  } catch {
    automations.value = []
  } finally {
    loading.value = false
  }
}

async function loadAgents() {
  try {
    const resp = await listAgents()
    agentOptions.value = (resp.data || [])
      .filter((a: any) => !a.is_builtin)
      .map((a: any) => ({ id: a.id, name: a.name }))
  } catch {
    agentOptions.value = []
  }
}

function openCreate() {
  drawerRef.value?.openCreate()
}
function openEdit(a: Automation) {
  pins.touchRecent('automation', a.id)
  if (!canManage.value) return
  drawerRef.value?.openEdit(a)
}
function openRuns(a: Automation) {
  runsTarget.value = a
  runsVisible.value = true
}

async function toggleEnabled(a: Automation, enabled: boolean) {
  try {
    await updateAutomation(a.id, { enabled })
    a.enabled = enabled
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'update failed')
  }
}

async function runNow(a: Automation) {
  try {
    await runAutomationNow(a.id)
    MessagePlugin.success(t('automation.run.runQueued'))
    setTimeout(reload, 1500)
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'run failed')
  }
}

function confirmDelete(a: Automation) {
  const dlg = DialogPlugin.confirm({
    header: t('automation.drawer.delete'),
    body: `${a.title || a.name} — ${t('automation.list.confirmDelete')}`,
    theme: 'warning',
    onConfirm: async () => {
      try {
        await deleteAutomation(a.id)
        automations.value = automations.value.filter(x => x.id !== a.id)
        MessagePlugin.success(t('automation.drawer.deleted'))
      } catch (e: any) {
        MessagePlugin.error(e?.response?.data?.error || 'delete failed')
      }
      dlg.destroy()
    },
  })
}

function lastStatusClass(a: Automation): string {
  if (!a.last_run) return 'idle'
  const s = a.last_run.status
  if (s === 'success') return 'success'
  if (s === 'running' || s === 'pending') return 'running'
  return 'error'
}
function lastStatusIcon(a: Automation): string {
  const s = a.last_run?.status
  if (s === 'success') return 'check-circle'
  if (s === 'running' || s === 'pending') return 'loading'
  if (!s) return 'circle'
  return 'error-circle'
}
function lastStatusLabel(a: Automation): string {
  if (!a.last_run) return t('automation.list.never')
  return t(`automation.run.status.${a.last_run.status}`)
}
function nextRunLabel(a: Automation): string {
  if (a.enabled && a.next_runs?.length) {
    return `${t('automation.list.nextRun')} ${String(a.next_runs[0]).slice(0, 16).replace('T', ' ')}`
  }
  return ''
}

onMounted(() => {
  reload()
  loadAgents()
})
</script>

<style scoped lang="less">
@import (reference) '@/components/css/resource-card.less';
.automations-container {
  height: 100%;
  display: flex;
  min-width: 0;
  min-height: 0;
  box-sizing: border-box;
}
.automations-page {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 20px 28px 0;
  box-sizing: border-box;
}
/* Header and toolbar stay fixed; only the list scrolls (KB/agent pages). */
.automations-main {
  .resource-list-main();
}
/* 顶层调用: mixin 自带 .header 选择器, 嵌在 .header{} 里会展开成无效的 .header .header,
   导致新建按钮丢失推右与胶囊样式 (与知识库页头不一致的根因) */
.resource-list-header();
.header-title h2 {
  margin: 0;
  font-size: var(--app-text-4xl);
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.header-subtitle {
  margin: 4px 0 0;
  font-size: var(--app-text-md);
  color: var(--td-text-color-secondary);
}
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
  flex-shrink: 0;
}
.card-time {
  font-size: var(--app-text-sm);
  color: var(--td-text-color-placeholder);
}
.feature-badge {
  display: flex;
  align-items: center;
  gap: 3px;
  height: 22px;
  border-radius: var(--app-radius-sm);
  padding: 0 6px;
  font-size: var(--app-text-xs);
  font-weight: 500;
}
.feature-badge.status-badge.success {
  background: color-mix(in srgb, var(--td-brand-color) 8%, transparent);
  color: var(--td-brand-color-active);
}
.feature-badge.status-badge.running {
  background: rgba(0, 82, 217, 0.08);
  color: var(--td-brand-color);
}
.feature-badge.status-badge.error,
.feature-badge.status-badge.idle {
  background: rgba(227, 77, 89, 0.08);
  color: var(--td-error-color);
}
.feature-badge.status-badge.idle {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-placeholder);
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
  font-size: var(--app-text-md);
}
.popup-menu-item:hover {
  background: var(--td-bg-color-container-hover);
}
.popup-menu-item.delete {
  color: var(--td-error-color);
}
.menu-icon {
  font-size: var(--app-text-xl);
}
.empty-state .empty-img {
  width: 120px;
  opacity: 0.8;
}
.empty-state .empty-txt {
  font-size: var(--app-text-lg);
  font-weight: 500;
  color: var(--td-text-color-primary);
}
.empty-state-btn {
  width: fit-content;
}
</style>
