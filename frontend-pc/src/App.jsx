import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Breadcrumb, Spin } from 'antd'
import {
  AppstoreOutlined,
  SettingOutlined,
  DatabaseOutlined,
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

const { Header, Content, Sider } = Layout

const BRAND_COLOR = '#002140'
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

function handleLogout() {
  localStorage.removeItem('token')
  localStorage.removeItem('token_expiry')
  localStorage.removeItem('user_info')
  localStorage.removeItem('user_role')
  window.location.href = '/'
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
      key: 'core', icon: <AppstoreOutlined />, label: '核心业务',
      children: [
        { key: '/', label: '仪表盘 (Dashboard)' },
        { key: '/assets', label: '资产管理' },
        { key: '/lease/ledger', label: '租约管理' }
      ]
    },
    {
      key: 'config', icon: <SettingOutlined />, label: '资产配置',
      children: [
        { key: '/finance', label: '乐器定价配置' },
        { key: '/finance/quotes', label: '报价单管理' }
      ]
    },
    {
      key: 'data', icon: <DatabaseOutlined />, label: '基础数据',
      children: [
        { key: '/site/stock', label: '乐器库存' },
        { key: '/site/management', label: 'Site网点管理' }
      ]
    }
  ]

  const onMenuClick = (e) => {
    navigate(e.key)
  }

  const selectedKeys = [location.pathname]
  
  let openKeys = []
  if (['/', '/assets', '/lease/ledger'].includes(location.pathname)) openKeys = ['core']
  else if (['/finance', '/finance/quotes'].includes(location.pathname)) openKeys = ['config']
  else if (['/site/stock', '/site/management'].includes(location.pathname)) openKeys = ['data']
  else if (location.pathname.startsWith('/site/stock/')) openKeys = ['data']

  let pageTitle = '管理后台'
  let breadcrumbItems = [{ title: 'TuneLoop' }]
  
  const routeMap = {
    '/': { title: '仪表盘 (Dashboard)', parent: '核心业务' },
    '/assets': { title: '资产管理', parent: '核心业务' },
    '/lease/ledger': { title: '租约管理', parent: '核心业务' },
    '/finance': { title: '乐器定价配置', parent: '资产配置' },
    '/finance/quotes': { title: '报价单管理', parent: '资产配置' },
    '/site/stock': { title: '乐器库存', parent: '基础数据' },
    '/site/management': { title: 'Site网点管理', parent: '基础数据' }
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
            <Route path="/site/management" element={<ProtectedRoute><SiteManagement /></ProtectedRoute>} />
            <Route path="/lease/deposit" element={<ProtectedRoute><DepositFlow /></ProtectedRoute>} />
            <Route path="/lease/warning" element={<ProtectedRoute><ExpireWarning /></ProtectedRoute>} />
            <Route path="/workorders" element={<ProtectedRoute><WorkOrderList /></ProtectedRoute>} />
            <Route path="/maintenance/suppliers" element={<ProtectedRoute><SupplierDB /></ProtectedRoute>} />
            <Route path="/settings/roles" element={<ProtectedRoute><RolePermission /></ProtectedRoute>} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  )
}

function OAuthCallback() {
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const error = params.get('error')

    if (error) {
      console.error('OAuth error:', error)
      navigate('/')
      return
    }

    if (!code) {
      navigate('/')
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
          if (data.role) {
            localStorage.setItem('user_role', data.role)
          }
          
          const redirectTo = sessionStorage.getItem('post_auth_redirect') || '/'
          sessionStorage.removeItem('post_auth_redirect')
          navigate(redirectTo)
        } else {
          throw new Error('No access token received')
        }
      } catch (error) {
        console.error('Token exchange failed:', error)
        navigate('/')
      }
    }

    exchangeCodeForToken()
  }, [navigate])

  if (loading) {
    return <Spin fullscreen tip="正在完成登录..." />
  }

  return null
}

function App() {
  return (
    <BrowserRouter>
      <MainLayout />
    </BrowserRouter>
  )
}

export default App
