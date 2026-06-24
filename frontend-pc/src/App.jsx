import { useState, useEffect, useRef } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation, useSearchParams } from 'react-router-dom'
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
import { SysPermBits, checkPermission, isNamespaceAdmin, getNamespaceAdminMenuKeys } from './config/menuPermissions'
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
import StaffEdit from './pages/StaffEdit'
import StaffResetPassword from './pages/StaffResetPassword'
import PermissionManage from './pages/admin/PermissionManage'
import AssetDetail from './pages/AssetDetail'
import ClientManagement from './pages/ClientManagement'
import TenantManagement from './pages/TenantManagement'
import AppealManagement from './pages/AppealManagement'
import MaintenanceSessionManagement from './pages/MaintenanceSessionManagement'
import WarehouseManagement from './pages/WarehouseManagement'
import LogoutPage from './pages/LogoutPage'
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
import UserProfile from './pages/UserProfile'
import ChangePassword from './pages/ChangePassword'

import PropertyList from './pages/admin/property/List'
import Setup from './pages/Setup'
import MerchantManagement from './pages/MerchantManagement'
import AuditLogPage from './pages/System/AuditLogPage'
import BannerManagePage from './pages/System/BannerManagePage'

import RentSetting from './pages/admin/inventory/RentSetting'
import MerchantPricingConfig from './pages/admin/pricing/MerchantPricingConfig'

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
  
    // Redirect to IAM login for proper logout with redirect back
    const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
    const clientId = window.APP_CONFIG?.pc?.iamClientId
    if (!clientId) { alert('无法获取配置，请刷新页面重试'); return }
    const redirectUri = encodeURIComponent(window.location.origin + '/callback')
    window.location.href = iamUrl + '/oauth/authorize?prompt=login&client_id=' + clientId + '&redirect_uri=' + redirectUri + '&response_type=code&noRegister=1'
}

function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const [userInfo, setUserInfo] = useState(null)
  const [showExpiryWarning, setShowExpiryWarning] = useState(false)
  const [searchParams] = useSearchParams()
  const isFirstLogin = location.pathname === '/user/change-password' && searchParams.get('first_login') === '1'

  const redirectToIAMLogin = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('token_expiry')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user_info')
    localStorage.removeItem('user_role')
    localStorage.removeItem('user_is_owner')
    const cookieDomains = ['', '.cadenzayueqi.com', '.linxdeep.com']
    cookieDomains.forEach(domain => {
      const path = domain ? `; domain=${domain}` : ''
      document.cookie = `token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
      document.cookie = `refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
    })
    localStorage.setItem('logout_reason', 'session_expired')
    window.location.href = '/logout'
  }

  // Session expiry warning — check every 30s
  useEffect(() => {
    const checkExpiry = () => {
      if (location.pathname === '/callback') return
      const token = getToken()
      if (!token) {
        redirectToIAMLogin()
        return
      }
      try {
        const payload = JSON.parse(atob(token.split('.')[1]))
        const timeLeft = payload.exp * 1000 - Date.now()
        if (timeLeft < 60000 && timeLeft > 0) {
          setShowExpiryWarning(true)
        }
      } catch (e) {}
    }
    checkExpiry()
    const timer = setInterval(checkExpiry, 30000)
    return () => clearInterval(timer)
  }, [])

  // 60s countdown — force redirect when warning is shown
  useEffect(() => {
    if (showExpiryWarning) {
      const redirectTimer = setTimeout(() => {
        redirectToIAMLogin()
      }, 60000)
      return () => clearTimeout(redirectTimer)
    }
  }, [showExpiryWarning])

  const handleExtendSession = async () => {
    setShowExpiryWarning(false)
    try {
      await api.get('/config')
    } catch (e) {
      // DEBUG: temporarily disabled
      // redirectToIAMLogin()
    }
  }

  useEffect(() => {
    const token = getToken()
    console.log('%c[APP DEBUG] userInfo useEffect', 'color: teal;', {
      hasToken: !!token,
      tokenLen: token?.length,
      isJWT: token?.includes('.'),
      hasUserInfo: !!localStorage.getItem('user_info'),
      userInfoLen: localStorage.getItem('user_info')?.length,
      timestamp: new Date().toISOString()
    })

    if (token && token.includes('.')) {
      console.log('%c[APP DEBUG] attempting JWT parse...', 'color: teal;')
      try {
        const payload = JSON.parse(atob(token.split('.')[1]))

        console.log('%c[APP DEBUG] JWT success', 'color: green;', {
          sub: payload.sub,
          tid: payload.tid,
          oid: payload.oid,
          sys_perm: payload.sys_perm || payload.sysPerm,
          cus_perm: payload.cus_perm || payload.cusPerm,
          roles: payload.roles,
          role: payload.role,
          is_owner: payload.is_owner || payload.isOwner,
          timestamp: new Date().toISOString()
        })

        const name = payload.name || payload.username || payload.preferred_username || 
                     payload.displayName || payload.nickName || payload.nickname
        const email = payload.email || payload.mail || ''

        const userName = name || email || payload.sub?.substring(0, 8) || '用户'
        const userId = payload.sub || ''
        const role = (payload.role || payload.roles || payload.authorities || '').toString().toLowerCase()
        
        // Permission bitmaps from JWT (#414)
        // IAM may emit camelCase (sysPerm) or snake_case (sys_perm), read both
        const sysPerm = parseInt(payload.sys_perm || payload.sysPerm) || 0
        const cusPerm = parseInt(payload.cus_perm || payload.cusPerm) || 0
        const cusPermExt = payload.cus_perm_ext || payload.cusPermExt || ''
        const tid = payload.tid || ''
        const oid = payload.oid || ''
        const isOwner = !!(payload.is_owner || payload.isOwner)
        const roles = Array.isArray(payload.roles) ? payload.roles : []
        let businessRole = 'site_member'
        if (!tid || !oid || roles.includes('namespace_admin')) {
          businessRole = 'system_admin'
        } else if (tid === oid) {
          businessRole = 'merchant_admin'
        } else if (role === 'admin' || role === 'owner' || role === 'manager') {
          businessRole = 'site_admin'
        } else {
          businessRole = 'site_member'
        }
        
        const { role: _payloadRole, roles: _payloadRoles, ...payloadWithoutRole } = payload
        setUserInfo({
          name: userName,
          email,
          role,
          roles,
          businessRole,
          sysPerm,
          cusPerm,
          cusPermExt,
          isOwner,
          tid,
          oid,
          ...payloadWithoutRole
        })
        localStorage.setItem('user_info', JSON.stringify({ ...payloadWithoutRole, name, email, role, roles, businessRole, sysPerm, cusPerm, cusPermExt, isOwner, tid, oid }))
        localStorage.setItem('user_sys_perm', sysPerm.toString())
        localStorage.setItem('user_cus_perm', cusPerm.toString())
        localStorage.setItem('user_cus_perm_ext', cusPermExt || '')
        localStorage.setItem('user_business_role', businessRole)
        const permVersion = payload.perm_version || payload.permVersion || 0
        localStorage.setItem('perm_version', String(permVersion))

        // If user name is missing, fetch from API
        if (!name && !email && userId) {
          api.get('/users/me').then(resp => {
            if (resp.code === 20000 && resp.data?.name) {
              setUserInfo(prev => ({ ...prev, name: resp.data.name }))
            }
          }).catch(() => {})
        }
        console.log('%c[APP DEBUG] setUserInfo called', 'color: green;', { name: userName, email, role: businessRole, sysPerm, cusPerm })
      } catch (e) {
        console.error('%c[APP DEBUG] JWT parse FAILED', 'color: red;', e.message, e.stack)
      }
    } else if (token) {
      console.log('%c[APP DEBUG] token is opaque (no dots), trying localStorage fallback', 'color: orange;')
      const info = localStorage.getItem('user_info')
      console.log('%c[APP DEBUG] localStorage fallback', 'color: orange;', { hasUserInfo: !!info, infoLen: info?.length })
      if (info) {
        try {
          const parsed = JSON.parse(info)
          console.log('%c[APP DEBUG] setUserInfo from fallback', 'color: green;', parsed)
          setUserInfo(parsed)
        } catch (e) {
          console.error('%c[APP DEBUG] localStorage parse FAILED', 'color: red;', e.message)
        }
      }
    } else {
      console.log('%c[APP DEBUG] no token at all', 'color: red;')
    }
  }, [])

  // Load permission mapping and trigger re-render when ready
  const [permMappingReady, setPermMappingReady] = useState(false)
  useEffect(() => {
    console.log('%c[APP DEBUG] initPermissionMapping starting...', 'color: blue;')
    initPermissionMapping().then(() => {
      const mapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')
      console.log('%c[APP DEBUG] permMappingReady=true', 'color: green;', {
        keys: Object.keys(mapping).length,
        mapping: mapping,
        timestamp: new Date().toISOString()
      })
      setPermMappingReady(true)
    }).catch(() => setPermMappingReady(true))
  }, [])

  // Check force_password_change after userInfo is loaded
  useEffect(() => {
    if (!userInfo || location.pathname === '/user/change-password') return
    api.get('/users/me').then(resp => {
      if (resp.code === 20000 && resp.data?.force_password_change) {
        navigate('/user/change-password?first_login=1')
      }
    }).catch(() => {})
  }, [userInfo])
  
  const menuConfig = [
  {
    key: 'instruments',
    icon: <SettingOutlined />,
    label: '乐器管理',
    children: [
      { key: '/instruments/list', label: '乐器列表', permission: { cusPermCodes: ['instrument:create', 'instrument:read', 'instrument:update', 'instrument:delete'] } },
      { key: '/instruments/categories', label: '分类设置', permission: { cusPermCodes: ['category:manage'] } },
      { key: '/instruments/properties', label: '属性管理', permission: { cusPermCodes: ['attribute:manage'] } }
    ]
  },
  {
    key: 'maintenance',
    icon: <ToolOutlined />,
    label: '维修管理',
    children: [
      { key: '/maintenance/sessions', label: '会话管理', permission: { cusPermCodes: ['instrument:read', 'instrument:maintain'] } }
    ]
  },
  {
    key: 'inventory',
    icon: <AppstoreOutlined />,
    label: '库存监控',
    children: [
      { key: '/inventory/rent-setting', label: '租金设定', permission: { cusPermCodes: ['instrument:price'] } },
      { key: '/pricing/config', label: '定价策略', permission: { cusPermCodes: ['instrument:price_config'] } },
      { key: '/warehouse', label: '库管工作台', permission: { cusPermCodes: ['instrument:read', 'instrument:update'] } }
    ]
  },
  {
    key: 'organization',
    icon: <TeamOutlined />,
    label: '组织管理',
    children: [
      { key: '/organization/sites', label: '网点管理', permission: { sysPermBits: [10], cusPermCodes: ['instrument:create', 'instrument:read'], requireAll: true } },
      { key: '/staff', label: '人员管理', permission: { sysPermBits: [15], cusPermCodes: ['instrument:create', 'instrument:read'], requireAll: true } },
      { key: '/appeals', label: '申诉处理', permission: { cusPermCodes: ['appeal:read'] } },
    ]
  },
  {
    key: 'system',
    icon: <SettingOutlined />,
    label: '系统管理',
    children: [
      { key: '/merchants', label: '商户管理', permission: { sysPermBits: [5] } },
      { key: '/system/audit-logs', label: '操作日志', permission: { cusPermCodes: ['audit_log:read'] } },
      { key: '/system/permissions', label: '权限管理', permission: { sysPermBits: [27] } },
      { key: '/system/banners', label: '轮播图管理', permission: { cusPermCodes: ['banner:manage'] } }
    ]
  },
  { key: '/user/profile', icon: <UserOutlined />, label: '个人中心' }
]


function onMenuClick(e) {
  navigate(e.key)
}

  const sysPerm = userInfo?.sysPerm || 0
  const cusPerm = userInfo?.cusPerm || 0

  // Permission mapping from localStorage (loaded by api.js)
  const cusPermMapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')

  // Filter menu: children by permission, parent visible if any child visible
  const ownerAtMerchant = userInfo?.tid && userInfo?.tid === userInfo?.oid
  const isNsAdmin = isNamespaceAdmin(userInfo?.roles || [])
  const filteredItems = menuConfig
    .map(item => ({
      ...item,
      children: item.children?.filter(child => {
        const childKey = child.key || ''
        if (isNsAdmin && getNamespaceAdminMenuKeys().includes(childKey)) return true
        if (ownerAtMerchant && cusPerm === 0 && sysPerm === 0) {
          if (item.key !== 'system') return true
        }
        return checkPermission(child.permission, sysPerm, cusPerm, cusPermMapping)
      }).filter(child => !isNsAdmin || getNamespaceAdminMenuKeys().includes(child.key))
    }))
    .filter(item => (item.children && item.children.length > 0) || item.key.startsWith('/'));

  console.log('%c[APP DEBUG] Menu filter result', 'color: purple;', {
    userInfoExists: !!userInfo,
    sysPerm, cusPerm,
    isNsAdmin, ownerAtMerchant,
    permMappingKeys: Object.keys(cusPermMapping).length,
    visibleItems: filteredItems.map(i => i.key),
    totalItems: menuConfig.length,
    timestamp: new Date().toISOString()
  })

  const selectedKeys = [location.pathname]
  let openKeys = []
  if (['/', '/instruments/categories', '/instruments/list', '/instruments/properties'].includes(location.pathname) || location.pathname.startsWith('/instruments/')) openKeys = ['instruments']
  else if (['/site/stock', '/instruments/detail'].includes(location.pathname) || location.pathname.startsWith('/site/stock/')) openKeys = ['instruments']
  else if (['/inventory/transfer', '/inventory/rent-setting', '/pricing/config'].includes(location.pathname) || location.pathname.startsWith('/inventory/')) openKeys = ['inventory']
  else if (['/organization/sites', '/staff', '/appeals'].includes(location.pathname)) openKeys = ['organization']
  else if (['/merchants', '/system/audit-logs'].includes(location.pathname)) openKeys = ['system']
  else if (location.pathname.startsWith('/user/')) openKeys = []


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
    '/pricing/config': { title: '定价策略', parent: '库存监控' },
    '/organization/sites': { title: '网点管理', parent: '组织管理' },
    '/organization/sites/new': { title: '新建网点', parent: '网点管理' },
    '/merchants': { title: '商户管理', parent: '系统管理' },
    '/system/audit-logs': { title: '操作日志', parent: '系统管理' },
    '/system/banners': { title: '轮播图管理', parent: '系统管理' },
    '/staff': { title: '人员管理', parent: '组织管理' },
    '/appeals': { title: '申诉处理', parent: '组织管理' },
    '/user/profile': { title: '个人中心' },

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
  } else if (location.pathname.startsWith('/instruments/detail/')) {
    pageTitle = '乐器详情'
    breadcrumbItems.push(
      { title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/'); }}>乐器管理</a> },
      { title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/instruments/list'); }}>乐器列表</a> },
      { title: '乐器详情' }
    )
  } else if (location.pathname.match(/^\/instruments\/[^/]+\/edit$/)) {
    pageTitle = '编辑乐器'
    breadcrumbItems.push(
      { title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/'); }}>乐器管理</a> },
      { title: <a href="#" onClick={(e) => { e.preventDefault(); navigate('/instruments/list'); }}>乐器列表</a> },
      { title: '编辑乐器' }
    )
  } else if (location.pathname.match(/^\/instruments\/[^/]+$/) && location.pathname !== '/instruments/list') {
    pageTitle = '乐器详情'
    breadcrumbItems.push({ title: '乐器详情' })
  }

  const tokenBeforeRedirect = getToken()
  console.log('%c[APP DEBUG] render-level token check', 'color: teal;', {
    hasToken: !!tokenBeforeRedirect,
    tokenType: tokenBeforeRedirect ? (tokenBeforeRedirect.includes('.') ? 'JWT' : 'opaque') : 'none',
    pathname: location.pathname,
    localStorageToken: !!localStorage.getItem('token'),
    sessionStorageToken: !!sessionStorage.getItem('token'),
    cookieToken: !!(document.cookie && document.cookie.includes('token')),
    timestamp: new Date().toISOString()
  })

  if (!getToken() && location.pathname !== '/callback' && location.pathname !== '/logout') {
    console.log('%c[APP DEBUG] redirecting to IAM (no token)', 'color: red;')
    redirectToIAMLogin()
    return null
  }

  if (isFirstLogin) {
    return <ChangePassword />
  }

  if (location.pathname !== '/callback' && (userInfo === null || !permMappingReady)) {
    console.log('%c[APP DEBUG] Waiting for init', 'color: gray;', {
      userInfoExists: !!userInfo, permMappingReady,
      timestamp: new Date().toISOString()
    })
    return <Spin fullscreen tip="正在初始化..." />
  }

  return (
    <>
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
                    {userInfo.businessRole === 'system_admin' ? '命名空间管理员' :
                     userInfo.businessRole === 'merchant_admin' ? '管理员' :
                     userInfo.businessRole === 'site_admin' ? '管理员' :
                     userInfo.businessRole === 'worker' ? '维修工程师' : '员工'}
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
            <Route path="/setup" element={<Setup />} />
            <Route path="/" element={<ProtectedRoute><Dashboard /></ProtectedRoute>} />
            <Route path="/assets" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">资产管理</div></ProtectedRoute>} />
            <Route path="/lease/ledger" element={<ProtectedRoute><LeaseLedger /></ProtectedRoute>} />
            <Route path="/finance" element={<ProtectedRoute><FinanceConfig /></ProtectedRoute>} />
            <Route path="/finance/quotes" element={<ProtectedRoute><div className="bg-white p-6 rounded shadow">报价单管理</div></ProtectedRoute>} />
            <Route path="/site/stock" element={<ProtectedRoute><InstrumentStock /></ProtectedRoute>} />
            <Route path="/site/stock/:id" element={<ProtectedRoute><AssetDetail /></ProtectedRoute>} />
            <Route path="/organization/sites" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10], cusPermCodes: ['instrument:create', 'instrument:read'], requireAllGroups: true }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/organization/sites/new" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10] }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/organization/sites/:id/edit" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10] }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/organization/sites/:id/new" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10] }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/organization/sites/:id" element={<ProtectedRoute requiredPermission={{ sysPermBits: [10] }}><SiteManagement /></ProtectedRoute>} />
            <Route path="/merchants" element={<ProtectedRoute requiredPermission={{ sysPermBits: [5] }}><MerchantManagement /></ProtectedRoute>} />
            <Route path="/system/audit-logs" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['audit_log:read'] }}><AuditLogPage /></ProtectedRoute>} />
            <Route path="/staff" element={<ProtectedRoute requiredPermission={{ sysPermBits: [15], cusPermCodes: ['instrument:create', 'instrument:read'], requireAllGroups: true }}><StaffManagement /></ProtectedRoute>} />
            <Route path="/staff/:id/edit" element={<ProtectedRoute requiredPermission={{ sysPermBits: [15] }}><StaffEdit /></ProtectedRoute>} />
            <Route path="/staff/:id/reset-password" element={<ProtectedRoute requiredPermission={{ sysPermBits: [15] }}><StaffResetPassword /></ProtectedRoute>} />
            <Route path="/appeals" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['appeal:read'] }}><AppealManagement /></ProtectedRoute>} />
            <Route path="/workorders" element={<ProtectedRoute><WorkOrderList /></ProtectedRoute>} />
            <Route path="/maintenance/sessions" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:read', 'instrument:maintain'] }}><MaintenanceSessionManagement /></ProtectedRoute>} />
            <Route path="/maintenance/suppliers" element={<ProtectedRoute><SupplierDB /></ProtectedRoute>} />
            <Route path="/system/permissions" element={<ProtectedRoute requiredPermission={{ sysPermBits: [27] }}><PermissionManage /></ProtectedRoute>} />
            <Route path="/system/clients" element={<ProtectedRoute requiredPermission={{ sysPermBits: [0] }}><ClientManagement /></ProtectedRoute>} />
            <Route path="/system/tenants" element={<ProtectedRoute requiredPermission={{ sysPermBits: [6] }}><TenantManagement /></ProtectedRoute>} />
            <Route path="/system/banners" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['banner:manage'] }}><BannerManagePage /></ProtectedRoute>} />
            <Route path="/inventory/rent-setting" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:price'] }}><RentSetting /></ProtectedRoute>} />
            <Route path="/pricing/config" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:price_config'] }}><MerchantPricingConfig /></ProtectedRoute>} />
            <Route path="/warehouse" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:read', 'instrument:update'] }}><WarehouseManagement /></ProtectedRoute>} />
            <Route path="/user/rentals" element={<ProtectedRoute><UserRental /></ProtectedRoute>} />
            <Route path="/user/profile" element={<ProtectedRoute><UserProfile /></ProtectedRoute>} />
            <Route path="/user/change-password" element={<ProtectedRoute><ChangePassword /></ProtectedRoute>} />
            <Route path="/instruments" element={<ProtectedRoute><InstrumentListUser /></ProtectedRoute>} />
            <Route path="/instruments/:id" element={<ProtectedRoute><InstrumentDetailUser /></ProtectedRoute>} />
            <Route path="/orders/:id/payment" element={<ProtectedRoute><OrderPayment /></ProtectedRoute>} />
            <Route path="/user/contracts/:id" element={<ProtectedRoute><ContractView /></ProtectedRoute>} />
            <Route path="/user/rentals/:id/return" element={<ProtectedRoute><ReturnProcess /></ProtectedRoute>} />
          
            <Route path="/instruments/categories" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['category:manage'] }}><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/:id/edit" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/categories/new" element={<ProtectedRoute><CategoryList /></ProtectedRoute>} />
            <Route path="/instruments/list" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['instrument:create', 'instrument:read', 'instrument:update', 'instrument:delete'] }}><InstrumentList /></ProtectedRoute>} />
             <Route path="/instruments/list/add" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
             <Route path="/instruments/list/edit/:id" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
              <Route path="/instruments/detail/:id" element={<ProtectedRoute><InstrumentDetail /></ProtectedRoute>} />
               <Route path="/instruments/batch-import" element={<ProtectedRoute><BatchImport /></ProtectedRoute>} />
                <Route path="/instruments/:id/edit" element={<ProtectedRoute><InstrumentForm /></ProtectedRoute>} />
                <Route path="/instruments/properties" element={<ProtectedRoute requiredPermission={{ cusPermCodes: ['attribute:manage'] }}><PropertyList /></ProtectedRoute>} />
                <Route path="/organization/sites/bulk-import" element={<ProtectedRoute requiredPermission={{ sysPermBits: [12] }}><SiteBulkImport /></ProtectedRoute>} />
                <Route path="/staff/bulk-import" element={<ProtectedRoute requiredPermission={{ sysPermBits: [17] }}><StaffBulkImport /></ProtectedRoute>} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
    {showExpiryWarning && (
      <div
        onClick={handleExtendSession}
        style={{
          position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
          background: 'rgba(0,0,0,0.6)', zIndex: 9999,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          cursor: 'pointer',
        }}
      >
        <div style={{
          background: '#fff', padding: '32px 48px', borderRadius: 12,
          textAlign: 'center', boxShadow: '0 4px 20px rgba(0,0,0,0.3)',
        }}>
          <h2 style={{ marginBottom: 12, fontSize: 20 }}>会话即将结束</h2>
          <p style={{ color: '#888', marginBottom: 8 }}>点击任意位置即可继续操作</p>
        </div>
      </div>
    )}
  </>
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
      iamClientId: null,
      iamRedirectUri: import.meta.env.VITE_IAM_PC_REDIRECT_URI || ''
    }
    const redirectUri = encodeURIComponent(config.iamRedirectUri)
    return `${config.iamExternalUrl}/oauth/authorize?prompt=login&client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code&noRegister=1`
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
        const cookieDomains = ['', '.cadenzayueqi.com', '.linxdeep.com']
        cookieDomains.forEach(domain => {
          const path = domain ? `; domain=${domain}` : ''
          document.cookie = `token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
          document.cookie = `refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
        })
        localStorage.setItem('logout_reason', 'auth_failed')
        window.location.href = '/logout'
      }, 3000)
      return
    }

    if (!code) {
      setErrorMsg('缺少授权码')
      setLoading(false)
      setTimeout(() => {
        const cookieDomains = ['', '.cadenzayueqi.com', '.linxdeep.com']
        cookieDomains.forEach(domain => {
          const path = domain ? `; domain=${domain}` : ''
          document.cookie = `token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
          document.cookie = `refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
        })
        localStorage.setItem('logout_reason', 'auth_failed')
        window.location.href = '/logout'
      }, 3000)
      return
    }

    const existingToken = getToken()
    if (existingToken) {
      console.log('%c[CALLBACK DEBUG] Token already exists, skipping exchange', 'color: teal;', { tokenType: existingToken.includes('.') ? 'JWT' : 'opaque' })
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
        console.log('%c[CALLBACK DEBUG] exchange response', 'color: teal;', { status: response.status, code: responseData.code, hasAccessToken: !!responseData.data?.access_token, hasData: !!responseData.data })

        const tokenData = responseData.data || responseData

        if (tokenData.relogin) {
          console.log('[OAuth] Code already used, redirecting to IAM for new code')
          const cookieDomains = ['', '.cadenzayueqi.com', '.linxdeep.com']
          cookieDomains.forEach(domain => {
            const path = domain ? `; domain=${domain}` : ''
            document.cookie = `token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
            document.cookie = `refresh_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/${path}`
          })
          localStorage.setItem('logout_reason', 'auth_failed')
          window.location.href = '/logout'
          return
        }
        
        if (tokenData.access_token) {
          console.log('%c[CALLBACK DEBUG] got access_token, storing...', 'color: green;', { tokenLen: tokenData.access_token.length, isJWT: tokenData.access_token.includes('.'), hasRefresh: !!tokenData.refresh_token })

          const expiresIn = Math.max(tokenData.expires_in || 3600, 60)
          storeToken(tokenData.access_token, expiresIn)

          console.log('%c[CALLBACK DEBUG] after storeToken', 'color: green;', { localStorageHasToken: !!localStorage.getItem('token'), localStorageHasExpiry: !!localStorage.getItem('token_expiry') })
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
          console.log('%c[CALLBACK DEBUG] redirecting to', 'color: green;', { redirectTo, localStorageHasToken: !!localStorage.getItem('token') })
          sessionStorage.removeItem('post_auth_redirect')
          window.location.href = redirectTo
        } else {
          console.error('%c[CALLBACK DEBUG] no access_token in response', 'color: red;', responseData)
          throw new Error('No access token received')
        }
      } catch (error) {
        console.error('%c[CALLBACK DEBUG] exchange FAILED', 'color: red;', { message: error.message, stack: error.stack })
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
  const [configReady, setConfigReady] = useState(false)
  useEffect(() => {
    api.get('/config')
      .then(data => {
        if (data) {
          window.APP_CONFIG = data?.data || data
        }
      })
      .catch(err => console.error('Failed to load config:', err))
      .finally(() => setConfigReady(true))
  }, [])
  
  if (!configReady) return <Spin fullscreen tip="正在加载配置..." />
  
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/logout" element={<LogoutPage />} />
        <Route path="/callback" element={<OAuthCallback />} />
        <Route path="*" element={<MainLayout />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
