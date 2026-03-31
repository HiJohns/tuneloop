const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

/**
 * 检测是否在微信小程序环境中
 */
function isWeChatMiniProgram() {
  return typeof wx !== 'undefined' && wx.miniProgram
}

/**
 * 统一的登录重定向函数
 */
function redirectToLogin() {
  // 清理 token
  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  
  if (isWeChatMiniProgram()) {
    // 微信小程序环境：跳转到小程序登录页
    wx.miniProgram.redirectTo({
      url: '/pages/login/login'
    })
  } else {
    // 普通 H5 环境：跳转到 IAM OAuth
    const iamUrl = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
    const clientId = window.APP_CONFIG?.iamClientId || 'tuneloop'
    const redirectUri = encodeURIComponent(window.location.origin + '/callback')
    const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
  }
}

export function getToken() {
  // 1. 优先从 localStorage 获取（与 OAuthCallback 存储一致）
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (token && expiry) {
    const isExpired = new Date().getTime() > parseInt(expiry)
    
    if (isExpired) {
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
      localStorage.removeItem('user_info')
      // 过期时尝试从 cookie 获取
      const cookieToken = getTokenFromCookie()
      return cookieToken
    }
    return token
  }
  
  // 2. 尝试从 sessionStorage 获取
  const sessionToken = sessionStorage.getItem('token')
  if (sessionToken) return sessionToken
  
  // 3. 最后从 cookie 获取作为 fallback
  // 这主要是为了处理页面刷新后 localStorage 不可用的场景
  const cookieToken = getTokenFromCookie()
  return cookieToken
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
    redirectToLogin()
    return
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()
  
  // 处理 token 过期错误
  if (data.code === 40101 || data.code === 401) {
    redirectToLogin()
    return []
  }
  
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
      // 后端返回: { code: 20000, data: { instruments: [...] } }
      if (Array.isArray(data.data.instruments)) return data.data.instruments
      // 后端返回: { code: 20000, data: { list: [...] } }
      if (Array.isArray(data.data.list)) return data.data.list
    }
    
    // 处理统一响应格式: { success: true, data: [...] }
    if (data.success && Array.isArray(data.data)) return data.data
    
    // 处理 code 格式: { code: 0, data: [...] }
    if (data.code === 0 && Array.isArray(data.data)) return data.data
    if (data.code === 20000 && Array.isArray(data.data)) return data.data
  }
  
  // 兜底: 返回空数组
  console.warn(`API ${endpoint} returned non-array data:`, data)
  return []
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
  
  try {
    const response = await fetch(url, {
      ...options,
      headers,
    })
    
    // 统一 401 处理
    if (response.status === 401) {
      redirectToLogin()
      throw new Error('Unauthorized')
    }
    
    return response
  } catch (error) {
    // 网络错误也可能需要重新登录
    throw error
  }
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
}

export const ordersApi = {
  preview: (data) => api.post('/orders/preview', data),
  create: (data) => api.post('/orders', data),
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

export default api
