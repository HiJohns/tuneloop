// #2298：zh-CN.json 合并写回（保留既有 key，新增 key；稳定排序）。
export function mergeLocale(existing, keys) {
  const out = { ...(existing && typeof existing === 'object' ? existing : {}) }
  for (const k of keys) {
    if (typeof k === 'string' && k && !(k in out)) out[k] = k
  }
  return Object.fromEntries(Object.keys(out).sort().map((k) => [k, out[k]]))
}
