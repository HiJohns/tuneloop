const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function getToken() {
  // Try localStorage first (where IAM callback stores token)
  const localStorageToken = localStorage.getItem('token')
  if (localStorageToken) {
    console.log('[Token] Retrieved from localStorage')
    return localStorageToken
  }
  
  // Try sessionStorage as fallback
  const sessionStorageToken = sessionStorage.getItem('token')
  if (sessionStorageToken) {
    console.log('[Token] Retrieved from sessionStorage')
    return sessionStorageToken
  }
  
  // Try cookie as last resort (simplified parsing)
  const cookieMatch = document.cookie.match(/token=([^;]+)/)
  if (cookieMatch) {
    console.log('[Token] Retrieved from Cookie')
    return cookieMatch[1]
  }
  
  console.log('[Token] No token found in any storage')
  return null
}

async function request(endpoint, options = {}) {
  const token = getToken()
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
    const IAM_URL = import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || 'http://opencode.linxdeep.com:5552'
    const CLIENT_ID = import.meta.env.VITE_IAM_CLIENT_ID || 'tuneloop'
    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
    const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
    throw new Error('Unauthorized')
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  return response.json()
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
