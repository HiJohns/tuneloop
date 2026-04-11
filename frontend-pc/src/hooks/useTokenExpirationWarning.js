import { useEffect } from 'react'
import { message } from 'antd'
import Logger from '../utils/logger'

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

export function useTokenExpirationWarning() {
  useEffect(() => {
    const checkExpiration = setInterval(() => {
      const token = getToken()
      if (token) {
        try {
          const payload = JSON.parse(atob(token.split('.')[1]))
          const daysLeft = (payload.exp * 1000 - Date.now()) / (24 * 60 * 60 * 1000)
          if (daysLeft < 3 && daysLeft > 0) {
            message.info('登录即将过期，正在自动延期...')
            Logger.log('AUTH', `Token expires in ${daysLeft.toFixed(1)} days`)
          }
        } catch (e) {
          Logger.error('AUTH', 'Failed to parse token for expiration check:', e)
        }
      }
    }, 60 * 60 * 1000) // 每小时检查一次

    return () => clearInterval(checkExpiration)
  }, [])
}
