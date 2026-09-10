// Key-parity test for the semantic module's five locale bundles, mirroring
// the upstream localeKeyAudit contract ("locale bundles expose the same
// translation keys") for module-registered messages that the upstream audit
// cannot see (they are merged at runtime via mergeLocaleMessage).
import { test } from 'node:test'
import assert from 'node:assert/strict'

import { zhCN, enUS, ruRU, jaJP, koKR } from './messages.ts'

type Bundle = Record<string, unknown>

const BUNDLES: Record<string, Bundle> = {
  'zh-CN': zhCN,
  'en-US': enUS,
  'ru-RU': ruRU,
  'ja-JP': jaJP,
  'ko-KR': koKR,
}

function flatten(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) =>
    flatten(v, prefix ? `${prefix}.${k}` : k),
  )
}

function placeholders(s: string): string[] {
  return (s.match(/\{[a-zA-Z]+\}/g) ?? []).sort()
}

function collectPlaceholders(obj: unknown, prefix = ''): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  if (obj === null || typeof obj !== 'object') return out
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out[key] = placeholders(v)
    else Object.assign(out, collectPlaceholders(v, key))
  }
  return out
}

test('semantic locale bundles expose the same translation keys', () => {
  const locales = Object.keys(BUNDLES)
  const [reference, ...rest] = locales
  const referenceKeys = flatten(BUNDLES[reference]).sort()

  for (const locale of rest) {
    const keys = flatten(BUNDLES[locale]).sort()
    assert.deepEqual(
      keys,
      referenceKeys,
      `${locale} key set must match ${reference}`,
    )
  }
})

test('semantic locale placeholders match across locales', () => {
  const locales = Object.keys(BUNDLES)
  const [reference, ...rest] = locales
  const referencePh = collectPlaceholders(BUNDLES[reference])

  for (const locale of rest) {
    const ph = collectPlaceholders(BUNDLES[locale])
    for (const [key, expected] of Object.entries(referencePh)) {
      assert.deepEqual(
        ph[key],
        expected,
        `${locale} placeholder mismatch at ${key}`,
      )
    }
  }
})
