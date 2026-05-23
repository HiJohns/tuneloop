import { useState, useEffect, useRef } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Breadcrumb, Spin } from 'antd'
import { Tabs } from 'antd'
import {
  ShoppingOutlined,
  SettingOutlined,
  LogoutOutlined,
  UserOutlined,
  ToolOutlined,
  AppstoreOutlined,
  TeamOutlined
} from '@ant-design/icons'

import { ProtectedRoute, AuthGuard, getToken, storeToken } from './components/ProtectedRoute'
import { api, initPermissionMapping } from './services/api'
import { checkPermission, isNamespaceAdmin, getNamespaceAdminMenuKeys } from './config/menuPermissions'
import Dashboard from './pages/Dashboard'
import FinanceConfig from './pages/FinanceConfig'
import WorkOrderList from './pages/WorkOrderList'
import LeaseLedger from './pages/LeaseLedger'
import DepositFlow from './pages/DepositFlow'
import ExpireWarning from './pages/ExpireWarning'
import SupplierDB from './pages/SupplierDB'
import InstrumentStock from './pages/InstrumentStock'
import SiteManagement from './pages/SiteManagement'
import StaffManagement from './pages/StaffManagement'
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
import BatchImport from './pages/admin/instrument/BatchImport'
import SiteBulkImport from './pages/SiteBulkImport'
import StaffBulkImport from './pages/StaffBulkImport'

import PropertyList from './pages/admin/property/List'
import Setup from './pages/Setup'
import MerchantManagement from './pages/MerchantManagement'
import AuditLogPage from './pages/System/AuditLogPage'

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
  localStorage.removeItem('user_sys_perm')
  localStorage.removeItem('user_cus_perm')
  localStorage.removeItem('user_cus_perm_ext')
  localStorage.removeItem('user_is_owner')
  localStorage.removeItem('refresh_token')
  
  // Clear cookies
  document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  document.cookie = 'refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
  
  // Redirect to IAM OAuth page for proper logout
  const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
  const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc'
  const redirectUri = encodeURIComponent(window.APP_CONFIG?.pc?.iamRedirectUri || window.location.origin + '/callback')
  window.location.href = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&response_type=code`
}

function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const [userInfo, setUserInfo] = useState(null)

  useEffect(() => {
    const token = getToken()
    
    if (token && token.includes('.')) {
      try {
        const payload = JSON.parse(atob(token.split('.')[1]))
        const name = payload.name || payload.username || payload.preferred_username || 
                     payload.displayName || payload.nickName || payload.nickname ||
                     payload.email || payload.sub?.substring(0, 12) || '用户'
        const email = payload.email || payload.mail || ''
        const role = (payload.role || payload.roles || payload.authorities || '').toString().toLowerCase()
        const roles = Array.isArray(payload.roles) ? payload.roles : [] // Functional roles from IAM #113
        
        // Permission bitmaps from JWT (#414)
        // IAM may emit camelCase (sysPerm) or snake_case (sys_perm), read both
        const sysPerm = parseInt(payload.sys_perm || payload.sysPerm) || 0
        const cusPerm = parseInt(payload.cus_perm || payload.cusPerm) || 0
        const cusPermExt = payload.cus_perm_ext || payload.cusPermExt || ''
        const isOwner = !!(payload.is_owner || payload.isOwner)
        let businessRole = 'site_member'
        if (role === 'owner' || role === 'OWNER') {
          businessRole = isOwner ? 'merchant_admin' : 'site_admin'
        } else if (role === 'admin' || role === 'ADMIN') {
          businessRole = 'site_admin'
        }
        
        // Override businessRole if namespace admin (has sys_perm but no cus_perm)
        if (isNamespaceAdmin(sysPerm, cusPerm)) {
          businessRole = 'system_admin'
        }
        
        const { role: _payloadRole, roles: _payloadRoles, ...payloadWithoutRole } = payload
        setUserInfo({
          name,
          email,
          role,
          roles,
          businessRole,
          sysPerm,
          cusPerm,
          cusPermExt,
          isOwner,
          ...payloadWithoutRole
        })
        localStorage.setItem('user_info', JSON.stringify({ ...payloadWithoutRole, name, email, role, roles, businessRole, sysPerm, cusPerm, cusPermExt, isOwner }))
        localStorage.setItem('user_sys_perm', sysPerm.toString())
        localStorage.setItem('user_cus_perm', cusPerm.toString())
        localStorage.setItem('user_cus_perm_ext', cusPermExt || '')
        localStorage.setItem('user_is_owner', isOwner ? '1' : '0')
        // Store perm_version from JWT payload
        const permVersion = payload.perm_version || payload.permVersion || 0
        localStorage.setItem('perm_version', String(permVersion))
      } catch (e) {
        // ignore parse errors
      }
    } else {
      const info = localStorage.getItem('user_info')
      if (info) {
        try {
          setUserInfo(JSON.parse(info))
        } catch (e) {
          // ignore parse errors
        }
      }
    }
  }, [])
  
  const menuConfig = [
  {
    key: 'instruments',
    icon: <SettingOutlined />,
    label: '乐器管理',
    children: [
      { key: '/instruments/list', label: '乐器列表', permission: { cusPermCodes: ['instrument:create', 'instrument:edit', 'instrument:delete', 'inventory:view', 'instrument:view'] } },
      { key: '/instruments/categories', label: '分类设置', permission: { cusPermCodes: ['category:manage'] } },
      { key: '/instruments/properties', label: '属性管理', permission: { cusPermCodes: ['property:manage'] } }
    ]
  },
  {
    key: 'maintenance',
    icon: <ToolOutlined />,
    label: '维修管理',
    children: [
      { key: '/maintenance/workers', label: '师傅管理', permission: { cusPermCodes: ['maintenance:assign'] } },
      { key: '/maintenance/sessions', label: '会话管理', permission: { cusPermCodes: ['maintenance:view', 'maintenance:assign', 'maintenance:complete'] } }
    ]
  },
  {
    key: 'inventory',
    icon: <AppstoreOutlined />,
    label: '库存监控',
    children: [
      { key: '/inventory/rent-setting', label: '租金设定', permission: { cusPermCodes: ['rent:setting'] } },
      { key: '/warehouse', label: '库管工作台', permission: { cusPermCodes: ['inventory:view', 'inventory:manage'] } }
    ]
  },
  {
    key: 'organization',
    icon: <TeamOutlined />,
    label: '组织管理',
    children: [
      { key: '/organization/sites', label: '网点管理', permission: { sysPermBits: [10], cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'], requireAll: true } },
      { key: '/staff', label: '人员管理', permission: { sysPermBits: [15], cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'], requireAll: true } },
      { key: '/appeals', label: '申诉处理', permission: { cusPermCodes: ['appeal:handle'] } }
    ]
  },
  {
    key: 'system',
    icon: <SettingOutlined />,
    label: '系统管理',
    children: [
      { key: '/merchants', label: '商户管理', permission: { sysPermBits: [5] } },
      { key: '/system/audit-logs', label: '操作日志', permission: { sysPermBits: [5] } }
    ]
  }
]


function onMenuClick(e) {
  navigate(e.key)
}

  const sysPerm = userInfo?.sysPerm || 0
  const cusPerm = userInfo?.cusPerm || 0

  // Permission mapping from localStorage (loaded by api.js)
  const cusPermMapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')

  // Filter menu: children by permission, parent visible if any child visible
  const isOwnerUser = userInfo?.isOwner || false
  const isNsAdmin = isNamespaceAdmin(sysPerm, cusPerm)
  const filteredItems = menuConfig
    .map(item => ({
      ...item,
      children: item.children?.filter(child => {
        const childKey = child.key || ''
        if (isNsAdmin && getNamespaceAdminMenuKeys().includes(childKey)) return true
        if (isOwnerUser && cusPerm === 0 && sysPerm === 0) {
          if (item.key !== 'system') return true
        }
        return checkPermission(child.permission, sysPerm, cusPerm, cusPermMapping)
      }).filter(child => !isNsAdmin || getNamespaceAdminMenuKeys().includes(child.key))
    }))
    .filter(item => item.children && item.children.length > 0);

  const selectedKeys = [location.pathname]
  let openKeys = []
  if (['/', '/instruments/categories', '/instruments/list', '/instruments/properties'].includes(location.pathname) || location.pathname.startsWith('/instruments/')) openKeys = ['instruments']
  else if (['/site/stock', '/instruments/detail'].includes(location.pathname) || location.pathname.startsWith('/site/stock/')) openKeys = ['instruments']
  else if (['/inventory/transfer', '/inventory/rent-setting'].includes(location.pathname) || location.pathname.startsWith('/inventory/')) openKeys = ['inventory']
  else if (['/organization/sites', '/staff', '/appeals'].includes(location.pathname)) openKeys = ['organization']
  else if (['/merchants', '/system/audit-logs'].includes(location.pathname)) openKeys = ['system']


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
    '/inventory/rent-setting': { title: '租金设定', parent: '库存监控' },
    '/organization/sites': { title: '网点管理', parent: '组织管理' },
    '/merchants': { title: '商户管理', parent: '系统管理' },
    '/system/audit-logs': { title: '操作日志', parent: '系统管理' },
    '/staff': { title: '人员管理', parent: '组织管理' },
    '/appeals': { title: '申诉处理', parent: '组织管理' },

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
          items={filteredItems}
          onClick={onMenuClick}
          style={{ backgroundColor: BRAND_COLOR }}
        />
      </Sider>
      <Layout>
        <Header className="bg-white px-6 shadow flex justify-between items-center h-16">
          <div>
            <Breadcrumb items={breadcrumbItems} className="text-sm text-gray-500" />
          </div>
          <div className="flex items-center gap-4">
            <div>
              {userInfo ? (
                <div className="flex items-center gap-2">
                  <UserOutlined className="text-gray-600 text-lg" />
                  <span className="text-gray-700 font-medium">
                    {userInfo.name || userInfo.email}
                  </span>
                  <span className="px-2 py-1 bg-blue-100 text-blue-700 rounded text-xs font-medium">
                    {userInfo.businessRole === 'system_admin' ? '命名空间管理员' : userInfo.role}
                  </span>
                </div>
              ) : (
                <span className="text-gray-500">用户信息加载中...</span>
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
            <Route path="/setup" element={<Setup />} />
            <Route path="/" element={<ProtectedRoute><Dashboard /></ProtectedRoute>} />
            <Route path="/assets" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">资产管理</div></ProtectedRoute>} />
            <Route path="/lease/ledger" element={<ProtectedRoute><LeaseLedger /></ProtectedRoute>} />
            <Route path="/finance" element={<ProtectedRoute><FinanceConfig /></ProtectedRoute>} />
            <Route path="/finance/quotes" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">报价单管理</div></ProtectedRoute>} />
            <Route path="/site/stock" element={<ProtectedRoute><InstrumentStock /></ProtectedRoute>} />
            <Route path="/site/stock/:id" element={<ProtectedRoute><AssetDetail /></ProtectedRoute>} />
            <Route path="/organization/sites" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10], cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'], requireAllGroups: true }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/merchants" element={<ProtectedRoute requiredPermission={{ sysPermBits: [5] }}><MerchantManagement /></ProtectedRoute>} />
            <Route path="/system/audit-logs" element={<ProtectedRoute requiredPermission={{ sysPermBits: [5] }}><AuditLogPage /></ProtectedRoute>} />
            <Route path="/staff" element={<ProtectedRoute requiredPermission={{ sysPermBits: [15], cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'], requireAllGroups: true }}><StaffManagement /></ProtectedRoute>} />
            <Route path="/workorders" element={<ProtectedRoute><WorkOrderList /></ProtectedRoute>} />
            <Route path="/maintenance/suppliers" element={<ProtectedRoute><SupplierDB /></ProtectedRoute>} />
            <Route path="/maintenance/sessions" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['maintenance:view', 'maintenance:assign', 'maintenance:complete'] }}><MaintenanceSessionManagement /></ProtectedRoute>} />
            <Route path="/maintenance/workers" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['maintenance:assign'] }}><MaintenanceWorkerManagement /></ProtectedRoute>} />
            <Route path="/settings/roles" element={<ProtectedRoute requiredPermission={{ sysPermBits: [20] }}><RolePermission /></ProtectedRoute>} />
            <Route path="/system/clients" element={<ProtectedRoute requiredPermission={{ sysPermBits: [0] }}><ClientManagement /></ProtectedRoute>} />
            <Route path="/system/tenants" element={<ProtectedRoute requiredPermission={{ sysPermBits: [6] }}><TenantManagement /></ProtectedRoute>} />
            <Route path="/appeals" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['appeal:handle'] }}><AppealManagement /></ProtectedRoute>} />
            <Route path="/user/appeals" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['appeal:handle'] }}><AppealManagement /></ProtectedRoute>} />
            <Route path="/inventory/transfer" element={<ProtectedRoute><InstrumentStock /></ProtectedRoute>} />
            <Route path="/inventory/rent-setting" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['rent:setting'] }}><RentSetting /></ProtectedRoute>} />
            <Route path="/warehouse" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['inventory:view', 'inventory:manage'] }}><WarehouseManagement /></ProtectedRoute>} />
            <Route path="/user/rentals" element={<ProtectedRoute><UserRental /></ProtectedRoute>} />
            <Route path="/instruments" element={<ProtectedRoute><InstrumentListUser /></ProtectedRoute>} />
            <Route path="/instruments/:id" element={<ProtectedRoute><InstrumentDetailUser /></ProtectedRoute>} />
            <Route path="/orders/:id/payment" element={<ProtectedRoute><OrderPayment /></ProtectedRoute>} />
            <Route path="/user/contracts/:id" element={<ProtectedRoute><ContractView /></ProtectedRoute>} />
            <Route path="/user/rentals/:id/return" element={<ProtectedRoute><ReturnProcess /></ProtectedRoute>} />
          
            <Route path="/instruments/categories" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['category:manage'] }}><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id/edit" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/new" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/list" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:create', 'instrument:edit', 'instrument:delete', 'inventory:view', 'instrument:view'] }}><InstrumentList /></ProtectedRoute>} />
             <Route path="/instruments/list/add" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
             <Route path="/instruments/list/edit/:id" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
              <Route path="/instruments/detail/:id" element={<ProtectedRoute><InstrumentDetail /></ProtectedRoute>} />
               <Route path="/instruments/batch-import" element={<ProtectedRoute><BatchImport /></ProtectedRoute>} />
                <Route path="/instruments/:id/edit" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
                <Route path="/instruments/properties" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['property:manage'] }}><PropertyList /></ProtectedRoute>} />
                <Route path="/organization/sites/bulk-import" element={<ProtectedRoute requiredPermission={{ sysPermBits: [12] }}><SiteBulkImport /></ProtectedRoute>} />
                <Route path="/staff/bulk-import" element={<ProtectedRoute requiredPermission={{ sysPermBits: [17] }}><StaffBulkImport /></ProtectedRoute>} />
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
      iamExternalUrl: import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '',
      iamClientId: import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc',
      iamRedirectUri: import.meta.env.VITE_IAM_PC_REDIRECT_URI || ''
    }
    const redirectUri = encodeURIComponent(config.iamRedirectUri)
    return `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code`
    return url
  }

  useEffect(() => {
    if (exchangedRef.current) return
    exchangedRef.current = true

    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const error = params.get('error')

    if (error) {
      setErrorMsg(`OAuth 错误: ${error}`)
      setLoading(false)
      setTimeout(() => {
        window.location.href = getOAuthUrl()
      }, 3000)
      return
    }

    if (!code) {
      setErrorMsg('缺少授权码')
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

        // Clear code from URL to prevent re-submission on refresh
        if (window.history && window.history.replaceState) {
          const url = new URL(window.location)
          url.searchParams.delete('code')
          url.searchParams.delete('state')
          window.history.replaceState({}, '', url)
        }

        if (!response.ok) {
          const errorText = await response.text()
          throw new Error(`Token exchange failed: ${response.status} - ${errorText}`)
        }

        const responseData = await response.json()
        const tokenData = responseData.data || responseData

        if (tokenData.relogin) {
          console.log('[OAuth] Code already used, redirecting to IAM for new code')
          window.location.href = getOAuthUrl()
          return
        }
        
        if (tokenData.access_token) {
          const expiresIn = Math.max(tokenData.expires_in || 3600, 60)
          storeToken(tokenData.access_token, expiresIn)
          
          if (tokenData.refresh_token) {
            localStorage.setItem('refresh_token', tokenData.refresh_token)
          }
          
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
        setErrorMsg(error.message || '认证失败')
        localStorage.removeItem('token')
        localStorage.removeItem('token_expiry')
        // Don't auto-redirect — let user see the error and manually retry
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
        flexDirection: 'column',
        gap: '16px'
      }}>
        <h2 style={{ color: 'red' }}>Authentication Error</h2>
        <p style={{ maxWidth: '600px', wordBreak: 'break-all', textAlign: 'center' }}>{errorMsg}</p>
        <p style={{ color: '#666' }}>Check browser console (F12) for debug details</p>
        <button onClick={() => window.location.href = getOAuthUrl()} style={{ padding: '10px 24px', cursor: 'pointer' }}>
          Retry Login
        </button>
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
          window.APP_CONFIG = data?.data || data
        }
      })
      .catch(err => console.error('Failed to load config:', err))
    initPermissionMapping()
  }, [])
  
  return (
    <BrowserRouter>
      <MainLayout />
    </BrowserRouter>
  )
}

export default App
