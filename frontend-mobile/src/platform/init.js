import { storage, session, request, navigation } from './index'

let initCalled = false
let _initPermissionMapping = null
let _publicRoutes = []

export function setInitDeps(initPermissionMapping, publicRoutes) {
  _initPermissionMapping = initPermissionMapping
  _publicRoutes = publicRoutes
}

export function storeToken(accessToken, expiresIn = 3600, refreshToken) {
  const expiry = new Date().getTime() + (expiresIn * 1000)
  storage.setItem('token', accessToken)
  storage.setItem('token_expiry', expiry.toString())
  if (refreshToken) storage.setItem('refresh_token', refreshToken)
}

export function parseJWT(token) {
  if (!token || !token.includes('.')) return {}
  try {
    return JSON.parse(atob(token.split('.')[1]))
  } catch (e) {
    return {}
  }
}

export function cachePermissions(claims) {
  const sysPerm = parseInt(claims.sys_perm) || 0
  const cusPerm = parseInt(claims.cus_perm) || 0
  storage.setItem('user_sys_perm', sysPerm.toString())
  storage.setItem('user_cus_perm', cusPerm.toString())
  storage.setItem('user_cus_perm_ext', claims.cus_perm_ext || '')
}

export function isNamespaceAdmin() {
  const sysPerm = parseInt(storage.getItem('user_sys_perm') || '0')
  const cusPerm = parseInt(storage.getItem('user_cus_perm') || '0')
  return sysPerm > 0 && cusPerm === 0
}

export function getWXConfig() {
  try {
    const config = storage.getJSON('app_config', {})
    return config.wx || null
  } catch {
    return null
  }
}

async function fetchConfig(retries = 3) {
  for (let i = 0; i < retries; i++) {
    try {
      const res = await request('/api/config')
      const data = await res.json()
      if (data.code === 20000) {
        storage.setJSON('app_config', data.data)
        if (typeof window !== 'undefined') window.APP_CONFIG = data.data
        return data.data
      }
    } catch {}
    if (i < retries - 1) await new Promise(r => setTimeout(r, 1000))
  }
  return null
}

export function getAppConfig() {
  return storage.getJSON('app_config', null)
}

export async function initializeApp() {
  if (initCalled) return
  initCalled = true

  const config = await fetchConfig()

  if (_initPermissionMapping) {
    _initPermissionMapping()
  }

  const token = storage.getItem('token')
  const path = navigation.getCurrentPath()

  if (!token && _publicRoutes && !_publicRoutes.includes(path)) {
    session.setItem('post_auth_redirect', path)
  }

  if (token) {
    cachePermissions(parseJWT(token))
  }

  return config
}

export function showLoginReason() {
  const showReason = session.getItem('show_login_reason')
  if (showReason) {
    session.removeItem('show_login_reason')
    const message = {
      session_expired: '登录已过期，请重新登录',
      token_missing: '请先登录',
    }
    return message[showReason] || '请先登录'
  }
  return null
}
