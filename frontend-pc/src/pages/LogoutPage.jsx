import { useState, useEffect } from 'react'
import { Spin } from 'antd'

const REASONS = {
  'no_token': '未检测到登录状态，即将跳转到登录页',
  'token_invalid': '登录已失效，即将跳转到登录页',
  'auth_failed': '身份验证失败，即将跳转到登录页',
  'session_expired': '会话已过期，即将跳转到登录页',
  'redirect_to_login': '即将跳转到登录页',
}

export default function LogoutPage() {
  const [countdown, setCountdown] = useState(10)
  const reason = localStorage.getItem('logout_reason') || 'redirect_to_login'
  const reasonText = REASONS[reason] || reason

  useEffect(() => {
    localStorage.removeItem('logout_reason')

    // Clear all auth tokens — must match cookie domain from backend
    localStorage.removeItem('token')
    localStorage.removeItem('token_expiry')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user_info')
    localStorage.removeItem('user_role')
    localStorage.removeItem('user_is_owner')
    
    // Clear cookie without domain + with domain suffix (match backend SetCookie)
    const domains = ['', '.' + window.location.hostname]
    for (const domain of domains) {
      const domainPart = domain ? '; domain=' + domain : ''
      document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/' + domainPart
      document.cookie = 'refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/' + domainPart
    }

    const timer = setInterval(() => {
      setCountdown(prev => {
        if (prev <= 1) {
          clearInterval(timer)
          const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
          const clientId = window.APP_CONFIG?.pc?.iamClientId || (import.meta.env.VITE_IAM_PC_CLIENT_ID || '')
          const redirectUri = encodeURIComponent(window.APP_CONFIG?.pc?.iamRedirectUri || `${window.location.origin}/callback`)
          window.location.href = `${iamUrl}/oauth/authorize?prompt=login&client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code&noRegister=1`
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(timer)
  }, [])

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(180deg, #FDF4E7 0%, #fff 100%)',
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
    }}>
      <Spin size="large" />
      <div style={{ marginTop: 24, textAlign: 'center' }}>
        <p style={{ fontSize: 16, color: '#333', marginBottom: 8, fontWeight: 600 }}>
          {reasonText}
        </p>
        <p style={{ fontSize: 14, color: '#999' }}>
          将在 <strong style={{ color: '#333' }}>{countdown}</strong> 秒后跳转
        </p>
      </div>
    </div>
  )
}
