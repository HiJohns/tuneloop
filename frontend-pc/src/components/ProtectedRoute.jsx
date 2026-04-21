import { Navigate, useLocation } from 'react-router-dom'
import { Spin } from 'antd'
import { useState, useEffect } from 'react'

const IAM_URL = import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
const CLIENT_ID = import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc'

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
  
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (!token || !expiry) {
    return sessionStorage.getItem('token') || null
  }
  
  const now = new Date().getTime()
  const exp = parseInt(expiry)
  
  if (now > exp) {
    localStorage.removeItem('token')
    localStorage.removeItem('token_expiry')
    localStorage.removeItem('user_info')
    return sessionStorage.getItem('token') || null
  }
  
  return token
}

async function getTokenWithRetry(maxRetries = 1, delay = 100) {
  let token = getToken()
  
  // If no token and we might be in OAuth callback flow, retry after delay
  if (!token && window.location.pathname === '/callback') {
    for (let i = 0; i < maxRetries; i++) {
      await new Promise(resolve => setTimeout(resolve, delay))
      token = getToken()
      if (token) break
    }
  }
  
  return token
}

function storeToken(token, expiresIn = 3600) {
  const expiry = new Date().getTime() + (expiresIn * 1000)
  localStorage.setItem('token', token)
  localStorage.setItem('token_expiry', expiry.toString())
}

function isTokenValid(token) {
  if (!token) return false
  // If token doesn't look like JWT, accept it anyway (opaque token from IAM)
  if (!token.includes('.')) {
    return true
  }
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    // If no exp claim, accept the token
    if (!payload.exp) {
      return true
    }
    const expTime = payload.exp * 1000
    const now = Date.now()
    return expTime > now
  } catch (e) {
    // Accept token even if parsing fails (opaque token from IAM)
    return true
  }
}

function clearTokens() {
  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  localStorage.removeItem('user_info')
  localStorage.removeItem('user_role')
  sessionStorage.removeItem('token')
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
}

function redirectToLogin() {
  // First call backend to get authorization URL with state cookie set
  fetch('/api/auth/oidc/authorization-url', {
    credentials: 'include'
  })
    .then(response => response.json())
    .then(data => {
      if (data.code === 20000 && data.data && data.data.authorization_url) {
        window.location.href = data.data.authorization_url;
      } else {
        // Fallback to direct OAuth URL
        console.error('[Auth] Failed to get authorization URL, using fallback');
        const redirectUri = encodeURIComponent(`${window.location.origin}/callback`);
        const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`;
        window.location.href = authUrl;
      }
    })
    .catch(error => {
      console.error('[Auth] Error getting authorization URL:', error);
      // Fallback to direct OAuth URL
      const redirectUri = encodeURIComponent(`${window.location.origin}/callback`);
      const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`;
      window.location.href = authUrl;
    });
}

export function ProtectedRoute({ children, requiredRoles = [] }) {
  const location = useLocation()
  const [token, setToken] = useState(null)
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    const checkToken = async () => {
      const tokenValue = await getTokenWithRetry()
      
      if (!tokenValue) {
        setChecking(false)
        redirectToLogin();
        return
      }
      
      if (!isTokenValid(tokenValue)) {
        clearTokens()
        setChecking(false)
        redirectToLogin();
        return
      }
      
      setToken(tokenValue)
      setChecking(false)
    }
    
    checkToken()
  }, [])

  if (checking) {
    return <Spin fullscreen />
  }

  if (!token) {
    return null // Will redirect via useEffect
  }

  if (requiredRoles.length > 0) {
    const userRole = localStorage.getItem('user_role') || ''
    if (!requiredRoles.includes(userRole)) {
      return (
        <div style={{ padding: 40, textAlign: 'center' }}>
          <h2>权限不足</h2>
          <p>您没有权限访问此页面</p>
        </div>
      )
    }
  }

  return children
}

export function AuthGuard({ children }) {
  const [token, setToken] = useState(null)
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    const checkToken = async () => {
      const tokenValue = await getTokenWithRetry()
      setToken(tokenValue)
      setChecking(false)
      
      if (!tokenValue) {
        redirectToLogin();
      }
    }
    
    checkToken()
  }, [])

  if (checking) {
    return <Spin fullscreen />
  }

  if (!token) {
    return null // Will redirect via useEffect
  }

  return children
}

export function useAuth() {
  return {
    token: getToken(),
    logout: () => {
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
      localStorage.removeItem('user_info')
      localStorage.removeItem('user_role')
      window.location.href = '/'
    }
  }
}

export { getToken, storeToken }

export default ProtectedRoute
