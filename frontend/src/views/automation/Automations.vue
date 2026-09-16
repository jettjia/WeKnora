<template>
  <div class="automations-container">
    <ListSpaceSidebar
      v-if="!authStore.isLiteMode"
      v-model="spaceSelection"
      :count-all="automations.length"
      :count-favorites="favoritesCount"
      :count-recents="recentsCount"
      :count-mine="mineCount"
      show-favorites
      show-recents
    />
    <div class="automations-page">
      <div class="header">
        <div class="header-title">
          <div class="title-row">
            <h2>{{ t('automation.list.title') }}</h2>
            <t-tooltip v-if="canManage" :content="t('automation.list.add')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn" @click="openCreate">
                <template #icon><t-icon name="add" size="16px" /></template>
              </t-button>
            </t-tooltip>
          </div>
          <p class="header-subtitle">{{ t('automation.list.subtitle') }}</p>
        </div>
      </div>

      <div v-if="filteredAutomations.length" class="card-grid">
        <div v-for="a in filteredAutomations" :key="a.id" class="kb-style-card" @click="openEdit(a)">
          <div class="card-header">
            <span class="card-title" :title="a.name">
              <button type="button" class="kb-favorite-star" :class="{ 'is-favorited': isFavorited(a) }"
                @click.stop="toggleFavoriteAutomation(a)">
                <svg viewBox="0 0 24 24" width="16" height="16">
                  <path :fill="isFavorited(a) ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="1.8"
                    stroke-linejoin="round"
                    d="M12 3.6l2.6 5.3 5.8.8-4.2 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8-4.2-4.1 5.8-.8z" />
                </svg>
              </button>
              <span class="card-title-text">{{ a.title || a.name }}</span>
              <span class="card-slug">{{ a.name }}</span>
            </span>
            <div class="header-actions">
              <t-switch :value="a.enabled" size="small" @click.stop @change="(v: unknown) => toggleEnabled(a, !!v)" />
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
          </div>

          <div class="card-content">
            <div class="card-description">{{ a.description || a.query_template }}</div>
          </div>

          <div class="card-bottom">
            <div class="bottom-left">
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

      <div v-else class="empty-state">
        <img class="empty-img" src="@/assets/img/upload.svg" alt="" />
        <span class="empty-txt">{{ emptyText }}</span>
        <t-button v-if="canManage && spaceSelection === 'all'" class="empty-state-btn" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('automation.list.add') }}
        </t-button>
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
import ListSpaceSidebar from '@/components/ListSpaceSidebar.vue'
import { useResourcePins } from '@/composables/useResourcePins'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()

const automations = ref<Automation[]>([])
const agentOptions = ref<{ id: string; name: string }[]>([])
const drawerRef = ref<InstanceType<typeof AutomationDrawer> | null>(null)
const runsVisible = ref(false)
const runsTarget = ref<Automation | null>(null)

// 收藏 (服务端) + 最近 (本地), 类型 automation — 与知识库/智能体同款机制
const pins = useResourcePins()
const spaceSelection = ref('all')

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

// 侧栏视图过滤: 全部 / 收藏 / 最近 / 本空间(我创建的)
const filteredAutomations = computed(() => {
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

const emptyText = computed(() => {
  if (spaceSelection.value === 'favorites') return t('automation.list.emptyFavorites')
  if (spaceSelection.value === 'recents') return t('automation.list.emptyRecents')
  if (spaceSelection.value === 'mine') return t('automation.list.emptyMine')
  return t('automation.list.empty')
})

async function reload() {
  try {
    const resp = await listAutomations()
    automations.value = resp.automations || []
  } catch {
    automations.value = []
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

<style scoped>
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
  padding: 20px 28px;
  box-sizing: border-box;
  overflow-y: auto;
}
.header {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
}
.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
/* Same square icon button as the knowledge-base/agent/org list headers. */
.header-action-btn {
  padding: 0 !important;
  min-width: 28px !important;
  width: 28px !important;
  height: 28px !important;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  background: var(--td-bg-color-secondarycontainer) !important;
  border: 1px solid var(--td-component-stroke) !important;
  border-radius: 6px !important;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--td-bg-color-container) 72%, transparent);
  transition: background 0.2s, border-color 0.2s, color 0.2s;
}
.header-action-btn:hover {
  background: var(--td-bg-color-secondarycontainer) !important;
  border-color: var(--td-component-stroke) !important;
  color: var(--td-text-color-primary);
}
.header-action-btn :deep(.t-icon) {
  color: var(--td-brand-color);
}
.header-title h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.header-subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}
@media (min-width: 1250px) {
  .card-grid { grid-template-columns: repeat(3, 1fr); }
}
@media (min-width: 1600px) {
  .card-grid { grid-template-columns: repeat(4, 1fr); }
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
.card-header, .card-content, .card-bottom {
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
.kb-favorite-star {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  align-self: center;
  transition: color 0.2s ease, transform 0.15s ease;
}
.kb-favorite-star:hover {
  color: #f5a623;
  transform: scale(1.1);
}
.kb-favorite-star.is-favorited {
  color: #f5a623;
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
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
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
.more-icon {
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
  flex-shrink: 0;
}
.card-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.feature-badge {
  display: flex;
  align-items: center;
  gap: 3px;
  height: 22px;
  border-radius: 5px;
  padding: 0 6px;
  font-size: 11px;
  font-weight: 500;
}
.feature-badge.status-badge.success {
  background: rgba(7, 192, 95, 0.08);
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
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 64px 0;
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
</style>
