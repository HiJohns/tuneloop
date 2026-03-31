import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Breadcrumb, Spin } from 'antd'
import {
  
  SettingOutlined,
  
  LogoutOutlined
} from '@ant-design/icons'

import { ProtectedRoute, AuthGuard, getToken, storeToken } from './components/ProtectedRoute'
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
import CategoryList from './pages/admin/category/List'
import CategoryForm from './pages/admin/category/Form'
import InstrumentList from './pages/admin/instrument/List'
import InstrumentForm from './pages/admin/instrument/Form'
import InstrumentDetail from './pages/admin/instrument/Detail'

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

  useEffect(() => {
    const info = localStorage.getItem('user_info')
    if (info) {
      try {
        setUserInfo(JSON.parse(info))
      } catch (e) {
        console.error('Failed to parse user info:', e)
      }
    }
  }, [])

  const items = [
    {
      key: 'instruments', icon: <SettingOutlined />, label: '乐器管理',
      children: [
        { key: '/instruments/categories', label: '分类设置' },
        { key: '/instruments/list', label: '乐器列表 🔥' },
        { key: '/site/stock', label: '库存监控' }
      ]
    },
    {
      key: 'system', icon: <SettingOutlined />, label: '系统管理',
      children: [
        { key: '/system/clients', label: '客户端管理' },
        { key: '/system/tenants', label: '租户管理' }
      ]
    }
  ]

  const onMenuClick = (e) => {
    navigate(e.key)
  }

  const selectedKeys = [location.pathname]
  
  let openKeys = []
  if (['/', '/instruments/categories', '/instruments/list'].includes(location.pathname)) openKeys = ['instruments']
  else if (['/site/stock', '/instruments/detail'].includes(location.pathname) || location.pathname.startsWith('/site/stock/')) openKeys = ['instruments']
  else if (['/system/clients', '/system/tenants'].includes(location.pathname)) openKeys = ['system']

  let pageTitle = '管理后台'
  let breadcrumbItems = [{ title: 'TuneLoop' }]
  
  const routeMap = {
    '/': { title: '仪表盘 (Dashboard)', parent: '乐器管理' },
    '/instruments/categories': { title: '分类设置', parent: '乐器管理' },
    '/instruments/list': { title: '乐器列表', parent: '乐器管理' },
    '/site/stock': { title: '库存监控', parent: '乐器管理' },
    '/system/clients': { title: '客户端管理', parent: '系统管理' },
    '/system/tenants': { title: '租户管理', parent: '系统管理' },
  }

  if (routeMap[location.pathname]) {
    pageTitle = routeMap[location.pathname].title
    breadcrumbItems.push({ title: routeMap[location.pathname].parent })
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
            {userInfo && (
              <span className="text-gray-600">
                {userInfo.name || userInfo.email}
                <span className="ml-2 px-2 py-1 bg-gray-100 rounded text-xs">{userInfo.role}</span>
              </span>
            )}
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
            <Route path="/workorders" element={<ProtectedRoute><WorkOrderList /></ProtectedRoute>} />
            <Route path="/maintenance/suppliers" element={<ProtectedRoute><SupplierDB /></ProtectedRoute>} />
            <Route path="/settings/roles" element={<ProtectedRoute><RolePermission /></ProtectedRoute>} />
            <Route path="/system/clients" element={<ProtectedRoute><ClientManagement /></ProtectedRoute>} />
            <Route path="/system/tenants" element={<ProtectedRoute><TenantManagement /></ProtectedRoute>} />
          
            <Route path="/instruments/categories" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/edit/:id" element={<ProtectedRoute><CategoryForm /></ProtectedRoute>} />
            <Route path="/instruments/categories/add" element={<ProtectedRoute><CategoryForm /></ProtectedRoute>} />
            <Route path="/instruments/list" element={<ProtectedRoute><InstrumentList /></ProtectedRoute>} />
            <Route path="/instruments/list/add" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
            <Route path="/instruments/list/edit/:id" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
            <Route path="/instruments/detail/:id" element={<ProtectedRoute><InstrumentDetail /></ProtectedRoute>} />
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
  
  const getOAuthUrl = () => {
    const IAM_URL = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552'
    const CLIENT_ID = window.APP_CONFIG?.iamClientId || 'tuneloop'
    const redirectUri = encodeURIComponent(window.location.origin + '/callback')
    return `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`
  }

  useEffect(() => {
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

    const exchangeCodeForToken = async (retryCount = 0) => {
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
          // If 500 error and haven't retried, try once more
          if (response.status === 500 && retryCount < 1) {
            console.log('[OAuth] Got 500, retrying...')
            await new Promise(resolve => setTimeout(resolve, 1000))
            return exchangeCodeForToken(retryCount + 1)
          }
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
          
          setLoading(false)
          const redirectTo = sessionStorage.getItem('post_auth_redirect') || '/'
          sessionStorage.removeItem('post_auth_redirect')
          navigate(redirectTo, { replace: true })
        } else {
          throw new Error('No access token received')
        }
      } catch (error) {
        setLoading(false)
        setErrorMsg(error.message || 'Authentication failed')
        setTimeout(() => {
          window.location.href = getOAuthUrl()
        }, 5000)
      }
    }

    exchangeCodeForToken()
  }, [navigate])

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
  
  return (
    <BrowserRouter>
      <MainLayout />
    </BrowserRouter>
  )
}

export default App
