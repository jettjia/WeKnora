<template>
  <t-dialog
    v-model:visible="visible"
    :header="`${t('semantic.model.share')} · ${target?.title || target?.name || ''}`"
    width="520px"
    :footer="false"
  >
    <div v-if="target?.status !== 'published'" class="share-hint">
      {{ t('semantic.model.shareNeedPublish') }}
    </div>
    <template v-else>
      <div class="share-row">
        <t-select v-model="selectedOrg" filterable :placeholder="t('semantic.model.shareSelectOrg')" style="flex: 1">
          <t-option v-for="o in orgs" :key="o.id" :value="o.id" :label="o.name" />
        </t-select>
        <t-button theme="primary" :loading="sharing" :disabled="!selectedOrg" @click="share">
          {{ t('semantic.model.shareConfirm') }}
        </t-button>
      </div>

      <div v-if="shares.length" class="share-list">
        <div v-for="sh in shares" :key="sh.id" class="share-item">
          <span class="share-org">{{ orgName(sh.organization_id) }}</span>
          <span class="share-time">{{ fmtTime(sh.created_at) }}</span>
          <t-button variant="text" theme="danger" size="small" @click="unshare(sh)">
            {{ t('semantic.model.shareRemove') }}
          </t-button>
        </div>
      </div>
      <t-empty v-else size="small" :description="t('semantic.model.shareNone')" />
    </template>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { shareModel, unshareModel, listModelShares, type SemanticModel, type ModelShare } from './api'

const props = defineProps<{ orgs: { id: string; name: string }[] }>()
const { t } = useI18n()

const visible = ref(false)
const target = ref<SemanticModel | null>(null)
const shares = ref<ModelShare[]>([])
const selectedOrg = ref('')
const sharing = ref(false)

const orgName = (id: string) => props.orgs.find(o => o.id === id)?.name || id

function fmtTime(ts: string): string {
  return ts ? String(ts).slice(0, 16).replace('T', ' ') : ''
}

async function open(m: SemanticModel) {
  target.value = m
  selectedOrg.value = ''
  visible.value = true
  try {
    const resp = await listModelShares(m.id)
    shares.value = resp.shares || []
  } catch {
    shares.value = []
  }
}

async function share() {
  if (!target.value || !selectedOrg.value) return
  sharing.value = true
  try {
    await shareModel(target.value.id, selectedOrg.value)
    const resp = await listModelShares(target.value.id)
    shares.value = resp.shares || []
    MessagePlugin.success(t('semantic.model.shareDone'))
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'share failed')
  } finally {
    sharing.value = false
  }
}

async function unshare(sh: ModelShare) {
  if (!target.value) return
  try {
    await unshareModel(target.value.id, sh.organization_id)
    shares.value = shares.value.filter(x => x.id !== sh.id)
  } catch (e: any) {
    MessagePlugin.error(e?.response?.data?.error || 'unshare failed')
  }
}

defineExpose({ open })
</script>

<style scoped>
.share-row {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.share-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.share-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  font-size: 13px;
}
.share-org {
  flex: 1;
}
.share-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.share-hint {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
</style>
