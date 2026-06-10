export const storage = {
  getItem: (key) => localStorage.getItem(key),
  setItem: (key, value) => localStorage.setItem(key, value),
  removeItem: (key) => localStorage.removeItem(key),
  getJSON: (key, defaultValue = null) => {
    try { return JSON.parse(localStorage.getItem(key)) } catch { return defaultValue }
  },
  setJSON: (key, value) => localStorage.setItem(key, JSON.stringify(value)),
}

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
