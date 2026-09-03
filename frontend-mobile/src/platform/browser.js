export const storage = {
  getItem: (key) => localStorage.getItem(key),
  setItem: (key, value) => localStorage.setItem(key, value),
  removeItem: (key) => localStorage.removeItem(key),
  getJSON: (key, defaultValue = null) => {
    try {
      const raw = localStorage.getItem(key)
      if (raw === null || raw === '') return defaultValue
      const parsed = JSON.parse(raw)
      return parsed === null ? defaultValue : parsed
    } catch { return defaultValue }
  },
  setJSON: (key, value) => localStorage.setItem(key, JSON.stringify(value)),
}

export const getWindowSize = () => ({ width: window.innerWidth || 375, height: window.innerHeight || 667 })

export const session = {
  getItem: (key) => sessionStorage.getItem(key),
  setItem: (key, value) => sessionStorage.setItem(key, value),
  removeItem: (key) => sessionStorage.removeItem(key),
}

export const cookie = {
  get: (name) => {
    const cookies = document.cookie.split(';')
    for (const cookie of cookies) {
      const [n, ...rest] = cookie.trim().split('=')
      if (n === name) return rest.join('=')
    }
    return null
  },
  set: (name, value, options = {}) => {
    let cookie = `${name}=${value}; path=${options.path || '/'}`
    if (options.expires) cookie += `; expires=${options.expires}`
    document.cookie = cookie
  },
  remove: (name) => {
    document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;`
  },
}

// #1785: 15s request timeout — abort hangs so payment screens never spin forever.
const REQUEST_TIMEOUT_MS = 15000

export const request = (url, options = {}) => {
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  return fetch(url, { ...options, signal: controller.signal })
    .catch((err) => {
      if (err && err.name === 'AbortError') {
        // #1785: surface a readable message instead of a bare DOMException.
        throw new Error('请求超时，请重试')
      }
      throw err
    })
    .finally(() => clearTimeout(timeoutId))
}

export const uploadFile = (url, file, options = {}) => {
  const fd = new FormData()
  fd.append(options.name || 'file', file)
  if (options.formData) {
    for (const [k, v] of Object.entries(options.formData)) {
      fd.append(k, v)
    }
  }
  const { formData, name, ...rest } = options
  return fetch(url, { method: 'POST', body: fd, headers: rest.headers, ...rest })
}

export const dialog = {
  alert: (msg) => window.alert(msg),
  confirm: (msg) => window.confirm(msg),
  toast: (msg) => window.alert(msg),
}

// H5: native <input> fires React synthetic event with e.target.value.
export const getInputValue = (e) => e.target?.value ?? ''

export const navigation = {
  redirect: (url) => { window.location.href = url },
  getCurrentPath: () => window.location.pathname + window.location.search,
  getOrigin: () => window.location.origin,
  getQueryParams: () => Object.fromEntries(new URLSearchParams(window.location.search)),
}

export const eventBus = {
  on: (event, handler) => window.addEventListener(event, handler),
  off: (event, handler) => window.removeEventListener(event, handler),
  emit: (event) => window.dispatchEvent(new Event(event)),
}

export const phone = {
  call: (number) => { window.location.href = `tel:${number}` },
}

export const openLink = (url) => { window.open(url, '_blank') }

export const scanQRCode = () => new Promise((resolve, reject) => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.capture = 'environment'
  input.onchange = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    try {
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')
      const img = new Image()
      img.onload = async () => {
        canvas.width = img.width
        canvas.height = img.height
        ctx.drawImage(img, 0, 0)
        try {
          const blob = await new Promise(r => canvas.toBlob(r, 'image/png'))
          const bitmap = await createImageBitmap(blob)
          const detector = new BarcodeDetector({ formats: ['qr_code'] })
          const codes = await detector.detect(bitmap)
          resolve(codes[0].rawValue)
        } catch { reject(new Error('未识别到二维码')) }
      }
      img.src = URL.createObjectURL(file)
    } catch { reject(new Error('扫码功能不可用')) }
  }
  input.click()
})

export const previewImage = ({ urls = [], current = '' }) => {
  const container = document.createElement('div')
  container.style.cssText = 'position:fixed;inset:0;z-index:9999;background:#000;overflow:auto;-webkit-overflow-scrolling:touch'
  const closeBtn = document.createElement('div')
  closeBtn.textContent = '✕'
  closeBtn.style.cssText = 'position:fixed;top:20px;right:20px;z-index:10000;color:#fff;font-size:28px;cursor:pointer;width:40px;height:40px;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.5);border-radius:50%'
  const img = document.createElement('img')
  img.src = current
  img.style.cssText = 'display:block;max-width:none;max-height:none'
  // Keep original size but make viewport scrollable
  img.onload = () => {
    img.style.width = img.naturalWidth + 'px'
    img.style.height = img.naturalHeight + 'px'
  }
  closeBtn.onclick = () => document.body.removeChild(container)
  container.appendChild(img)
  container.appendChild(closeBtn)
  document.body.appendChild(container)
}

export const getLocation = () => new Promise((resolve, reject) => {
  if (!navigator.geolocation) {
    reject(new Error('Geolocation not supported'))
    return
  }
  navigator.geolocation.getCurrentPosition(
    (pos) => resolve({
      latitude: pos.coords.latitude,
      longitude: pos.coords.longitude,
    }),
    (err) => reject(err),
    { enableHighAccuracy: false, timeout: 10000 }
  )
})

export const onPageScroll = (handler) => {
  window.addEventListener('scroll', handler)
  return () => window.removeEventListener('scroll', handler)
}

const isWechatBrowser = typeof window !== 'undefined' && /micromessenger/i.test(navigator.userAgent)
const isMiniProgram = typeof window !== 'undefined' && window.__wxjs_environment === 'miniprogram'
export const env = {
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || '/api',
  version: import.meta.env.VITE_APP_VERSION || '',
  iamExternalUrl: import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '',
  isDev: import.meta.env.DEV,
  isWechatBrowser,
  isMiniProgram,
  isWechat: isWechatBrowser || isMiniProgram,
}
// H5: no Camera component — return a stub that rejects all operations.
export const getCameraContext = () => ({
  takePhoto: (opts) => opts?.fail?.({ errMsg: 'H5 不支持摄像头' }) || opts?.complete?.({}),
  startRecord: (opts) => opts?.fail?.({ errMsg: 'H5 不支持录像' }) || opts?.complete?.({}),
  stopRecord: (opts) => opts?.fail?.({ errMsg: 'H5 不支持录像' }) || opts?.complete?.({}),
})

export const wxLogin = () => Promise.resolve('')
export const getPhoneNumber = () => ({ encryptedData: '', iv: '' })

export { toWeappRoute } from './navigation'
