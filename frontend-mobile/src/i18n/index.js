// i18n 核心（#2213）：**源中文串即 key**；缺失回退原串；`{name}` 具名插值；当前仅 zh-CN。
// 设计：docs/topics/i18n-strategy.md §3。安全性质：字典为空时 t(K) === K（恒等），
// 因此机械改写（含 `'前缀: '+x` → `t('前缀: {msg}',{msg})`）零行为变化。
import zhCN from './locales/zh-CN.json'

export const I18N_LOCALE = 'zh-CN'

const dict = zhCN || {}

/**
 * 取文案。key 为中文源串（清单键）；缺失时回退 key 本身；params 做 `{name}` 插值。
 * @param {string} key
 * @param {Record<string, unknown>} [params]
 * @returns {string}
 */
export function t(key, params) {
  if (key == null) return ''
  let s = typeof key === 'string' ? (dict[key] !== undefined ? dict[key] : key) : String(key)
  if (params) {
    s = s.replace(/\{(\w+)\}/g, (m, name) => (params[name] != null ? String(params[name]) : m))
  }
  return s
}

export default t
