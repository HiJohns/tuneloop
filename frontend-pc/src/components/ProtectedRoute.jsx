import { Navigate, useLocation } from 'react-router-dom'
import { Spin } from 'antd'

const IAM_URL = import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || 'http://opencode.linxdeep.com:5552'
const CLIENT_ID = import.meta.env.VITE_IAM_CLIENT_ID || 'tuneloop'

function getToken() {
  const token = localStorage.getItem('token')
  const expiry = localStorage.getItem('token_expiry')
  
  if (!token || !expiry) return null
  
  const now = new Date().getTime()
  const exp = parseInt(expiry)
  
  if (now > exp) {
    localStorage.removeItem('token')
    localStorage.removeItem('token_expiry')
    localStorage.removeItem('user_info')
    return null
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
  const token = getToken()

  if (!token) {
    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
    const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
    return <Spin fullscreen />
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
  const token = getToken()
  if (!token) {
    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
    const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
    return <Spin fullscreen />
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
