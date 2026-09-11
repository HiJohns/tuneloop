// 共享的照片字段解析/渲染工具（#1873）。
// storage 字段为 JSON 数组字符串，元素可能是：
//   - 完整 URL（如 "/uploads/media/xxx.webp" 或 "https://..."）—— #1871 起 /upload 返回的 url
//   - 纯文件 key —— RepairRecordPanel 等按 key 存储
//   - 历史假文件名（"photo_<ts>.jpg"，#1871 之前的 mock 数据，渲染必 404）—— 过滤

import { env } from '../platform'

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

// #1876: 相对路径 /uploads/... 必须补全 origin 后交给 weapp <Image>
// （小程序 Image 不支持相对路径，静默失败；与 Cart/OrderDetail 的 fixImg 同语义）。
export function photoSrc(p) {
  if (!p) return ''
  if (/^https?:\/\//.test(p) || p.startsWith('data:')) return p
  const path = p.startsWith('/') ? p : `/uploads/media/${p}`
  return env.apiBaseUrl.replace(/\/api$/, '') + path
}
