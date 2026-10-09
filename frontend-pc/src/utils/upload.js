// #2181: PC 上传统一入口。
// 所有上传走 request() 鉴权生命周期（Authorization + 滑窗续期 + 401 重放），
// 杜绝 antd <Upload action> / 裸 XMLHttpRequest 依赖 token cookie 的旧通道。
// 后续上传转换逻辑变更只需改本文件。
import { api, ensureFreshToken, getToken } from '../services/api'

const DEFAULT_ENDPOINT = '/upload'
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function buildFormData(file, name, formData) {
  const fd = new FormData()
  fd.append(name || 'file', file)
  if (formData) {
    for (const [k, v] of Object.entries(formData)) {
      if (v !== undefined && v !== null) fd.append(k, v)
    }
  }
  return fd
}

// 后端上传契约统一为 { code:20000, data:{ url, file_key } }
// （id-photo 类端点返回 data.url，无 file_key）。
function normalize(resp) {
  const data = resp?.data || {}
  return { url: data.url || '', fileKey: data.file_key || data.fileKey || '' }
}

function resolveError(resp) {
  const code = resp?.code
  if (code === 40100 || code === 40101 || code === 40104) {
    return new Error('登录态已失效，请重新登录')
  }
  return new Error(resp?.message || '上传失败')
}

// 无进度需求：走 request()（fetch）完整生命周期。
async function uploadViaRequest(file, { endpoint = DEFAULT_ENDPOINT, name, formData }) {
  const fd = buildFormData(file, name, formData)
  const resp = await api.uploadFile(endpoint, fd)
  if (resp?.code === 20000) return normalize(resp)
  throw resolveError(resp)
}

// 有进度需求：XHR（fetch 无上传进度），鉴权取 ensureFreshToken()，401 刷新一次并重放。
function uploadViaXhr(file, { endpoint = DEFAULT_ENDPOINT, name, formData, onProgress }) {
  return new Promise((resolve, reject) => {
    let retried = false
    const send = async () => {
      let token
      try {
        token = await ensureFreshToken()
      } catch {
        token = getToken()
      }
      const xhr = new XMLHttpRequest()
      xhr.upload.onprogress = (e) => {
        if (onProgress && e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100))
        }
      }
      xhr.onload = () => {
        let resp
        try {
          resp = JSON.parse(xhr.responseText)
        } catch {
          resp = null
        }
        if (xhr.status === 200 && resp?.code === 20000) {
          resolve(normalize(resp))
        } else if (xhr.status === 401 && !retried) {
          // 401 → 刷新一次并重放（避免 cookie 失效后的死路）
          retried = true
          ensureFreshToken().then(send).catch(() => reject(resolveError(resp)))
        } else {
          reject(resolveError(resp))
        }
      }
      xhr.onerror = () => reject(new Error('上传失败：网络错误'))
      xhr.open('POST', `${API_BASE_URL}${endpoint}`)
      if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      xhr.send(buildFormData(file, name, formData))
    }
    send()
  })
}

/**
 * 统一上传入口。
 * @param {File|Blob} file 待上传文件
 * @param {{endpoint?:string, name?:string, formData?:Object, onProgress?:(percent:number)=>void}} opts
 * @returns {Promise<{url:string, fileKey:string}>}
 */
export function uploadToApi(file, opts = {}) {
  if (typeof opts.onProgress === 'function') return uploadViaXhr(file, opts)
  return uploadViaRequest(file, opts)
}

/**
 * antd <Upload> 适配器：返回可直接展开的 props（customRequest），彻底消灭 action=。
 * @param {(result:{url:string, fileKey:string}) => void} onDone
 * @param {{endpoint?:string, name?:string, formData?:Object, onError?:(err:Error)=>void}} opts
 */
export function antdUploadProps(onDone, opts = {}) {
  const { onError, ...uploadOpts } = opts
  return {
    customRequest: async ({ file, onSuccess, onError: antdOnError }) => {
      try {
        const result = await uploadToApi(file, uploadOpts)
        if (onDone) onDone(result)
        if (onSuccess) onSuccess(result)
      } catch (e) {
        if (onError) onError(e)
        if (antdOnError) antdOnError(e)
      }
    },
  }
}
