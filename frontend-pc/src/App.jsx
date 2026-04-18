import { useState, useEffect, useRef } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Breadcrumb, Spin } from 'antd'
import {
  
  SettingOutlined,
  
  LogoutOutlined,
  UserOutlined
} from '@ant-design/icons'

import { ProtectedRoute, AuthGuard, getToken, storeToken } from './components/ProtectedRoute'
import { api } from './services/api'
import Dashboard from './pages/Dashboard'
import FinanceConfig from './pages/FinanceConfig'
import WorkOrderList from './pages/WorkOrderList'
import LeaseLedger from './pages/LeaseLedger'
import DepositFlow from './pages/DepositFlow'
import ExpireWarning from './pages/ExpireWarning'
import SupplierDB from './pages/SupplierDB'
import InstrumentStock from './pages/InstrumentStock'
import SiteManagement from './pages/SiteManagement'
import RolePermission from './pages/RolePermission'
import AssetDetail from './pages/AssetDetail'
import ClientManagement from './pages/ClientManagement'
import TenantManagement from './pages/TenantManagement'
import MaintenanceWorkerManagement from './pages/MaintenanceWorkerManagement'
import MaintenanceSessionManagement from './pages/MaintenanceSessionManagement'
import AppealManagement from './pages/AppealManagement'
import WarehouseManagement from './pages/WarehouseManagement'
import UserRental from './pages/UserRental'
import InstrumentListUser from './pages/InstrumentListUser'
import InstrumentDetailUser from './pages/InstrumentDetailUser'
import OrderPayment from './pages/OrderPayment'
import ContractView from './pages/ContractView'
import ReturnProcess from './pages/ReturnProcess'
import CategoryList from './pages/admin/category/List'
import CategoryForm from './pages/admin/category/Form'
import InstrumentList from './pages/admin/instrument/List'
import InstrumentForm from './pages/admin/instrument/Form'
import InstrumentDetail from './pages/admin/instrument/Detail'

import PropertyList from './pages/admin/property/List'
import RentSetting from './pages/admin/inventory/RentSetting'

const { Header, Content, Sider } = Layout

const BRAND_COLOR = '#002140'
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function handleLogout() {
  // Clear localStorage
  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  localStorage.removeItem('user_info')
  localStorage.removeItem('user_role')
  localStorage.removeItem('refresh_token')
  
  // Clear cookies
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  document.cookie = 'refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  
  // Redirect to IAM OAuth page for proper logout
  const iamUrl = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
  const clientId = window.APP_CONFIG?.iamClientId || 'tuneloop'
  const redirectUri = encodeURIComponent(window.location.origin + '/callback')
  window.location.href = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
}

function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const [userInfo, setUserInfo] = useState(null)

  // Track renders and useEffect execution
  const renderCount = useRef(0)
  const effectRunCount = useRef(0)
  const tokenCheckCount = useRef(0)

  renderCount.current++
  console.log('[DEBUG TIMING] ===== MainLayout RENDER #' + renderCount.current + ' at:', new Date().toISOString())
  console.log('[DEBUG TIMING]   userInfo state:', userInfo)
  console.log('[DEBUG TIMING]   localStorage token exists:', !!localStorage.getItem('token'))
  console.log('[DEBUG TIMING]   cookie token exists:', document.cookie.includes('token='))

  useEffect(() => {
    effectRunCount.current++
    console.log('[DEBUG TIMING] ===== useEffect RUN #' + effectRunCount.current + ' at:', new Date().toISOString())
    
    // Try to get user info from JWT token first
    const token = getToken()
    tokenCheckCount.current++
    console.log('[DEBUG TIMING] getToken() attempt #' + tokenCheckCount.current + ':', token ? 'TOKEN_FOUND' : 'TOKEN_NOT_FOUND')
    
    if (token) {
      console.log('[DEBUG TIMING] Token length:', token.length)
      console.log('[DEBUG TIMING] Token includes dots:', token.includes('.'))
    }
    
    if (token && token.includes('.')) {
      console.log('[DEBUG TIMING] Attempting to parse JWT token...')
      try {
        const payload = JSON.parse(atob(token.split('.')[1]))
        console.log('[DEBUG TIMING] JWT payload successfully parsed:', payload)
        console.log('[DEBUG TIMING] Available JWT fields:', Object.keys(payload))
        
        // Try multiple possible field names
        // Handle empty string as well (treat as falsy)
        const name = payload.name || payload.username || payload.preferred_username || 
                     payload.displayName || payload.nickName || payload.nickname || 
                     (payload.is_owner ? '所有者' : '') || ''
        const email = payload.email || payload.mail || ''
        const role = (payload.role || payload.roles || payload.authorities || '').toString().toLowerCase()
        
        console.log('[DEBUG TIMING] Extracted user data - name:', name, 'email:', email, 'role:', role)
        console.log('[DEBUG TIMING] Setting userInfo state...')
        
        // Remove role from payload before spreading to prevent overwrite
        const { role: _payloadRole, ...payloadWithoutRole } = payload
        setUserInfo({
          name,
          email,
          role,
          ...payloadWithoutRole
        })
        localStorage.setItem('user_info', JSON.stringify({ ...payloadWithoutRole, name, email, role }))
        console.log('[DEBUG TIMING] userInfo state SET SUCCESSFULLY')
      } catch (e) {
        console.error('[DEBUG TIMING] FAILED to parse token:', e.message)
      }
    } else {
      console.log('[DEBUG TIMING] No valid token found, falling back to localStorage user_info')
      const info = localStorage.getItem('user_info')
      if (info) {
        console.log('[DEBUG TIMING] Found user_info in localStorage, parsing...')
        try {
          const parsed = JSON.parse(info)
          console.log('[DEBUG TIMING] localStorage user_info parsed:', parsed)
          setUserInfo(parsed)
          console.log('[DEBUG TIMING] userInfo state SET from localStorage')
        } catch (e) {
          console.error('[DEBUG TIMING] FAILED to parse localStorage user_info:', e.message)
        }
      } else {
        console.log('[DEBUG TIMING] No user_info in localStorage either')
      }
    }
    
    console.log('[DEBUG TIMING] ===== useEffect FINISHED at:', new Date().toISOString())
    {
  const items = [
    {
      key: 'browse', icon: <ShoppingOutlined />, label: '乐器浏览',
      children: [
        { key: '/instruments', label: '浏览乐器' }
      ]
    },
    {
      key: 'instruments', icon: <SettingOutlined />, label: '乐器管理',
      children: [
        { key: '/instruments/list', label: '乐器列表' },
        { key: '/instruments/categories', label: '分类设置' },
        { key: '/instruments/properties', label: '属性管理' }
      ]
    },
    {
      key: 'maintenance', icon: <SettingOutlined />, label: '维修管理',
      children: [
        { key: '/maintenance/workers', label: '师傅管理' },
        { key: '/maintenance/sessions', label: '会话管理' }
      ]
    },
    // Inventory monitoring menu - visible to MANAGER roles only
    ...(userInfo ? (() => {
      console.log('[DEBUG TIMING] ===== INVENTORY MENU RENDER CHECK =====')
      console.log('[DEBUG TIMING] Time:', new Date().toISOString())
      console.log('[DEBUG TIMING] userInfo exists:', !!userInfo)
      
      const role = userInfo.role || ''
      console.log('[DEBUG TIMING] userInfo object:', userInfo)
      console.log('[DEBUG TIMING] userInfo.role:', role)
      console.log('[DEBUG TIMING] role === "site_manager":', role === 'site_manager')
      console.log('[DEBUG TIMING] role === "admin":', role === 'admin')
      console.log('[DEBUG TIMING] role === "owner":', role === 'owner')
      
      const shouldShow = role === 'site_manager' || role === 'admin' || role === 'owner'
      console.log('[DEBUG TIMING] shouldShow result:', shouldShow)
      
      if (shouldShow) {
        console.log('[DEBUG TIMING] ===== RETURNING INVENTORY MENU =====')
        return [{
          key: 'inventory', icon: <SettingOutlined />, label: '库存监控',
          children: [
            { key: '/inventory/transfer', label: '库存调拨' },
            { key: '/inventory/rent-setting', label: '租金设定' },
            { key: '/warehouse', label: '库管工作台' }
          ]
        }]
      } else {
        console.log('[DEBUG TIMING] ===== RETURNING EMPTY ARRAY (menu hidden) =====')
        return []
      }
    })() : []),
    {
      key: 'organization', icon: <SettingOutlined />, label: '组织管理',
      children: [
        { key: '/organization/sites', label: '网点管理' }
      ]
    },
    {
      key: 'system', icon: <SettingOutlined />, label: '系统管理',
      children: [
        { key: '/system/clients', label: '客户端管理' },
        { key: '/system/tenants', label: '租户管理' },
        { key: '/appeals', label: '申诉处理' }
      ]
    }
  ]

  const onMenuClick = (e) => {
    navigate(e.key)
  }

  const selectedKeys = [location.pathname]
  
  let openKeys = []
  if (['/', '/instruments/categories', '/instruments/list', '/instruments/properties'].includes(location.pathname) || location.pathname.startsWith('/instruments/')) openKeys = ['instruments']
  else if (['/site/stock', '/instruments/detail'].includes(location.pathname) || location.pathname.startsWith('/site/stock/')) openKeys = ['instruments']
  else if (['/inventory/transfer', '/inventory/rent-setting'].includes(location.pathname) || location.pathname.startsWith('/inventory/')) openKeys = ['inventory']
  else if (['/organization/sites'].includes(location.pathname)) openKeys = ['organization']
  else if (['/system/clients', '/system/tenants'].includes(location.pathname)) openKeys = ['system']

  let pageTitle = '管理后台'
  
  // Make breadcrumb items clickable
  const breadcrumbItems = [
    { title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/'); }}>TuneLoop</a> }
  ]
  
  const routeMap = {
    '/': { title: '仪表盘 (Dashboard)', parent: '乐器管理' },
    '/instruments/categories': { title: '分类设置', parent: '乐器管理' },
    '/instruments/list': { title: '乐器列表', parent: '乐器管理' },
    '/instruments/properties': { title: '属性管理', parent: '乐器管理' },
    '/site/stock': { title: '库存监控', parent: '乐器管理' },
    '/inventory/transfer': { title: '库存调拨', parent: '库存监控' },
    '/inventory/rent-setting': { title: '租金设定', parent: '库存监控' },
    '/organization/sites': { title: '网点管理', parent: '组织管理' },
    '/system/clients': { title: '客户端管理', parent: '系统管理' },
    '/system/tenants': { title: '租户管理', parent: '系统管理' },
  }

  if (routeMap[location.pathname]) {
    pageTitle = routeMap[location.pathname].title
    // 父级菜单可点击（返回首页）
    if (routeMap[location.pathname].parent === '乐器管理') {
      breadcrumbItems.push({ 
        title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/'); }}>{routeMap[location.pathname].parent}</a> 
      })
    } else {
      breadcrumbItems.push({ title: routeMap[location.pathname].parent })
    }
    breadcrumbItems.push({ title: pageTitle })
  } else if (location.pathname.startsWith('/site/stock/')) {
    pageTitle = '资产详情'
    breadcrumbItems.push({ title: '基础数据' }, { title: '乐器库存' }, { title: '资产详情' })
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider 
        width={220} 
        theme="dark"
        style={{ backgroundColor: BRAND_COLOR }}
        collapsible 
        collapsed={collapsed} 
        onCollapse={(value) => setCollapsed(value)}
      >
        <div className="h-16 flex items-center justify-center text-white font-bold text-lg overflow-hidden whitespace-nowrap" style={{ backgroundColor: 'rgba(255,255,255,0.1)' }}>
          {collapsed ? 'TL' : 'TuneLoop 管理后台'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selectedKeys}
          defaultOpenKeys={openKeys}
          items={items}
          onClick={onMenuClick}
          style={{ backgroundColor: BRAND_COLOR }}
        />
      </Sider>
      <Layout>
        <Header className="bg-white px-6 shadow flex justify-between items-center h-16">
          <div>
            <Breadcrumb items={breadcrumbItems} className="text-sm text-gray-500" />
            <h1 className="text-xl font-bold m-0" style={{ color: BRAND_COLOR }}>{pageTitle}</h1>
          </div>
          <div className="flex items-center gap-4">
            {/* DEBUG: Manual load button */}
            <button
              onClick={() => {
                console.log('========== DEBUG BUTTON CLICKED ==========')
                console.log('[DEBUG BUTTON] Time:', new Date().toISOString())
                const token = getToken()
                console.log('[DEBUG BUTTON] 1. Token exists:', !!token)
                console.log('[DEBUG BUTTON] 2. Token value:', token)
                
                if (token && token.includes('.')) {
                  try {
                    const [header, payload, signature] = token.split('.')
                    const decoded = JSON.parse(atob(payload))
                    console.log('[DEBUG BUTTON] 3. JWT Payload:', decoded)
                    console.log('[DEBUG BUTTON] 4. Payload keys:', Object.keys(decoded))
                    console.log('[DEBUG BUTTON] 5. name field:', decoded.name)
                    console.log('[DEBUG BUTTON] 6. username field:', decoded.username)
                    console.log('[DEBUG BUTTON] 7. role field:', decoded.role)
                    console.log('[DEBUG BUTTON] 8. localStorage user_info:', localStorage.getItem('user_info'))
                  } catch (e) {
                    console.log('[DEBUG BUTTON] Error parsing token:', e.message)
                  }
                } else {
                  console.log('[DEBUG BUTTON] 3. No valid token found')
                }
                
                console.log('[DEBUG BUTTON] 9. localStorage user_info:', localStorage.getItem('user_info'))
                console.log('[DEBUG BUTTON] 10. localStorage token:', localStorage.getItem('token'))
                console.log('[DEBUG BUTTON] 11. document.cookie:', document.cookie)
                
                // Also reload user info
                const infoStr = localStorage.getItem('user_info')
                if (infoStr) {
                  try {
                    const info = JSON.parse(infoStr)
                    console.log('[DEBUG BUTTON] 12. Manually loaded userInfo:', info)
                    setUserInfo(info)
                    console.log('[DEBUG BUTTON] 13. userInfo state SET')
                  } catch (e) {
                    console.error('[DEBUG BUTTON] Error loading user_info:', e)
                  }
                }
                console.log('========== DEBUG BUTTON FINISHED ==========')
              }}
              className="px-2 py-1 bg-yellow-500 text-white text-xs rounded"
            >
              调试
            </button>
            
            <div>
              {userInfo ? (
                <div className="flex items-center gap-2">
                  <UserOutlined className="text-gray-600 text-lg" />
                  <span className="text-gray-700 font-medium">
                    {userInfo.name || userInfo.email}
                  </span>
                  <span className="px-2 py-1 bg-blue-100 text-blue-700 rounded text-xs font-medium">
                    {userInfo.role}
                  </span>
                </div>
              ) : (
                <span className="text-gray-500">userInfo is null</span>
              )}
            </div>
            <button
              onClick={handleLogout}
              className="flex items-center gap-1 px-3 py-1 text-sm text-gray-600 hover:text-red-600"
            >
              <LogoutOutlined />
              退出
            </button>
          </div>
        </Header>
        <Content className="p-6 bg-gray-100 overflow-y-auto">
          <Routes>
            <Route path="/callback" element={<OAuthCallback />} />
            <Route path="/" element={<ProtectedRoute><Dashboard /></ProtectedRoute>} />
            <Route path="/assets" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">资产管理</div></ProtectedRoute>} />
            <Route path="/lease/ledger" element={<ProtectedRoute><LeaseLedger /></ProtectedRoute>} />
            <Route path="/finance" element={<ProtectedRoute><FinanceConfig /></ProtectedRoute>} />
            <Route path="/finance/quotes" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">报价单管理</div></ProtectedRoute>} />
            <Route path="/site/stock" element={<ProtectedRoute><InstrumentStock /></ProtectedRoute>} />
            <Route path="/site/stock/:id" element={<ProtectedRoute><AssetDetail /></ProtectedRoute>} />
            <Route path="/organization/sites" element={<ProtectedRoute><SiteManagement /></ProtectedRoute>} />
            <Route path="/workorders" element={<ProtectedRoute><WorkOrderList /></ProtectedRoute>} />
            <Route path="/maintenance/suppliers" element={<ProtectedRoute><SupplierDB /></ProtectedRoute>} />
            <Route path="/maintenance/workers" element={<ProtectedRoute><MaintenanceWorkerManagement /></ProtectedRoute>} />
           <Route path="/maintenance/sessions" element={<ProtectedRoute><MaintenanceSessionManagement /></ProtectedRoute>} />
            <Route path="/appeals" element={<ProtectedRoute><AppealManagement /></ProtectedRoute>} />
            <Route path="/user/appeals" element={<ProtectedRoute><AppealManagement /></ProtectedRoute>} />
            <Route path="/warehouse" element={<ProtectedRoute><WarehouseManagement /></ProtectedRoute>} />
            <Route path="/user/rentals" element={<ProtectedRoute><UserRental /></ProtectedRoute>} />
            <Route path="/instruments" element={<ProtectedRoute><InstrumentListUser /></ProtectedRoute>} />
            <Route path="/instruments/:id" element={<ProtectedRoute><InstrumentDetailUser /></ProtectedRoute>} />
            <Route path="/orders/:id/payment" element={<ProtectedRoute><OrderPayment /></ProtectedRoute>} />
           <Route path="/maintenance/sessions" element={<ProtectedRoute><MaintenanceSessionManagement /></ProtectedRoute>} />
           <Route path="/maintenance/workers" element={<ProtectedRoute><MaintenanceWorkerManagement /></ProtectedRoute>} />
            <Route path="/settings/roles" element={<ProtectedRoute><RolePermission /></ProtectedRoute>} />
            <Route path="/system/clients" element={<ProtectedRoute><ClientManagement /></ProtectedRoute>} />
            <Route path="/system/tenants" element={<ProtectedRoute><TenantManagement /></ProtectedRoute>} />
            <Route path="/appeals" element={<ProtectedRoute><AppealManagement /></ProtectedRoute>} />
            <Route path="/user/appeals" element={<ProtectedRoute><AppealManagement /></ProtectedRoute>} />
            <Route path="/inventory/transfer" element={<ProtectedRoute><InstrumentStock /></ProtectedRoute>} />
            <Route path="/inventory/rent-setting" element={<ProtectedRoute><RentSetting /></ProtectedRoute>} />
            <Route path="/warehouse" element={<ProtectedRoute><WarehouseManagement /></ProtectedRoute>} />
            <Route path="/user/rentals" element={<ProtectedRoute><UserRental /></ProtectedRoute>} />
            <Route path="/instruments" element={<ProtectedRoute><InstrumentListUser /></ProtectedRoute>} />
            <Route path="/instruments/:id" element={<ProtectedRoute><InstrumentDetailUser /></ProtectedRoute>} />
            <Route path="/orders/:id/payment" element={<ProtectedRoute><OrderPayment /></ProtectedRoute>} />
            <Route path="/user/contracts/:id" element={<ProtectedRoute><ContractView /></ProtectedRoute>} />
            <Route path="/user/rentals/:id/return" element={<ProtectedRoute><ReturnProcess /></ProtectedRoute>} />
          
<Route path="/instruments/categories" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id/edit" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/new" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/list" element={<ProtectedRoute><InstrumentList /></ProtectedRoute>} />
             <Route path="/instruments/list/add" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
             <Route path="/instruments/list/edit/:id" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
             <Route path="/instruments/detail/:id" element={<ProtectedRoute><InstrumentDetail /></ProtectedRoute>} />
              <Route path="/instruments/:id/edit" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
              <Route path="/instruments/properties" element={<ProtectedRoute><PropertyList /></ProtectedRoute>} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  )
}

function OAuthCallback() {
  const [loading, setLoading] = useState(true)
  const [errorMsg, setErrorMsg] = useState(null)
  const navigate = useNavigate()
  const exchangedRef = useRef(false)
  
  const getOAuthUrl = () => {
    const config = window.APP_CONFIG?.pc || {
      iamExternalUrl: "http://opencode.linxdeep.com:5552",
      iamClientId: "tuneloop-pc",
      iamRedirectUri: "http://opencode.linxdeep.com:5554/callback"
    }
    const redirectUri = encodeURIComponent(config.iamRedirectUri)
    return `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code`
  }

  useEffect(() => {
    if (exchangedRef.current) return
    exchangedRef.current = true

    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const error = params.get('error')

    if (error) {
      setErrorMsg(`OAuth Error: ${error}`)
      setLoading(false)
      setTimeout(() => {
        window.location.href = getOAuthUrl()
      }, 3000)
      return
    }

    if (!code) {
      setErrorMsg('Missing authorization code')
      setLoading(false)
      setTimeout(() => {
        window.location.href = getOAuthUrl()
      }, 3000)
      return
    }

    const existingToken = getToken()
    if (existingToken) {
      console.log('[OAuth] Token already exists, skipping exchange')
      const redirectTo = sessionStorage.getItem('post_auth_redirect') || '/'
      sessionStorage.removeItem('post_auth_redirect')
      window.location.href = redirectTo
      return
    }

    const exchangeCodeForToken = async () => {
      try {
        setLoading(true)
        
        const response = await fetch(`${API_BASE_URL}/auth/callback`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ code }),
        })

        if (!response.ok) {
          const errorText = await response.text()
          throw new Error(`Token exchange failed: ${response.status} - ${errorText}`)
        }

        const responseData = await response.json()
        const tokenData = responseData.data || responseData
        
        if (tokenData.access_token) {
          const expiresIn = Math.max(tokenData.expires_in || 3600, 60)
          storeToken(tokenData.access_token, expiresIn)
          
          if (tokenData.user_info) {
            localStorage.setItem('user_info', JSON.stringify(tokenData.user_info))
          }
          if (tokenData.role) {
            localStorage.setItem('user_role', tokenData.role)
          }
          
          const redirectTo = sessionStorage.getItem('post_auth_redirect') || '/'
          sessionStorage.removeItem('post_auth_redirect')
          window.location.href = redirectTo
        } else {
          throw new Error('No access token received')
        }
      } catch (error) {
        setLoading(false)
        setErrorMsg(error.message || 'Authentication failed')
        localStorage.removeItem('token')
        localStorage.removeItem('token_expiry')
        setTimeout(() => {
          window.location.href = getOAuthUrl()
        }, 3000)
      }
    }

    exchangeCodeForToken()
  }, [])

  if (errorMsg) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh',
        background: '#f0f2f5',
        flexDirection: 'column'
      }}>
        <h2 style={{ color: 'red' }}>Authentication Error</h2>
        <p>{errorMsg}</p>
        <p>Redirecting to login...</p>
      </div>
    )
  }

  if (loading) {
    return <Spin fullscreen tip="正在完成登录..." />
  }

  return null
}

function App() {
  useEffect(() => {
    api.get('/config')
      .then(data => {
        if (data) {
          window.APP_CONFIG = data
        }
      })
      .catch(err => console.error('Failed to load config:', err))
  }, [])
  
  return (
    <BrowserRouter>
      <MainLayout />
    </BrowserRouter>
  )
}

export default App
