// Deposit-free (#1867) shared helpers — eligibility fetch, reason copy,
// recommendation-letter template download and photo upload. Used by the
// H5 (pages/Checkout.jsx) and weapp (pages-weapp/Checkout.jsx) checkouts.
import Taro from '@tarojs/taro'
import { apiFetch, getToken } from '../services/api'
import { dialog, env, uploadFile } from '../platform'

// GET /user/deposit-waiver/eligibility → { eligible, reasons[], ... }.
// Network/parse failures degrade to ineligible with a load_failed reason
// so the checkout never offers a waiver it cannot verify.
export async function fetchWaiverEligibility(baseUrl) {
  try {
    const res = await apiFetch(`${baseUrl}/user/deposit-waiver/eligibility`)
    const r = await res.json()
    if (r.code === 20000 && r.data) return r.data
    return { eligible: false, reasons: ['load_failed'] }
  } catch {
    return { eligible: false, reasons: ['load_failed'] }
  }
}

const REASON_COPY = {
  face_not_verified: '需先完成实名认证',
  identity_not_student_or_teacher: '免押金仅限学生／教职工申请',
  credit_below_threshold: '信用分未达到免押门槛',
  profile_not_ready: '请先完善个人资料',
  user_not_found: '请先完善个人资料',
  load_failed: '资格校验失败，请稍后重试',
}

export function waiverReasonText(reasons) {
  if (!Array.isArray(reasons) || reasons.length === 0) return '暂不符合免押金申请条件'
  return reasons.map(r => REASON_COPY[r] || r).join('；')
}

// Template PDF ships with the mobile static build (public/), served at the
// mobile origin — same origin derivation as content image normalization.
export function waiverTemplateUrl(baseUrl) {
  const origin = (baseUrl || '').replace(/\/api\/?$/, '')
  return `${origin}/deposit-waiver-recommendation-template.pdf`
}

// H5 opens the PDF in a new tab; weapp downloads + opens it, falling back
// to copying the link when downloadFile fails (e.g. domain not whitelisted).
export async function downloadLetterTemplate(baseUrl) {
  const url = waiverTemplateUrl(baseUrl)
  if (!env.isMiniProgram) {
    window.open(url, '_blank')
    return
  }
  try {
    const dl = await Taro.downloadFile({ url })
    if (dl.statusCode === 200 && dl.tempFilePath) {
      await Taro.openDocument({ filePath: dl.tempFilePath, fileType: 'pdf', showMenu: true })
      return
    }
    throw new Error(`download failed: ${dl.statusCode}`)
  } catch (e) {
    Taro.setClipboardData({
      data: url,
      success: () => dialog.toast('已复制模板链接，请在浏览器打开下载'),
    })
  }
}

// Upload the signed-letter photo via POST /upload (images only — the
// endpoint converts to WebP and returns { data: { url } }).
export async function uploadLetterPhoto(baseUrl, fileOrPath) {
  if (!fileOrPath) return { ok: false, error: '未选择图片' }
  try {
    const res = await uploadFile(`${baseUrl}/upload`, fileOrPath, {
      name: 'file',
      headers: { Authorization: 'Bearer ' + getToken() },
    })
    let body = res
    if (!env.isMiniProgram) body = await res.json()
    else if (typeof body.data === 'string') body = JSON.parse(body.data)
    if (body.code === 20000 && body.data?.url) {
      return { ok: true, url: body.data.url }
    }
    return { ok: false, error: body.message || '上传失败' }
  } catch (err) {
    return { ok: false, error: err?.message || '网络错误' }
  }
}
