// 数据建模模块自带的 i18n 消息。启动时通过 mergeLocaleMessage 合并进主
// i18n 实例 (见 index.ts), 上游 5 个 locale 文件保持零改动。
// 五种语言全部翻译, key 集合一致性由 messages.test.ts 保证 (对齐上游
// localeKeyAudit 的 parity 口径)。
import i18n from '@/i18n'
import { zhCN, enUS, ruRU, jaJP, koKR } from './messages'

const MESSAGES: Record<string, Record<string, unknown>> = {
  'zh-CN': { semantic: zhCN, menu: { semantic: zhCN.menu } },
  'en-US': { semantic: enUS, menu: { semantic: enUS.menu } },
  'ru-RU': { semantic: ruRU, menu: { semantic: ruRU.menu } },
  'ja-JP': { semantic: jaJP, menu: { semantic: jaJP.menu } },
  'ko-KR': { semantic: koKR, menu: { semantic: koKR.menu } }
}

export function mergeLocaleMessage() {
  for (const [locale, messages] of Object.entries(MESSAGES)) {
    i18n.global.mergeLocaleMessage(locale, messages)
  }
}
