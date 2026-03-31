import { Navigate, useLocation } from 'react-router-dom'
import { Spin } from 'antd'
import { useState, useEffect } from 'react'

const IAM_URL = import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || 'http://opencode.linxdeep.com:5552'
const CLIENT_ID = import.meta.env.VITE_IAM_CLIENT_ID || 'tuneloop'

function getToken() {
  // Check cookies first (same as api.js)
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
  
  // Then check localStorage with expiry
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (!token || !expiry) {
    // Fall back to sessionStorage
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

export function ProtectedRoute({ children, requiredRoles = [] }) {
  const location = useLocation()
  const [token, setToken] = useState(null)
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    const checkToken = async () => {
      const tokenValue = await getTokenWithRetry()
      setToken(tokenValue)
      setChecking(false)
      
      if (!tokenValue) {
        const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
        const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
        window.location.href = authUrl
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
        const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
        const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
        window.location.href = authUrl
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
