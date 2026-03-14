import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from 'antd'
import Dashboard from './pages/Dashboard'
import FinanceConfig from './pages/FinanceConfig'
import WorkOrderList from './pages/WorkOrderList'

const { Header, Content, Sider } = Layout

function App() {
  return (
    <BrowserRouter>
      <Layout style={{ minHeight: '100vh' }}>
        <Sider width={200} theme="dark">
          <div className="h-16 flex items-center justify-center text-white font-bold">
            TuneLoop 管理后台
          </div>
        </Sider>
        <Layout>
          <Header className="bg-white px-6 shadow">
            <h1 className="text-xl font-bold">资产管理</h1>
          </Header>
          <Content className="p-6 bg-gray-100">
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/finance" element={<FinanceConfig />} />
              <Route path="/workorders" element={<WorkOrderList />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </BrowserRouter>
  )
}

export default App
