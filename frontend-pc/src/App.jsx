import { useState } from 'react'
import { BrowserRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu } from 'antd'
import {
  DashboardOutlined,
  BookOutlined,
  ToolOutlined,
  BankOutlined,
  SettingOutlined
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

function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()

  const items = [
    { key: '/', icon: <DashboardOutlined />, label: '仪表盘' },
    { 
      key: 'business', icon: <BookOutlined />, label: '业务管理',
      children: [
        { key: '/lease/ledger', label: '租约台账' },
        { key: '/lease/deposit', label: '押金流水' },
        { key: '/lease/warning', label: '到期预警' }
      ] 
    },
    { 
      key: 'assets', icon: <BankOutlined />, label: '资产载体',
      children: [
        { key: '/site/stock', label: '乐器库存' },
        { key: '/site/management', label: 'Site网点管理(LBS)' }
      ] 
    },
    { 
      key: 'system', icon: <SettingOutlined />, label: '系统配置',
      children: [
        { key: '/finance', label: '报价单配置' },
        { key: '/settings/roles', label: '角色权限' },
        { key: '/workorders', label: '工单列表' },
        { key: '/maintenance/suppliers', label: '维保师傅管理' }
      ] 
    }
  ]

  const onMenuClick = (e) => {
    navigate(e.key)
  }

  // 计算当前应该选中的菜单项
  const selectedKeys = [location.pathname]
  
  // 找出打开的子菜单（简单的根据路径前缀判断）
  const openKeys = []
  if (location.pathname === '/') openKeys.push('/')
  else if (location.pathname.startsWith('/lease')) openKeys.push('business')
  else if (location.pathname.startsWith('/site')) openKeys.push('assets')
  else if (location.pathname.startsWith('/settings') || location.pathname === '/finance') openKeys.push('system')
  else if (location.pathname.startsWith('/maintenance') || location.pathname === '/workorders') openKeys.push('system')

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider 
        width={200} 
        theme="dark"
        style={{ backgroundColor: '#002140' }}
        collapsible 
        collapsed={collapsed} 
        onCollapse={(value) => setCollapsed(value)}
      >
        <div className="h-16 flex items-center justify-center text-white font-bold text-lg overflow-hidden whitespace-nowrap">
          {collapsed ? 'TL' : 'TuneLoop 管理后台'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selectedKeys}
          defaultOpenKeys={openKeys}
          items={items}
          onClick={onMenuClick}
        />
      </Sider>
      <Layout>
        <Header className="bg-white px-6 shadow flex items-center">
          <div style={{ width: 4, height: 24, backgroundColor: '#002140', marginRight: 12, borderRadius: 2 }} /><h1 className="text-xl font-bold m-0" style={{ fontSize: 20 }}>资产管理</h1>
        </Header>
        <Content className="p-6 bg-gray-100 overflow-y-auto">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            
            <Route path="/lease/ledger" element={<LeaseLedger />} />
            <Route path="/lease/deposit" element={<DepositFlow />} />
            <Route path="/lease/warning" element={<ExpireWarning />} />
            
            <Route path="/workorders" element={<WorkOrderList />} />
            <Route path="/maintenance/suppliers" element={<SupplierDB />} />
            
            <Route path="/site/stock" element={<InstrumentStock />} />
            <Route path="/site/management" element={<SiteManagement />} />
            
            <Route path="/finance" element={<FinanceConfig />} />
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
