// 自动化模块 API 客户端 (后端 /api/v1/automations/*)。
import { get, post, put, del } from '@/utils/request'

export interface RunSummary {
  id: string
  status: string
  trigger: string
  started_at?: string | null
  duration_ms: number
}

export interface NextRun {
  [k: string]: string
}

export interface Automation {
  id: string
  tenant_id: number
  name: string
  title: string
  description: string
  agent_id: string
  query_template: string
  schedule_cron: string
  schedule_tz: string
  enabled: boolean
  overlap_policy: string
  timeout_minutes: number
  created_by: string
  created_at: string
  updated_at: string
  next_runs?: string[]
  last_run?: RunSummary | null
}

export interface AutomationInput {
  name?: string
  title?: string
  description?: string
  agent_id?: string
  query_template?: string
  schedule_cron?: string
  schedule_tz?: string
  enabled?: boolean
  overlap_policy?: string
  timeout_minutes?: number
}

export interface AutomationRun {
  id: string
  tenant_id: number
  automation_id: string
  status: string
  trigger_type: string
  session_id: string
  output_summary: string
  error: string
  started_at?: string | null
  finished_at?: string | null
  duration_ms: number
  created_at: string
}

export function listAutomations() {
  return get<{ automations: Automation[] }>('/api/v1/automations')
}

export function getAutomation(id: string) {
  return get<Automation>(`/api/v1/automations/${id}`)
}

export function createAutomation(data: AutomationInput) {
  return post<Automation>('/api/v1/automations', data)
}

export function updateAutomation(id: string, data: AutomationInput) {
  return put<Automation>(`/api/v1/automations/${id}`, data)
}

export function deleteAutomation(id: string) {
  return del(`/api/v1/automations/${id}`)
}

export function runAutomationNow(id: string) {
  return post<{ enqueued: boolean }>(`/api/v1/automations/${id}/run`, {})
}

export function listAutomationRuns(id: string, limit = 50) {
  return get<{ runs: AutomationRun[] }>(`/api/v1/automations/${id}/runs?limit=${limit}`)
}
