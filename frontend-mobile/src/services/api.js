const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

export const publicRoutes = ['/', '/instrument', '/cart', '/success', '/callback']

function isPublicRoute() {
  const path = window.location.pathname
  return publicRoutes.some(p => path === p || path.startsWith(p + '/'))
}

// 开发环境注入 mock wx 对象，防止浏览器调试时崩溃
if (import.meta.env.DEV && typeof window !== 'undefined' && typeof window.wx === 'undefined') {
  console.log('[Dev Mode] Injecting mock wx object for browser debugging')
  window.wx = {
    miniProgram: {
      redirectTo: (options) => {
        console.log('[Mock wx.miniProgram] redirectTo:', options)
        // Fallback to window.location in dev mode
        window.location.href = options.url || '/login'
      },
      navigateTo: (options) => {
        console.log('[Mock wx.miniProgram] navigateTo:', options)
      },
      switchTab: (options) => {
        console.log('[Mock wx.miniProgram] switchTab:', options)
      }
    },
    scanQRCode: (options) => {
      console.log('[Mock wx] scanQRCode:', options)
      // Mock: simulate scanning a QR code after 1 second
      setTimeout(() => {
        if (options.success) {
          options.success({ resultStr: 'mock_qr_code_12345' })
        }
      }, 1000)
    },
    openLocation: (options) => {
      console.log('[Mock wx] openLocation:', options)
      window.open(`https://map.baidu.com/search/${options.latitude},${options.longitude}`, '_blank')
    },
    getLocation: (options) => {
      console.log('[Mock wx] getLocation:', options)
      // Mock: return a default location (Beijing)
      if (options.success) {
        options.success({
          latitude: 39.9042,
          longitude: 116.4074
        })
      }
    },
    login: (options) => {
      console.log('[Mock wx] login:', options)
      if (options.success) {
        options.success({ code: 'mock_login_code_12345' })
      }
    }
  }
}

/**
 * 检测是否在微信小程序环境中
 * 只使用 __wxjs_environment（微信官方推荐）
 * 这是唯一可靠的方式来区分真实微信环境和浏览器环境
 */
function isWeChatMiniProgram() {
  // 只检测 window.__wxjs_environment（微信官方推荐）
  // Mock 的 wx 对象不会设置 __wxjs_environment，因此不会误判
  return typeof window !== 'undefined' && 
         window.__wxjs_environment === 'miniprogram'
}

/**
 * 统一的登录重定向函数
 */
export function redirectToLogin(reason) {
  if (reason) {
    sessionStorage.setItem('login_reason', reason)
  }

  if (reason && reason !== 'session_expired' && reason !== 'token_missing') {
    if (!window.confirm('此功能需要登录，是否前往登录？')) return
  }

  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  localStorage.removeItem('user_sys_perm')
  localStorage.removeItem('user_cus_perm')
  localStorage.removeItem('user_cus_perm_ext')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'

  if (isWeChatMiniProgram()) {
    wx.miniProgram.redirectTo({
      url: '/pages/login/login'
    })
  } else {
    const wxConfig = window.APP_CONFIG?.wx || {}
    const iamUrl = wxConfig.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
    const clientId = wxConfig.iamClientId
    if (!clientId) {
      alert('无法获取配置，请刷新页面重试')
      return
    }
    const redirectUri = encodeURIComponent(window.location.origin + '/callback')
    const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
  }
}

/**
 * Token 过期时静默降级为游客：清除 token、跳转首页
 * 不同于 redirectToLogin() 跳转 IAM OAuth 页
 */
export function degradeToGuest() {
  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  localStorage.removeItem('user_sys_perm')
  localStorage.removeItem('user_cus_perm')
  localStorage.removeItem('user_cus_perm_ext')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  sessionStorage.setItem('logged_out_due_expiry', '1')
  window.location.href = '/'
}

export function getToken() {
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (token && expiry) {
    const now = new Date().getTime()
    if (now <= parseInt(expiry)) {
      return token
    }
  }
  
  const sessionToken = sessionStorage.getItem('token')
  if (sessionToken) return sessionToken
  
  return null
}

export function getTokenFromCookie() {
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const [name, value] = cookie.trim().split('=')
    if (name === 'token') {
      return value
    }
  }
  return null
}

async function refreshAccessToken() {
  const refreshToken = localStorage.getItem('refresh_token')
  if (!refreshToken) throw new Error('No refresh token')

  const response = await fetch(`${API_BASE_URL}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })

  if (!response.ok) throw new Error('Token refresh failed')

  const data = await response.json()
  if (data.code === 20000 && data.data?.access_token) {
    localStorage.setItem('token', data.data.access_token)
    if (data.data.refresh_token) localStorage.setItem('refresh_token', data.data.refresh_token)
    return data.data.access_token
  }
  throw new Error('Invalid refresh response')
}

function processApiResponse(endpoint, data) {
  // 标准化响应：确保返回数组
  if (Array.isArray(data)) {
    return data
  }

  // 提取常见包装字段
  if (data && typeof data === 'object') {
    if (Array.isArray(data.data)) return data.data
    if (Array.isArray(data.items)) return data.items
    if (Array.isArray(data.result)) return data.result
    if (Array.isArray(data.list)) return data.list

    // 处理嵌套格式: { code: 20000, data: { instruments: [...] } }
    if (data.data && typeof data.data === 'object') {
      if (Array.isArray(data.data.instruments)) return data.data.instruments
      if (Array.isArray(data.data.list)) return data.data.list
    }

    if (data.success && Array.isArray(data.data)) return data.data

    if (data.code === 0 && Array.isArray(data.data)) return data.data
    if (data.code === 20000 && Array.isArray(data.data)) return data.data
  }

  // 非数组响应返回完整对象（保留原始格式）
  return data
}

async function request(endpoint, options = {}) {
  console.log('[API Debug] Making request to:', endpoint)
  
  const token = getToken()
  console.log('[API Debug] Token found:', !!token)
  
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    if (isPublicRoute()) {
      return []
    }
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await fetch(`${API_BASE_URL}${endpoint}`, { ...options, headers })
      if (retryResp.ok) {
        const retryData = await retryResp.json()
        return processApiResponse(endpoint, retryData)
      }
    } catch {}
    // 40104 = no org binding, don't force logout, just return empty
    try {
      const clone = response.clone()
      const body = await clone.json()
      if (body.code === 40104) return []
    } catch {}
    degradeToGuest()
    return []
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()

  // 处理 token 过期错误 — 尝试刷新后重试
  if (data.code === 40101 || data.code === 401) {
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await fetch(`${API_BASE_URL}${endpoint}`, { ...options, headers })
      if (retryResp.ok) {
        const retryData = await retryResp.json()
        return processApiResponse(endpoint, retryData)
      }
    } catch {}
    degradeToGuest()
    return []
  }

  return processApiResponse(endpoint, data)
}

/**
 * 统一 API Fetch 函数
 * 自动添加 Authorization header，处理 401/40101 错误
 */
export async function apiFetch(url, options = {}) {
  const token = getToken()
  
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  }
  
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  
  const response = await fetch(url, {
    ...options,
    headers,
  })
  
  if (response.status === 401 && !url.includes('/public/')) {
    if (isPublicRoute()) {
      throw new Error('Unauthorized')
    }
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await fetch(url, { ...options, headers })
      if (retryResp.ok || retryResp.status !== 401) return retryResp
    } catch {}
    // 40104 = no org binding, don't force logout, just return response
    try {
      const clone = response.clone()
      const body = await clone.json()
      if (body.code === 40104) return response
    } catch {}
    degradeToGuest()
    throw new Error('Unauthorized')
  }
  
  return response
}

export const api = {
  get: (endpoint) => request(endpoint),
  post: (endpoint, data) => request(endpoint, { method: 'POST', body: JSON.stringify(data) }),
  put: (endpoint, data) => request(endpoint, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (endpoint) => request(endpoint, { method: 'DELETE' }),
}

export const instrumentsApi = {
  list: () => api.get('/instruments'),
  get: (id) => api.get(`/instruments/${id}`),
  getPricing: (id) => api.get(`/instruments/${id}/pricing`),
  create: (data) => api.post('/instruments', data),
}

export const ordersApi = {
  preview: (data) => api.post('/orders/preview', data),
  create: (data) => api.post('/user/orders', data),
  batchCreate: (data) => api.post('/user/orders/batch', data),
  transferOwnership: (id) => api.post(`/orders/${id}/transfer-ownership`),
  terminate: (id) => api.put(`/orders/${id}/terminate`),
  getOverdue: () => api.get('/overdue-leases'),
}

export const sitesApi = {
  list: () => api.get('/common/sites'),
  nearby: (params) => api.get(`/common/sites/nearby?lat=${params.lat}&lng=${params.lng}`),
  get: (id) => api.get(`/common/sites/${id}`),
  create: (data) => api.post('/merchant/sites', data),
  update: (id, data) => api.put(`/merchant/sites/${id}`, data),
  delete: (id) => api.delete(`/merchant/sites/${id}`),
}

export const inventoryApi = {
  list: () => api.get('/merchant/inventory'),
  transfer: (data) => api.post('/merchant/inventory/transfer', data),
  listTransfers: () => api.get('/merchant/inventory/transfers'),
}

export const maintenanceApi = {
  submit: (data) => api.post('/maintenance', data),
  get: (id) => api.get(`/maintenance/${id}`),
  cancel: (id) => api.put(`/maintenance/${id}/cancel`),
  listMerchant: () => api.get('/merchant/maintenance'),
  accept: (id) => api.put(`/merchant/maintenance/${id}/accept`),
  assign: (id, data) => api.put(`/merchant/maintenance/${id}/assign`, data),
  updateProgress: (id, data) => api.put(`/merchant/maintenance/${id}/update`, data),
  sendQuote: (id, data) => api.post(`/merchant/maintenance/${id}/quote`, data),
}

export const ownershipApi = {
  get: (id) => api.get(`/user/ownership/${id}`),
  download: (id) => api.get(`/user/ownership/${id}/download`),
}

export const warehouseApi = {
  listOrders: (params = {}) => {
    const query = new URLSearchParams()
    if (params.status) query.set('status', params.status)
    if (params.site_id) query.set('site_id', params.site_id)
    if (params.page) query.set('page', params.page)
    if (params.pageSize) query.set('pageSize', params.pageSize)
    const qs = query.toString()
    return api.get(`/warehouse/orders${qs ? '?' + qs : ''}`)
  },
}

export const appealsApi = {
  submit: (data) => api.post('/appeals', data),
  agree: (damageReportId) => api.post(`/appeals/${damageReportId}/agree`),
  list: () => api.get('/user/appeals'),
  get: (id) => api.get(`/appeals/${id}`),
}

export const contractsApi = {
  list: () => api.get('/user/contracts'),
  get: (id) => api.get(`/user/contracts/${id}`),
}

export const addressesApi = {
  list: () => api.get('/user/addresses'),
  create: (data) => api.post('/user/addresses', data),
  update: (id, data) => api.put(`/user/addresses/${id}`, data),
  setDefault: (id) => api.put(`/user/addresses/${id}/default`),
  delete: (id) => api.delete(`/user/addresses/${id}`),
}

export function resendEmailConfirmation() {
  return api.post('/users/me/resend-email-confirmation')
}

// Permission config API (#414)
export const permissionConfigApi = {
  getMapping: () => api.get('/config/permissions'),
}

let permissionMappingLoaded = false

export async function initPermissionMapping() {
  if (permissionMappingLoaded) return
  const token = getToken()
  if (!token) return
  try {
    const resp = await permissionConfigApi.getMapping()
    if (resp && resp.code === 20000) {
      localStorage.setItem('permission_mapping', JSON.stringify(resp.data.cus_perm_mapping || {}))
      permissionMappingLoaded = true
    }
  } catch (e) {
    console.warn('[Permissions] Failed to load permission mapping')
  }
}

// Global fetch 401 interceptor — catches all fetch calls, not just apiFetch/request
const origFetch = window.fetch.bind(window)
window.fetch = async function(input, init) {
  const response = await origFetch(input, init)
  if (response.status === 401) {
    const url = typeof input === 'string' ? input : (input?.url || '')
    if (url.includes('/public/')) return response
    let skipDegrade = false
    try {
      const clone = response.clone()
      const body = await clone.json()
      if (body.code === 40104) skipDegrade = true
    } catch {}
    if (!skipDegrade && getToken()) {
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
      localStorage.removeItem('user_sys_perm')
      localStorage.removeItem('user_cus_perm')
      localStorage.removeItem('user_cus_perm_ext')
      sessionStorage.removeItem('token')
      document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
      window.location.href = '/'
    }
  }
  return response
}

export default api
