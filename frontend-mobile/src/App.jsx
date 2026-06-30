import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { message } from 'antd'
import { getToken, initPermissionMapping, publicRoutes, request } from './services/api'
import { storage, session, navigation, env } from './platform'
import { initializeApp, storeToken, parseJWT, cachePermissions, getWXConfig, showLoginReason, setInitDeps } from './platform/init'

setInitDeps(initPermissionMapping, publicRoutes)

import Home from './pages/Home'
import Detail from './pages/Detail'
import Checkout from './pages/Checkout'
import Success from './pages/Success'
import Booking from './pages/Booking'
import Profile from './pages/Profile'
import ReceiveConfirm from './pages/ReceiveConfirm'
import ReturnConfirm from './pages/ReturnConfirm'
import MyService from './pages/MyService'
import MyLeases from './pages/MyLeases'
import LeaseHistory from './pages/LeaseHistory'
import Messages from './pages/Messages'
import MessageDetail from './pages/MessageDetail'
import PaymentComplete from './pages/PaymentComplete'
import StaffInstruments from './pages/StaffInstruments'
import StaffInstrumentDetail from './pages/StaffInstrumentDetail'
import StaffInstrumentForm from './pages/StaffInstrumentForm'
import StaffReceiveConfirm from './pages/StaffReceiveConfirm'
import ShippingInterface from './pages/ShippingInterface'
import ReceivingInterface from './pages/ReceivingInterface'
import Cart from './pages/Cart'
import MaintenanceProgress from './pages/MaintenanceProgress'
import SiteDetail from './pages/SiteDetail'
import MyContracts from './pages/MyContracts'
import StaffOrders from './pages/StaffOrders'
import MyRepairs from './pages/MyRepairs'
import RepairWorkflow from './pages/RepairWorkflow'
import RepairScan from './pages/RepairScan'
import OrderDetail from './pages/OrderDetail'
import Onboarding from './pages/Onboarding'
import ReturnSettlement from './pages/ReturnSettlement'
import MembershipCenter from './pages/MembershipCenter'

function ProtectedRoute({ children, requireAuth = true }) {
  const token = getToken()
  const location = navigation.getCurrentPath()

  if (!requireAuth) {
    session.removeItem('guest_degradation')
    return children
  }

  if (!token && !publicRoutes.includes(location)) {
    if (session.getItem('guest_degradation')) {
      navigation.redirect('/')
      return null
    }
    session.setItem('post_auth_redirect', location)
    const config = getWXConfig()
    const redirectUri = encodeURIComponent(`${navigation.getOrigin()}/callback`)
    const authUrl = `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code`
    navigation.redirect(authUrl)
    return null
  }

  return children
}

let oauthCallbackExecuted = false

function OAuthCallback() {
  const [loading] = useState(true)

  useEffect(() => {
    if (oauthCallbackExecuted) return
    oauthCallbackExecuted = true

    const params = navigation.getQueryParams()
    const code = params.code
    const error = params.error

    if (error) {
      console.error('OAuth error:', error)
      navigation.redirect('/')
      return
    }

    if (!code) {
      navigation.redirect('/')
      return
    }

    const exchangeCodeForToken = async () => {
      try {
        const result = await request('/auth/callback', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ code, client_type: 'wx' }),
        })

        const tokenData = result.data || result

        if (tokenData.access_token) {
          storeToken(tokenData.access_token, tokenData.expires_in || 3600, tokenData.refresh_token)

          if (tokenData.user_info) {
            storage.setJSON('user_info', tokenData.user_info)
          }

          cachePermissions(parseJWT(tokenData.access_token))

          const reason = session.getItem('login_reason')
          if (reason) {
            session.removeItem('login_reason')
            session.setItem('show_login_reason', reason)
          }

          let redirectTo = session.getItem('post_auth_redirect') || '/onboarding'
          session.removeItem('post_auth_redirect')
          navigation.redirect(redirectTo)
        } else {
          throw new Error('No access token in response')
        }
      } catch (error) {
        console.error('Token exchange failed:', error)
        navigation.redirect('/')
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
    initializeApp()

    const reason = showLoginReason()
    if (reason) {
      message.info(reason)
    }
  }, [])

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/callback" element={<OAuthCallback />} />
        <Route path="/" element={<ProtectedRoute requireAuth={false}><Home /></ProtectedRoute>} />
        <Route path="/instrument/:id" element={<ProtectedRoute requireAuth={false}><Detail /></ProtectedRoute>} />
        <Route path="/checkout" element={<ProtectedRoute requireAuth={false}><Checkout /></ProtectedRoute>} />
        <Route path="/checkout/:id" element={<ProtectedRoute><Checkout /></ProtectedRoute>} />
        <Route path="/success" element={<Success />} />
        <Route path="/booking" element={<ProtectedRoute><Booking /></ProtectedRoute>} />
        <Route path="/booking/:assetId" element={<ProtectedRoute><Booking /></ProtectedRoute>} />
        <Route path="/profile" element={<ProtectedRoute><Profile /></ProtectedRoute>} />
        <Route path="/receive/:orderId" element={<ProtectedRoute><ReceiveConfirm /></ProtectedRoute>} />
        <Route path="/return/:orderId" element={<ProtectedRoute><ReturnConfirm /></ProtectedRoute>} />
        <Route path="/service" element={<ProtectedRoute><MyService /></ProtectedRoute>} />
        <Route path="/my-leases" element={<ProtectedRoute><MyLeases /></ProtectedRoute>} />
        <Route path="/lease-history" element={<ProtectedRoute><LeaseHistory /></ProtectedRoute>} />
        <Route path="/my-contracts" element={<ProtectedRoute><MyContracts /></ProtectedRoute>} />
        <Route path="/messages" element={<ProtectedRoute><Messages /></ProtectedRoute>} />
        <Route path="/messages/:id" element={<ProtectedRoute><MessageDetail /></ProtectedRoute>} />
        <Route path="/payment-complete" element={<ProtectedRoute><PaymentComplete /></ProtectedRoute>} />
<Route path="/staff/instrument/new" element={<ProtectedRoute><StaffInstrumentForm /></ProtectedRoute>} />
<Route path="/staff/instruments" element={<ProtectedRoute><StaffInstruments /></ProtectedRoute>} />
<Route path="/staff/instrument/:id" element={<ProtectedRoute><StaffInstrumentDetail /></ProtectedRoute>} />
        <Route path="/staff/receiving/:orderId" element={<ProtectedRoute><StaffReceiveConfirm /></ProtectedRoute>} />
        <Route path="/staff/shipping" element={<ProtectedRoute><ShippingInterface /></ProtectedRoute>} />
        <Route path="/staff/receiving" element={<ProtectedRoute><ReceivingInterface /></ProtectedRoute>} />
        <Route path="/staff/orders" element={<ProtectedRoute><StaffOrders /></ProtectedRoute>} />
        <Route path="/staff/orders/:id" element={<ProtectedRoute><OrderDetail /></ProtectedRoute>} />
        <Route path="/my-repairs" element={<ProtectedRoute><MyRepairs /></ProtectedRoute>} />
        <Route path="/repair" element={<ProtectedRoute><RepairWorkflow /></ProtectedRoute>} />
        <Route path="/staff/repair-scan" element={<ProtectedRoute><RepairScan /></ProtectedRoute>} />
        <Route path="/order/:id" element={<ProtectedRoute><OrderDetail /></ProtectedRoute>} />
        <Route path="/cart" element={<ProtectedRoute requireAuth={false}><Cart /></ProtectedRoute>} />
        <Route path="/maintenance/:id" element={<ProtectedRoute><MaintenanceProgress /></ProtectedRoute>} />
        <Route path="/site/:id" element={<ProtectedRoute requireAuth={false}><SiteDetail /></ProtectedRoute>} />
        <Route path="/onboarding" element={<ProtectedRoute><Onboarding /></ProtectedRoute>} />
        <Route path="/return-settlement/:orderId" element={<ProtectedRoute><ReturnSettlement /></ProtectedRoute>} />
        <Route path="/membership" element={<ProtectedRoute><MembershipCenter /></ProtectedRoute>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
