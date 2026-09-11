// 共享的照片字段解析/渲染工具（#1873）。
// storage 字段为 JSON 数组字符串，元素可能是：
//   - 完整 URL（如 "/uploads/media/xxx.webp" 或 "https://..."）—— #1871 起 /upload 返回的 url
//   - 纯文件 key —— RepairRecordPanel 等按 key 存储
//   - 历史假文件名（"photo_<ts>.jpg"，#1871 之前的 mock 数据，渲染必 404）—— 过滤

const LEGACY_FAKE_PHOTO = /^photo_\d+\.jpg$/i

export function parsePhotos(raw) {
  if (!raw || raw === '[]') return []
  let parsed
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  return parsed.filter(p => typeof p === 'string' && p && !LEGACY_FAKE_PHOTO.test(p))
}

export function photoSrc(p) {
  if (!p) return ''
  if (/^https?:\/\//.test(p) || p.startsWith('/')) return p
  return `/uploads/media/${p}`
}
