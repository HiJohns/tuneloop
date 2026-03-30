const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function getToken() {
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const [name, value] = cookie.trim().split('=')
    if (name === 'token') return value
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

function redirectToIAM() {
  const iamUrl = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
  const clientId = window.APP_CONFIG?.iamClientId || 'tuneloop'
  const redirectUri = encodeURIComponent(window.location.origin + '/callback')
  const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
  window.location.href = authUrl
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
  const token = getToken()
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  // 🔍 调试：打印实际发送的 Authorization header
  console.log(`[API Debug] ${endpoint} -> Authorization:`, headers['Authorization'])

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers,
  })

  // Handle 401 Unauthorized
  if (response.status === 401) {
    if (retryCount < 1) {
      // Try to refresh token once
      try {
        await refreshAccessToken()
        return request(endpoint, options, retryCount + 1)
      } catch (refreshError) {
        // Refresh failed, clear tokens and redirect
      }
    }
    
    clearTokens()
    redirectToIAM()
    return
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()
  
  // Handle 40101 token expired
  if (data.code === 40101) {
    if (retryCount < 1) {
      try {
        await refreshAccessToken()
        return request(endpoint, options, retryCount + 1)
      } catch (refreshError) {
        // Refresh failed
      }
    }
    
    clearTokens()
    redirectToIAM()
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
