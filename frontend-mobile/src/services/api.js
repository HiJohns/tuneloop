const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

export function getToken() {
  console.log('[Token Debug] Starting getToken()')
  
  // 1. 优先从 localStorage 获取（与 OAuthCallback 存储一致）
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  console.log('[Token Debug] localStorage - token:', !!token, 'expiry:', expiry)
  
  if (token && expiry) {
    const isExpired = new Date().getTime() > parseInt(expiry)
    console.log('[Token Debug] Token expired check:', isExpired)
    
    if (isExpired) {
      console.log('[Token Debug] Token expired, clearing storage')
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
      localStorage.removeItem('user_info')
      // 过期时尝试从 cookie 获取
      const cookieToken = getTokenFromCookie()
      console.log('[Token Debug] Fallback to cookie token:', !!cookieToken)
      return cookieToken
    }
    console.log('[Token Debug] Returning localStorage token')
    return token
  }
  
  // 2. 尝试从 sessionStorage 获取
  const sessionToken = sessionStorage.getItem('token')
  console.log('[Token Debug] sessionStorage token:', !!sessionToken)
  if (sessionToken) return sessionToken
  
  // 3. 最后从 cookie 获取作为 fallback
  // 这主要是为了处理页面刷新后 localStorage 不可用的场景
  const cookieToken = getTokenFromCookie()
  console.log('[Token Debug] Fallback to cookie token:', !!cookieToken)
  return cookieToken
}

export function getTokenFromCookie() {
  console.log('[Token Debug] Checking cookies for token')
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const [name, value] = cookie.trim().split('=')
    if (name === 'token') {
      console.log('[Token Debug] Found token in cookie')
      return value
    }
  }
  console.log('[Token Debug] No token found in cookie')
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
    console.log('[API Debug] Authorization header SET for:', endpoint)
  } else {
    console.log('[API Debug] NO Authorization header for:', endpoint)
  }

  // Debug alert for /instruments endpoint
  if (endpoint === '/instruments' || endpoint.startsWith('/instruments/')) {
    const cookieToken = getTokenFromCookie()
    alert(`Request to: ${endpoint}\nAuthorization header: ${headers['Authorization'] || 'NOT SET'}\nCookie token: ${cookieToken || 'NOT FOUND'}`)
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    localStorage.removeItem('token')
    sessionStorage.removeItem('token')
    document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
    
    // 优先使用后端配置的 IAM URL
    if (window.APP_CONFIG?.iamLoginUrl) {
      window.location.href = window.APP_CONFIG.iamLoginUrl + '?redirect_uri=' + encodeURIComponent(window.location.href)
    } else {
      // Fallback: 直接跳转到 IAM OAuth 授权页面
      // 使用与 ProtectedRoute 相同的逻辑
      const iamUrl = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
      const clientId = window.APP_CONFIG?.iamClientId || 'tuneloop'
      const redirectUri = encodeURIComponent(window.location.origin + '/callback')
      const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
      window.location.href = authUrl
    }
    return
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()
  
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
