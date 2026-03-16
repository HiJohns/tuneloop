import { useState } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Breadcrumb } from 'antd'
import {
  AppstoreOutlined,
  SettingOutlined,
  DatabaseOutlined
} from '@ant-design/icons'

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

const { Header, Content, Sider } = Layout

const BRAND_COLOR = '#002140'

function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()

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

  // 计算当前应该选中的菜单项
  const selectedKeys = [location.pathname]
  
  // 找出打开的子菜单
  let openKeys = []
  if (['/', '/assets', '/lease/ledger'].includes(location.pathname)) openKeys = ['core']
  else if (['/finance', '/finance/quotes'].includes(location.pathname)) openKeys = ['config']
  else if (['/site/stock', '/site/management'].includes(location.pathname)) openKeys = ['data']
  else if (location.pathname.startsWith('/site/stock/')) openKeys = ['data']

  // 获取面包屑和标题
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
        <Header className="bg-white px-6 shadow flex flex-col justify-center h-24 py-4 leading-normal">
          <Breadcrumb items={breadcrumbItems} className="mb-2 text-sm text-gray-500" />
          <div className="flex items-center">
            <div style={{ width: 4, height: 24, backgroundColor: BRAND_COLOR, marginRight: 12, borderRadius: 2 }} />
            <h1 className="text-2xl font-bold m-0" style={{ color: BRAND_COLOR }}>{pageTitle}</h1>
          </div>
        </Header>
        <Content className="p-6 bg-gray-100 overflow-y-auto">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/assets" element={<div className="bg-white p-6 rounded shadow">资产管理待实现</div>} />
            <Route path="/lease/ledger" element={<LeaseLedger />} />
            <Route path="/finance" element={<FinanceConfig />} />
            <Route path="/finance/quotes" element={<div className="bg-white p-6 rounded shadow">报价单管理待实现</div>} />
            <Route path="/site/stock" element={<InstrumentStock />} />
            <Route path="/site/management" element={<SiteManagement />} />
            
            {/* 保留其他路由以防报错 */}
            <Route path="/lease/deposit" element={<DepositFlow />} />
            <Route path="/lease/warning" element={<ExpireWarning />} />
            <Route path="/workorders" element={<WorkOrderList />} />
            <Route path="/maintenance/suppliers" element={<SupplierDB />} />
            <Route path="/settings/roles" element={<RolePermission />} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  )
}

function App() {
  return (
    <BrowserRouter>
      <MainLayout />
    </BrowserRouter>
  )
}

export default App
