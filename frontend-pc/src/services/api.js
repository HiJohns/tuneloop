import Logger from '../utils/logger'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function getToken() {
  // 优先级 1: 检查 localStorage（前端主动存储的，最可靠）
  const localToken = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (localToken && expiry) {
    const now = new Date().getTime()
    if (now < parseInt(expiry)) {
      return localToken
    } else {
      // Token 过期，清理
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
    }
  }
  
  // 优先级 2: 检查 sessionStorage（临时会话）
  const sessionToken = sessionStorage.getItem('token')
  if (sessionToken) return sessionToken
  
  // 优先级 3: 检查 cookie（后端设置的）- 作为后备方案
  // 注意：由于 SameSite 和 domain 限制，前端可能无法读取
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const trimmed = cookie.trim()
    const eqPos = trimmed.indexOf('=')
    if (eqPos > 0) {
      const name = trimmed.substring(0, eqPos)
      const value = trimmed.substring(eqPos + 1)
      if (name === 'token') {
        Logger.log('AUTH', 'Token retrieved from cookie (fallback)')
        return decodeURIComponent(value)
      }
    }
  }
  
  return null
}

function getRefreshToken() {
  const rt = localStorage.getItem('refresh_token')
  console.log('%c[AUTH DEBUG] getRefreshToken:', 'color: blue;', rt ? `found (${rt.substring(0, 10)}...)` : 'NOT FOUND')
  return rt
}

function storeTokens(accessToken, refreshToken) {
  const token = typeof accessToken === 'string' ? accessToken : accessToken?.access_token
  const refresh = typeof refreshToken === 'string' ? refreshToken : accessToken?.refresh_token
  if (token) {
    localStorage.setItem('token', token)
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      const expTime = payload.exp ? new Date(payload.exp * 1000) : null
      if (expTime) {
        console.error(`%c🔑 Token 过期时间: ${expTime.toLocaleTimeString()}`, 'color: red; font-weight: bold')
      }
    } catch (e) {}
  }
  if (refresh) {
    localStorage.setItem('refresh_token', refresh)
  }
}

function storePermVersion(permVersion) {
  localStorage.setItem('perm_version', String(permVersion || 0))
}

function getPermVersion() {
  return parseInt(localStorage.getItem('perm_version') || '0')
}

function clearTokens() {
  localStorage.removeItem('token')
  localStorage.removeItem('refresh_token')
  localStorage.removeItem('perm_version')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
}

function isWeChatBrowser() {
  const ua = navigator.userAgent.toLowerCase()
  return ua.indexOf('micromessenger') !== -1
}

function redirectToIAM() {
  const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
  const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc'
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
    // Token有效期15分钟，当剩余<5分钟时触发续期
    const REFRESH_THRESHOLD = 5 * 60 * 1000  // 5分钟
    return timeLeft < REFRESH_THRESHOLD
  } catch (e) {
    Logger.error('AUTH', 'Failed to parse token:', e)
    return true
  }
}

async function handleAuthError(token, retryCount, endpoint, options) {
  console.log('%c[AUTH DEBUG] handleAuthError called', 'color: orange;', {
    endpoint, retryCount, hasToken: !!token,
    timestamp: new Date().toISOString()
  })

  if (retryCount < 1) {
    try {
      await refreshAccessToken()
      Logger.log('AUTH', 'Token refreshed after auth error, retrying request')
      return await request(endpoint, options, retryCount + 1)
    } catch (e) {
      console.error('%c[AUTH DEBUG] refreshAccessToken FAILED:', 'color: red;', e.message)
      Logger.warn('AUTH', 'Token refresh failed in handleAuthError:', e.message)
    }
  }

   console.log('%c[AUTH DEBUG] Auth failed → clearing tokens → redirecting to IAM', 'color: red;')
   Logger.warn('AUTH', 'Auth failed, redirecting to IAM login')
   clearTokens()
   const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
   const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc'
   const redirectUri = encodeURIComponent(window.location.origin + '/callback')
   window.location.href = iamUrl + '/oauth/authorize?client_id=' + clientId + '&redirect_uri=' + redirectUri + '&response_type=code'
  
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
  const method = options.method || 'GET'
  Logger.api(endpoint, method, { timestamp: new Date().toISOString() })
  
  let token = getToken()
  
  // Step 2: 实现滑动窗口续期 - 在请求前检查 Token 状态
  if (token && isTokenExpiringSoon(token) && retryCount === 0) {
    try {
      await refreshAccessToken()
      Logger.log('AUTH', 'Token auto-refreshed (sliding window)')
      // 刷新后重新获取 token
      token = getToken()
    } catch (e) {
      Logger.warn('AUTH', 'Token refresh failed:', e.message)
    }
}
  
  const headers = {
    ...options.headers,
  }
  // Don't set Content-Type for FormData — browser will auto-set multipart/form-data with boundary
  if (!(options.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json'
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers,
  })

  // Check perm_version mismatch and force re-login if changed
  const responsePermVersion = response.headers.get('x-perm-version')
  if (responsePermVersion) {
    const serverVersion = parseInt(responsePermVersion)
    const storedVersion = getPermVersion()
    if (serverVersion > 0 && storedVersion > 0 && serverVersion !== storedVersion) {
      console.warn('[AUTH] perm_version changed:', storedVersion, '->', serverVersion, ', forcing re-login')
      clearTokens()
      const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
      const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc'
      const redirectUri = encodeURIComponent(window.location.origin + '/callback')
      window.location.href = iamUrl + '/login?reason=session_expired&client_id=' + clientId + '&redirect_uri=' + redirectUri
      throw new Error('Permission version changed, please re-login')
    }
  }

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
    
    if (response.status === 404) {
      return {
        code: 40400,
        message: error.message || 'Resource not found',
        data: null,
        status: 404
      }
    }

    if (response.status === 409) {
      return {
        code: error.code || 40900,
        message: error.message || 'Conflict',
        data: error.data,
        status: 409
      }
    }
    
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()
  
  // Log non-200 responses for debugging
  if (data.code !== 20000) {
    Logger.error('API', `Non-200 response: ${endpoint}`, { status: response.status, body: data })
  } else {
    Logger.api(endpoint, method, { status: response.status, code: data.code })
  }
  
  // Handle 40101 token expired
  if (data.code === 40101) {
    const result = await handleAuthError(token, retryCount, endpoint, options)
    if (result && result.__authFailed) {
      return { code: 40101, message: '认证失败', data: null }
    }
    if (result && !result.__authFailed) {
      return result
    }
  }
  
  // 统一响应格式处理: 优先返回完整响应对象，让调用方自行处理
  // 后端标准格式: { code: 20000, data: ..., message: ... }
  if (data && typeof data === 'object') {
    // 如果是标准响应格式，直接返回完整对象
    if ('code' in data) {
      return data
    }
    // 兼容旧格式: { success: true, data: ... }
    if ('success' in data) {
      return data
    }
  }
  
  // 原始数组响应
  if (Array.isArray(data)) {
    return { code: 20000, data: data }
  }
  
  // 其他情况返回原始数据
  return data
}

export const api = {
  get: (endpoint, options = {}) => {
    if (options.params) {
      const qs = new URLSearchParams(
        Object.entries(options.params).filter(([, v]) => v !== undefined && v !== null && v !== '')
      ).toString()
      return request(`${endpoint}${qs ? '?' + qs : ''}`)
    }
    return request(endpoint)
  },
  post: (endpoint, data) => request(endpoint, { method: 'POST', body: JSON.stringify(data) }),
  put: (endpoint, data) => request(endpoint, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (endpoint, data) => request(endpoint, {
    method: 'DELETE',
    ...(data ? { body: JSON.stringify(data) } : {}),
  }),
}

export const instrumentsApi = {
  list: (params = {}) => {
    const query = new URLSearchParams(params).toString()
    return api.get(`/instruments${query ? '?' + query : ''}`)
  },
  get: (id) => api.get(`/instruments/${id}`),
  getPricing: (id) => api.get(`/instruments/${id}/pricing`),
  getLevels: () => api.get('/instruments/levels'),
  batchImportPreview: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return request('/instruments/batch-import/preview', {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
  batchImportMedia: (file, sessionId) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('session_id', sessionId)
    return request('/instruments/batch-import/media', {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
  batchImport: (sessionId) => api.post('/instruments/batch-import', { session_id: sessionId }),
  createMedia: (id, data) => api.post(`/instruments/${id}/media`, data),
  setMediaDisplay: (id, data) => api.put(`/instruments/${id}/media/display`, data),
  deleteMediaBatch: (id, batchId) => api.delete(`/instruments/${id}/media/${batchId}`),
  getMedia: (id) => api.get(`/instruments/${id}/media`),
}

export const ordersApi = {
  create: (data) => api.post('/user/orders', data),
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
  getTree: (rootId) => api.get(rootId ? `/sites/tree?root=${rootId}` : '/sites/tree'),
}

export const iamApi = {
  syncOrganizations: () => api.post('/iam/organizations/sync'),
  syncUsers: () => api.post('/iam/users/sync'),
}

export const staffApi = {
  list: (params) => api.get('/staff', { params }),
  getMe: () => api.get('/users/me'),
  createUser: (data) => api.post('/users', data),
  updateUser: (id, data) => api.put(`/users/${id}`, data),
  updateIAMUser: (id, data) => api.put(`/iam/users/${id}`, data),
  checkUserExists: (phone, email, username) => {
    const params = new URLSearchParams()
    if (phone) params.append('phone', phone)
    if (email) params.append('email', email)
    if (username) params.append('username', username)
    const qs = params.toString()
    return api.get(`/users/check${qs ? '?' + qs : ''}`)
  },
  batchDelete: (ids) => api.delete('/users/batch', { ids }),
  resetPassword: (userIds, redirectUrl) => api.post('/users/reset-password', { user_ids: userIds, redirect_url: redirectUrl }),
  activateUser: (id) => api.post(`/users/${id}/activate`),
}

export const inventoryApi = {
  getTransferList: (params) => api.get('/merchant/inventory/transfers', { params }),
  getRentSetting: (params) => api.get('/inventory/rent-setting', { params }),
  batchUpdateRent: (data) => api.put('/inventory/rent-setting/batch', data),
}

export const pricingApi = {
  getTemplates: () => api.get('/pricing/templates'),
  getMerchantConfig: () => api.get('/pricing/merchant-config'),
  updateMerchantConfig: (data) => api.put('/pricing/merchant-config', data),
  batchSetPricing: (data) => api.put('/instruments/batch-pricing', data),
  getInstrumentPricingV2: (id) => api.get(`/instruments/${id}/pricing-v2`),
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

export const adminApi = {
  listUsers: (params = {}) => {
    const query = new URLSearchParams(params).toString()
    return api.get(`/admin/users${query ? '?' + query : ''}`)
  },
  setUserPermissions: (userId, cusPermCodes) => api.put(`/admin/users/${userId}/permissions`, { cus_perm_codes: cusPermCodes }),
  setUserRole: (userId, roleCode) => api.put(`/admin/users/${userId}/roles`, { role_code: roleCode }),
  listRoles: () => api.get('/admin/roles'),
  createRole: (data) => api.post('/admin/roles', data),
  updateRole: (id, data) => api.put(`/admin/roles/${id}`, data),
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

export const bulkImportApi = {
  previewOrganizations: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return request('/admin/bulk-import/organizations?dry_run=true', {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
  importOrganizations: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return request('/admin/bulk-import/organizations', {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
  previewAccounts: (file, skipActivation) => {
    const formData = new FormData()
    formData.append('file', file)
    const params = skipActivation ? '?dry_run=true&skip_activation=true' : '?dry_run=true'
    return request('/admin/bulk-import/accounts' + params, {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
  importAccounts: (file, skipActivation) => {
    const formData = new FormData()
    formData.append('file', file)
    const params = skipActivation ? '?skip_activation=true' : ''
    return request('/admin/bulk-import/accounts' + params, {
      method: 'POST',
      body: formData,
      headers: {}
    })
  },
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

export const propertiesApi = {
  searchOptions: (propertyId, q, limit = 3) => api.get(`/properties/${propertyId}/options/search?q=${encodeURIComponent(q)}&limit=${limit}`),
}

// Permission API (#414)
export const permissionConfigApi = {
  // Fetch permission bit mapping from backend
  getMapping: () => api.get('/config/permissions'),
}

// Initialize permission mapping cache on app load
let permissionMappingLoaded = false

export async function initPermissionMapping() {
  if (permissionMappingLoaded) return
  try {
    const resp = await permissionConfigApi.getMapping()
    if (resp && resp.code === 20000) {
      localStorage.setItem('permission_mapping', JSON.stringify(resp.data.cus_perm_mapping || {}))
      permissionMappingLoaded = true
    }
  } catch (e) {
    console.warn('[Permissions] Failed to load permission mapping, using empty cache')
  }
}

export const auditLogApi = {
  list: (params) => api.get('/admin/audit-logs', { params }),
  get: (id) => api.get(`/admin/audit-logs/${id}`),
  export: async (data) => {
    const token = getToken();
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = `Bearer ${token}`;
    const resp = await fetch(`${API_BASE_URL}/admin/audit-logs/export`, {
      method: 'POST', headers, body: JSON.stringify(data),
    });
    if (!resp.ok) throw new Error('导出失败');
    return await resp.blob();
  },
}

export { getToken }
