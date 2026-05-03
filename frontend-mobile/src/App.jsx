import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { getToken } from './services/api'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'
import Home from './pages/Home'
import Detail from './pages/Detail'
import Checkout from './pages/Checkout'
import Success from './pages/Success'
import Booking from './pages/Booking'
import Profile from './pages/Profile'
import MyService from './pages/MyService'

const getWXConfig = () => {
  return window.APP_CONFIG?.wx || {
    iamExternalUrl: import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '',
    iamClientId: import.meta.env.VITE_IAM_WX_CLIENT_ID || 'tuneloop-wx',
    iamRedirectUri: import.meta.env.VITE_IAM_WX_REDIRECT_URI || ''
  }
}

function storeToken(token, expiresIn = 3600) {
  const expiry = new Date().getTime() + (expiresIn * 1000)
  localStorage.setItem('token', token)
  localStorage.setItem('token_expiry', expiry.toString())
}

function parseJWT(token) {
  if (!token || !token.includes('.')) return {}
  try {
    return JSON.parse(atob(token.split('.')[1]))
  } catch (e) {
    return {}
  }
}

function cachePermissions(claims) {
  const sysPerm = parseInt(claims.sys_perm) || 0
  const cusPerm = parseInt(claims.cus_perm) || 0
  localStorage.setItem('user_sys_perm', sysPerm.toString())
  localStorage.setItem('user_cus_perm', cusPerm.toString())
  localStorage.setItem('user_cus_perm_ext', claims.cus_perm_ext || '')
}

function isNamespaceAdmin() {
  const sysPerm = parseInt(localStorage.getItem('user_sys_perm') || '0')
  const cusPerm = parseInt(localStorage.getItem('user_cus_perm') || '0')
  return sysPerm > 0 && cusPerm === 0
}

const publicRoutes = ['/', '/success', '/callback']

function ProtectedRoute({ children, requireAuth = true }) {
  const token = getToken()
  const location = window.location.pathname
  
  if (!requireAuth) {
    return children
  }
  
  if (!token && !publicRoutes.includes(location)) {
    const config = getWXConfig()
    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)
    const authUrl = `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code`
    window.location.href = authUrl
    return null
  }
  
  return children
}

function OAuthCallback() {
  const [loading] = useState(true)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const error = params.get('error')

    if (error) {
      console.error('OAuth error:', error)
      window.location.href = '/'
      return
    }

    if (!code) {
      window.location.href = '/'
      return
    }

    const exchangeCodeForToken = async () => {
      try {
        const response = await fetch(`${API_BASE_URL}/auth/callback`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ code, client_type: 'wx' }),
        })

        if (!response.ok) {
          throw new Error('Failed to exchange code for token')
        }

        const data = await response.json()
        
        const tokenData = data.data || data
        
        if (tokenData.access_token) {
          storeToken(tokenData.access_token, tokenData.expires_in || 3600)
          
          if (tokenData.user_info) {
            localStorage.setItem('user_info', JSON.stringify(tokenData.user_info))
          }
          
          // Cache permission bitmaps from JWT (#414)
          cachePermissions(parseJWT(tokenData.access_token))
          
          const redirectTo = sessionStorage.getItem('post_auth_redirect') || '/'
          sessionStorage.removeItem('post_auth_redirect')
          window.location.href = redirectTo
        } else {
          throw new Error('No access token received')
        }
      } catch (error) {
        console.error('Token exchange failed:', error)
        window.location.href = '/'
      }
    }

    exchangeCodeForToken()
  }, [])

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <div className="text-lg mb-2">正在完成登录...</div>
        </div>
      </div>
    )
  }

  return null
}

function App() {
  useEffect(() => {
    fetch('/api/config')
      .then(res => res.json())
      .then(data => {
        if (data.code === 20000) {
          window.APP_CONFIG = data.data
        }
      })
      .catch(err => console.error('Failed to load config:', err))
  }, [])
  
  useEffect(() => {
    const token = getToken()
    const location = window.location.pathname
    
    if (!token && !publicRoutes.includes(location)) {
      sessionStorage.setItem('post_auth_redirect', location)
    }
    
    // Cache permissions from existing JWT on app start
    if (token) {
      cachePermissions(parseJWT(token))
    }
  }, [])

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/callback" element={<OAuthCallback />} />
        <Route path="/" element={<ProtectedRoute requireAuth={false}><Home /></ProtectedRoute>} />
        <Route path="/instrument/:id" element={<ProtectedRoute requireAuth={false}><Detail /></ProtectedRoute>} />
        <Route path="/checkout/:id" element={<ProtectedRoute><Checkout /></ProtectedRoute>} />
        <Route path="/success" element={<Success />} />
        <Route path="/booking" element={<ProtectedRoute><Booking /></ProtectedRoute>} />
        <Route path="/booking/:assetId" element={<ProtectedRoute><Booking /></ProtectedRoute>} />
        <Route path="/profile" element={<ProtectedRoute><Profile /></ProtectedRoute>} />
        <Route path="/service" element={<ProtectedRoute><MyService /></ProtectedRoute>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
