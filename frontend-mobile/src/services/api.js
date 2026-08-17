import { storage, session, cookie, request as platformRequest, dialog, navigation, env, wxLogin as wxLoginCode, eventBus } from '../platform'

export const publicRoutes = ['/', '/instrument', '/content', '/cart', '/success', '/callback']

function isPublicRoute() {
  const path = navigation.getCurrentPath()
  return publicRoutes.some(p => path === p || path.startsWith(p + '/'))
}

if (env.isDev && typeof window !== 'undefined' && typeof window.wx === 'undefined') {
  console.log('[Dev Mode] Injecting mock wx object for browser debugging')
  window.wx = {
    miniProgram: {
      redirectTo: (options) => {
        console.log('[Mock wx.miniProgram] redirectTo:', options)
        navigation.redirect(options.url || '/login')
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

function isWeChatMiniProgram() {
  return typeof window !== 'undefined' &&
         window.__wxjs_environment === 'miniprogram'
}

export async function wxLogin() {
  return new Promise((resolve, reject) => {
    if (typeof wx === 'undefined' || !wx.login) {
      reject(new Error('Not in WeChat mini-program'))
      return
    }
    wx.login({
      success: async (res) => {
        try {
          const response = await platformRequest(`${env.apiBaseUrl}/wx/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ code: res.code }),
          })
          const data = await response.json()
          if (data.code === 20000 && data.data?.access_token) {
            storage.setItem('token', data.data.access_token)
            if (data.data.expires_in) {
              storage.setItem('token_expiry', (new Date().getTime() + data.data.expires_in * 1000).toString())
            }
            if (data.data.user) {
              storage.setJSON('user_info', data.data.user)
            }
            cachePermissions(parseJWT(data.data.access_token))
            resolve(data.data)
          } else {
            reject(new Error(data.message || 'Login failed'))
          }
        } catch (err) {
          reject(err)
        }
      },
      fail: (err) => reject(err),
    })
  })
}

export async function redirectToLogin(reason) {
  if (reason) {
    session.setItem('login_reason', reason)
  }

  if (reason && reason !== 'session_expired' && reason !== 'token_missing') {
    // dialog.confirm is async in weapp (Taro.showModal) — must await, otherwise
    // the confirm is bypassed and the login flow runs behind the modal.
    const ok = await dialog.confirm('此功能需要登录，是否前往登录？')
    if (!ok) return
  }

  storage.removeItem('token')
  storage.removeItem('token_expiry')
  storage.removeItem('user_sys_perm')
  storage.removeItem('user_cus_perm')
  storage.removeItem('user_cus_perm_ext')
  session.removeItem('token')
  cookie.remove('token')

  if (isWeChatMiniProgram()) {
    // Multi-account login routing (#1639): reason='checkout' → source='checkout'
    // (cart submit / instant rent), anything else → source='profile' (我的).
    await resolveLogin(reason === 'checkout' ? 'checkout' : 'profile')
  } else {
    const wxConfig = window.APP_CONFIG?.wx || {}
    const iamUrl = wxConfig.iamExternalUrl || env.iamExternalUrl
    const clientId = wxConfig.iamClientId
    if (!clientId) {
      dialog.alert('无法获取配置，请刷新页面重试')
      return
    }
    const redirectUri = encodeURIComponent(navigation.getOrigin() + '/callback')
    const authUrl = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
    navigation.redirect(authUrl)
  }
}

// storeLoginToken persists a wx-login-select token and emits loginSuccess.
function storeLoginToken(data) {
  if (!data?.access_token) return false
  storage.setItem('token', data.access_token)
  if (data.expires_in) {
    storage.setItem('token_expiry', (new Date().getTime() + data.expires_in * 1000).toString())
  }
  if (data.refresh_token) storage.setItem('refresh_token', data.refresh_token)
  cachePermissions(parseJWT(data.access_token))
  eventBus.emit('loginSuccess')
  return true
}

// resolveLogin implements the WeChat multi-account login routing (#1639).
// source='profile' (个人中心「我的」): 0 → register, 1 → direct login,
// N → account-select page. source='checkout' (购物车提交/立即租赁):
// customer account exists → direct login; otherwise → register prompt modal.
// Only meaningful inside the WeChat mini-program (weapp).
export async function resolveLogin(source = 'profile') {
  if (!env.isMiniProgram) return false
  try {
    const code = await wxLoginCode()
    if (!code) return false
    const resp = await platformRequest(`${env.apiBaseUrl}/auth/wx-accounts?code=${encodeURIComponent(code)}`)
    const result = await resp.json()
    if (result.code !== 20000 || !result.data) return false
    const { accounts = [], exchange_token } = result.data
    const customers = accounts.filter(a => a.is_customer)

    if (source === 'checkout') {
      if (customers.length > 0) {
        // Direct login as the customer and return to the target page
        const loginResp = await platformRequest(`${env.apiBaseUrl}/auth/wx-login-select`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ exchange_token, user_id: customers[0].user_id }),
        })
        const loginResult = await loginResp.json()
        if (loginResult.code === 20000 && storeLoginToken(loginResult.data)) {
          return true
        }
        dialog.alert(loginResult.message || '登录失败，请重试')
        return false
      }
      // No customer account → register prompt
      const wantRegister = await dialog.confirm('您尚未注册会员，要注册吗？')
      if (wantRegister) {
        // #1690: keep the origin (post_auth_redirect, written by the entry
        // before redirectToLogin) so registration completion returns to it.
        if (!session.getItem('post_auth_redirect')) {
          session.setItem('post_auth_redirect', '/checkout')
        }
        navigation.navigateTo(`/pages-weapp/profile-complete/index?mode=member`)
      }
      return false
    }

    // source='profile'
    if (accounts.length === 0) {
      // Keep the exchange_token for the registration flow (#1644): the
      // WeChat code is single-use and consumed by wx-accounts above, so
      // wx-bind during register must use the exchange_token instead.
      session.setItem('wx_login_token', exchange_token || '')
      // Two-phase registration (#1663): if a pending registration session
      // exists, resume it (form prefill + jump to membership payment)
      // instead of creating a brand-new account.
      const pendingSid = session.getItem('pending_registration_session')
      if (pendingSid) {
        try {
          const statusResp = await platformRequest(`${env.apiBaseUrl}/auth/registration-sessions/me?session_id=${encodeURIComponent(pendingSid)}`)
          const statusResult = await statusResp.json()
          if (statusResult.code === 20000 && statusResult.data?.status === 'pending') {
            navigation.navigateTo(`/pages-weapp/profile-complete/index?session_id=${pendingSid}`)
            return false
          }
          // Session gone or already completed → clear the stale marker.
          session.removeItem('pending_registration_session')
        } catch {
          session.removeItem('pending_registration_session')
        }
      }
      navigation.navigateTo('/pages-weapp/profile-complete/index')
      return false
    }
    if (accounts.length === 1) {
      const loginResp = await platformRequest(`${env.apiBaseUrl}/auth/wx-login-select`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange_token, user_id: accounts[0].user_id }),
      })
      const loginResult = await loginResp.json()
      if (loginResult.code === 20000 && storeLoginToken(loginResult.data)) {
        return true
      }
      dialog.alert(loginResult.message || '登录失败，请重试')
      return false
    }
    navigation.navigateTo('/pages-weapp/account-select/index')
    return false
  } catch (err) {
    console.error('[resolveLogin] failed:', err)
    dialog.alert('登录失败，请重试')
    return false
  }
}

export function degradeToGuest() {
  session.setItem('guest_degradation', '1')
  storage.removeItem('token')
  storage.removeItem('token_expiry')
  storage.removeItem('user_sys_perm')
  storage.removeItem('user_cus_perm')
  storage.removeItem('user_cus_perm_ext')
  session.removeItem('token')
  cookie.remove('token')
  session.setItem('logged_out_due_expiry', '1')
  navigation.redirect('/')
}

function parseJWT(token) {
  if (!token || !token.includes('.')) return {}
  try {
    return JSON.parse(atob(token.split('.')[1]))
  } catch {
    return {}
  }
}

function cachePermissions(claims) {
  const sysPerm = parseInt(claims.sys_perm) || 0
  const cusPerm = parseInt(claims.cus_perm) || 0
  storage.setItem('user_sys_perm', sysPerm.toString())
  storage.setItem('user_cus_perm', cusPerm.toString())
  storage.setItem('user_cus_perm_ext', claims.cus_perm_ext || '')
}

export function getToken() {
  const token = storage.getItem('token')
  const expiry = storage.getItem('token_expiry')

  if (token && expiry) {
    const now = new Date().getTime()
    if (now <= parseInt(expiry)) {
      return token
    }
  }

  const sessionToken = session.getItem('token')
  if (sessionToken) return sessionToken

  return null
}

export function getCartKey() {
  const token = getToken()
  if (!token) return 'cart'
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    const userId = payload.sub || payload.user_id || payload.id
    if (userId) return `cart_${userId}`
  } catch {}
  return 'cart'
}

export function getTokenFromCookie() {
  return cookie.get('token')
}

async function refreshAccessToken() {
  const refreshToken = storage.getItem('refresh_token')
  if (!refreshToken) throw new Error('No refresh token')

  const response = await platformRequest(`${env.apiBaseUrl}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })

  if (!response.ok) throw new Error('Token refresh failed')

  const data = await response.json()
  if (data.code === 20000 && data.data?.access_token) {
    storage.setItem('token', data.data.access_token)
    if (data.data.refresh_token) storage.setItem('refresh_token', data.data.refresh_token)
    return data.data.access_token
  }
  throw new Error('Invalid refresh response')
}

function processApiResponse(endpoint, data) {
  if (Array.isArray(data)) {
    return data
  }

  if (data && typeof data === 'object') {
    if (Array.isArray(data.data)) return data.data
    if (Array.isArray(data.items)) return data.items
    if (Array.isArray(data.result)) return data.result
    if (Array.isArray(data.list)) return data.list

    if (data.data && typeof data.data === 'object') {
      if (Array.isArray(data.data.instruments)) return data.data.instruments
      if (Array.isArray(data.data.list)) return data.data.list
    }

    if (data.success && Array.isArray(data.data)) return data.data

    if (data.code === 0 && Array.isArray(data.data)) return data.data
    if (data.code === 20000 && Array.isArray(data.data)) return data.data
  }

  return data
}

export async function request(endpoint, options = {}) {
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

  const response = await platformRequest(`${env.apiBaseUrl}${endpoint}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    if (isPublicRoute()) {
      return []
    }
    try {
      const body = await response.clone().json()
      if (body.code === 40104) return []
    } catch {}
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await platformRequest(`${env.apiBaseUrl}${endpoint}`, { ...options, headers })
      if (retryResp.ok) {
        const retryData = await retryResp.json()
        return processApiResponse(endpoint, retryData)
      }
    } catch {}
    degradeToGuest()
    return []
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.code || 'Request failed')
  }

  const data = await response.json()

  if (data.code === 40101 || data.code === 401) {
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await platformRequest(`${env.apiBaseUrl}${endpoint}`, { ...options, headers })
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

export async function apiFetch(url, options = {}) {
  const token = getToken()

  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  }

  // auth:false — anonymous calls (#1681): two-phase registration prepay runs
  // without an account; a stale token from a previous login would 401.
  if (token && options.auth !== false) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await platformRequest(url, {
    ...options,
    headers,
  })

  if (response.status === 401 && !url.includes('/public/')) {
    if (isPublicRoute()) {
      throw new Error('Unauthorized')
    }
    try {
      const body = await response.clone().json()
      if (body.code === 40104) return response
    } catch {}
    try {
      const newToken = await refreshAccessToken()
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResp = await platformRequest(url, { ...options, headers })
      if (retryResp.ok || retryResp.status !== 401) return retryResp
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

export const notificationApi = {
  list: () => api.get('/notifications'),
  detail: (id) => api.get(`/notifications/${id}`),
  unreadCount: () => api.get('/notifications/unread-count'),
  markRead: (id) => api.post(`/notifications/${id}/read`),
  markAllRead: () => api.post('/notifications/mark-all-read'),
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

export const guarantorsApi = {
  list: () => api.get('/user/guarantors'),
  create: (data) => api.post('/user/guarantors', data),
  delete: (id) => api.delete(`/user/guarantors/${id}`),
}

export const pointsApi = {
  balance: () => api.get('/user/points/balance'),
  transactions: (params) => api.get('/user/points/transactions', params),
}

export function resendEmailConfirmation() {
  return api.post('/users/me/resend-email-confirmation')
}

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
      storage.setJSON('permission_mapping', resp.data.cus_perm_mapping || {})
      permissionMappingLoaded = true
    }
  } catch (e) {
    console.warn('[Permissions] Failed to load permission mapping')
  }
}

export default api
