export const storage = {
  getItem: (key) => localStorage.getItem(key),
  setItem: (key, value) => localStorage.setItem(key, value),
  removeItem: (key) => localStorage.removeItem(key),
  getJSON: (key, defaultValue = null) => {
    try { return JSON.parse(localStorage.getItem(key)) } catch { return defaultValue }
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

export const request = (url, options = {}) => fetch(url, options)

export const uploadFile = (url, file, options = {}) => {
  const fd = new FormData()
  fd.append('file', file)
  return fetch(url, { method: 'POST', body: fd, ...options })
}

export const dialog = {
  alert: (msg) => window.alert(msg),
  confirm: (msg) => window.confirm(msg),
  toast: (msg) => window.alert(msg),
}

export const navigation = {
  redirect: (url) => { window.location.href = url },
  getCurrentPath: () => window.location.pathname,
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
  iamExternalUrl: import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '',
  isDev: import.meta.env.DEV,
  isWechatBrowser,
  isMiniProgram,
  isWechat: isWechatBrowser || isMiniProgram,
}
