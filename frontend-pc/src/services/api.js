const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function getToken() {
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const trimmed = cookie.trim()
    const eqPos = trimmed.indexOf('=')
    if (eqPos > 0) {
      const name = trimmed.substring(0, eqPos)
      const value = trimmed.substring(eqPos + 1)
      if (name === 'token') return decodeURIComponent(value)
    }
  }
  return localStorage.getItem('token') || sessionStorage.getItem('token')
}

function storeTokens(accessToken, refreshToken) {
  localStorage.setItem('token', accessToken)
  localStorage.setItem('refresh_token', refreshToken)
  document.cookie = `token=${accessToken}; path=/; max-age=604800`
}

function getRefreshToken() {
  return localStorage.getItem('refresh_token')
}

function clearTokens() {
  localStorage.removeItem('token')
  localStorage.removeItem('refresh_token')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
}

function isWeChatBrowser() {
  const ua = navigator.userAgent.toLowerCase()
  return ua.indexOf('micromessenger') !== -1
}

function redirectToIAM() {
  const iamUrl = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
  const clientId = window.APP_CONFIG?.iamClientId || 'tuneloop'
  const redirectUri = encodeURIComponent(window.location.origin + '/callback')
  
  if (isWeChatBrowser()) {
    // WeChat-specific flow - force web login for better compatibility
    const webLoginUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code&force_web=1`
    window.location.href = webLoginUrl
  } else {
    // Standard OAuth flow
    const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
  }
}

function isTokenExpiringSoon(token) {
  if (!token) return true
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    const expTime = payload.exp * 1000
    const now = Date.now()
    const timeLeft = expTime - now
    const thirtyPercentOf30Days = 30 * 24 * 60 * 60 * 1000 * 0.3
    return timeLeft < thirtyPercentOf30Days
  } catch (e) {
    console.error('[Auth Debug] Failed to parse token:', e)
    return true
  }
}

async function handleAuthError(token, retryCount, endpoint, options) {
  // Step 1: 尝试刷新 token (不检查是否即将过期)
  // 如果后端返回 40101，总是先尝试刷新，不进行额外的过期检查
  if (retryCount < 1) {
    try {
      await refreshAccessToken()
      console.log('[Auth Debug] Token refreshed after auth error, retrying request')
      // 刷新成功后重试原请求
      return await request(endpoint, options, retryCount + 1)
    } catch (e) {
      console.log('[Auth Debug] Token refresh failed in handleAuthError:', e.message)
    }
  }
  
  // Step 2: 刷新失败，清除 token 并跳转
  console.log('[Auth Debug] Auth failed, clearing tokens and redirecting to IAM')
  clearTokens()
  redirectToIAM()
  
  // 返回特殊标记表示认证失败
  return { __authFailed: true }
}

async function refreshAccessToken() {
  const refreshToken = getRefreshToken()
  if (!refreshToken) {
    throw new Error('No refresh token available')
  }
  
  const response = await fetch(`${API_BASE_URL}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken })
  })
  
  if (!response.ok) {
    throw new Error('Token refresh failed')
  }
  
  const data = await response.json()
  if (data.code === 20000 && data.data) {
    storeTokens(data.data.access_token, data.data.refresh_token)
    return data.data.access_token
  }
  
  throw new Error('Invalid refresh response')
}

async function request(endpoint, options = {}, retryCount = 0) {
  let token = getToken()
  
  // Step 2: 实现滑动窗口续期 - 在请求前检查 Token 状态
  if (token && isTokenExpiringSoon(token) && retryCount === 0) {
    try {
      await refreshAccessToken()
      console.log('[Auth Debug] Token auto-refreshed (sliding window)')
      // 刷新后重新获取 token
      token = getToken()
    } catch (e) {
      console.log('[Auth Debug] Token refresh failed:', e.message)
    }
  }
  
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

  // Handle 401 Unauthorized
  // 统一处理：调用通用认证错误处理函数
  if (response.status === 401) {
    const result = await handleAuthError(token, retryCount, endpoint, options)
    if (result && result.__authFailed) {
      // 认证失败，已经跳转，直接返回
      return []
    }
    // 如果刷新成功并返回数据，直接返回
    if (result && !result.__authFailed) {
      return result
    }
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()
  
  // Handle 40101 token expired
  // 统一处理：调用通用认证错误处理函数
  if (data.code === 40101) {
    const result = await handleAuthError(token, retryCount, endpoint, options)
    if (result && result.__authFailed) {
      // 认证失败，已经跳转，直接返回
      return []
    }
    // 如果刷新成功并返回数据，直接返回
    if (result && !result.__authFailed) {
      return result
    }
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

export const api = {
  get: (endpoint) => request(endpoint),
  post: (endpoint, data) => request(endpoint, { method: 'POST', body: JSON.stringify(data) }),
  put: (endpoint, data) => request(endpoint, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (endpoint) => request(endpoint, { method: 'DELETE' }),
}

export const instrumentsApi = {
  list: (params = {}) => {
    const query = new URLSearchParams(params).toString()
    return api.get(`/instruments${query ? '?' + query : ''}`)
  },
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

export const permissionApi = {
  getPermissions: () => api.get('/admin/permissions'),
  getRoles: () => api.get('/admin/roles'),
  getRolePermissions: (id) => api.get(`/admin/roles/${id}/permissions`),
  updateRolePermissions: (id, permissions) => api.put(`/admin/roles/${id}/permissions`, { permissions }),
  createRole: (data) => api.post('/admin/roles', data),
  deleteRole: (id) => api.delete(`/admin/roles/${id}`),
}

export const leaseApi = {
  list: (params = {}) => {
    const query = new URLSearchParams(params).toString()
    return api.get(`/merchant/leases${query ? '?' + query : ''}`)
  },
  get: (id) => api.get(`/merchant/leases/${id}`),
  create: (data) => api.post('/merchant/leases', data),
  update: (id, data) => api.put(`/merchant/leases/${id}`, data),
  terminate: (id) => api.delete(`/merchant/leases/${id}`),
}

export const depositApi = {
  list: (params = {}) => {
    const query = new URLSearchParams(params).toString()
    return api.get(`/merchant/deposits${query ? '?' + query : ''}`)
  },
  create: (data) => api.post('/merchant/deposits', data),
  update: (id, data) => api.put(`/merchant/deposits/${id}`, data),
}

export const iamAdminApi = {
  // Client Management
  getClients: () => api.get('/system/clients'),
  createClient: (data) => api.post('/system/clients', data),
  updateClient: (id, data) => api.put(`/system/clients/${id}`, data),
  deleteClient: (id) => api.delete(`/system/clients/${id}`),
  
  // Tenant Management
  getTenants: () => api.get('/system/tenants'),
  createTenant: (data) => api.post('/system/tenants', data),
  getTenant: (id) => api.get(`/system/tenants/${id}`),
  updateTenant: (id, data) => api.put(`/system/tenants/${id}`, data),
  deleteTenant: (id) => api.delete(`/system/tenants/${id}`),
}

export const categoriesApi = {
  getList: () => api.get('/categories'),
  getById: (id) => api.get(`/categories/${id}`),
  create: (data) => api.post('/categories', data),
  update: (id, data) => api.put(`/categories/${id}`, data),
  delete: (id) => api.delete(`/categories/${id}`)
}
