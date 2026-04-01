import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { getToken } from './services/api'
import Home from './pages/Home'
import Detail from './pages/Detail'
import Checkout from './pages/Checkout'
import Success from './pages/Success'
import Booking from './pages/Booking'
import Profile from './pages/Profile'
import MyService from './pages/MyService'

const getWXConfig = () => {
  return window.APP_CONFIG?.wx || {
    iamExternalUrl: "http://opencode.linxdeep.com:5552",
    iamClientId: "tuneloop-wx",
    iamRedirectUri: "http://opencode.linxdeep.com:5556/callback"
  }
}

function storeToken(token, expiresIn = 3600) {
  const expiry = new Date().getTime() + (expiresIn * 1000)
  localStorage.setItem('token', token)
  localStorage.setItem('token_expiry', expiry.toString())
}

const publicRoutes = ['/', '/success', '/callback']

function ProtectedRoute({ children }) {
  const token = getToken()
  const location = window.location.pathname
  
  if (!token && !publicRoutes.includes(location)) {
    const config = getWXConfig(); const authUrl = `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)redirect_uri=${redirectUri}    const redirectUri = encodeURIComponent(`${window.location.origin}/callback`)response_type=code`;
    const authUrl = `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
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
          body: JSON.stringify({ code }),
        })

        if (!response.ok) {
          throw new Error('Failed to exchange code for token')
        }

        const data = await response.json()
        
        if (data.access_token) {
          storeToken(data.access_token, data.expires_in || 3600)
          
          if (data.user_info) {
            localStorage.setItem('user_info', JSON.stringify(data.user_info))
          }
          
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
    // 在应用启动时获取配置
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
  }, [])

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/callback" element={<OAuthCallback />} />
        <Route path="/" element={<ProtectedRoute><Home /></ProtectedRoute>} />
        <Route path="/instrument/:id" element={<ProtectedRoute><Detail /></ProtectedRoute>} />
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
