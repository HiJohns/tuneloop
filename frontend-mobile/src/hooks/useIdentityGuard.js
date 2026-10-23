import { useEffect, useCallback } from 'react'
import { getToken, parseJWT, apiFetch } from '../services/api'
import { storage, navigation, env, useDidShow } from '../platform'

const CACHE_KEY = 'identity_verify_cache'
const CACHE_TTL = 5 * 60 * 1000
const LOCKED_STATUSES = new Set(['none', 'uploaded', 'rejected'])

function isPureCustomer() {
  const payload = parseJWT(getToken())
  return payload?.role === 'USER' && !payload?.oid && !payload?.tid
}

function isExemptPath() {
  const path = navigation.getCurrentPath() || ''
  if (env.isMiniProgram) {
    return path.includes('/pages-weapp/profile/index') || path.includes('/pages-weapp/face-verify/index')
  }
  return path.startsWith('/profile') || path.startsWith('/face-verify')
}

function redirectToFaceVerify() {
  if (env.isMiniProgram) {
    navigation.redirect('/pages-weapp/face-verify/index')
  } else {
    navigation.redirect('/face-verify')
  }
}

function getCache() {
  const raw = storage.getItem(CACHE_KEY)
  if (!raw) return null
  try {
    const { status, ts } = JSON.parse(raw)
    if (Date.now() - ts > CACHE_TTL) return null
    return status
  } catch {
    return null
  }
}

export function setIdentityVerifyCache(status) {
  storage.setItem(CACHE_KEY, JSON.stringify({ status, ts: Date.now() }))
}

export function useIdentityGuard() {
  const check = useCallback(() => {
    if (!isPureCustomer()) return
    if (isExemptPath()) return

    const cached = getCache()
    if (cached) {
      if (LOCKED_STATUSES.has(cached)) {
        redirectToFaceVerify()
      }
      return
    }

    apiFetch(`${env.apiBaseUrl}/users/me`)
      .then(r => r.json())
      .then(data => {
        if (data.code === 20000 && data.data?.id_verify_status) {
          setIdentityVerifyCache(data.data.id_verify_status)
          if (LOCKED_STATUSES.has(data.data.id_verify_status)) {
            redirectToFaceVerify()
          }
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => { check() }, [check])
  useDidShow(check)
}
