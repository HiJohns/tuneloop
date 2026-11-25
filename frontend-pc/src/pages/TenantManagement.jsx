import { useState, useEffect } from 'react'
import { Table, message, Tag, Alert } from 'antd'
import { api } from '../services/api'
import { formatBeijingDate } from '../utils/date'
import { isApiOk, apiFailureMessage } from '../utils/apiResult'

// #2516：平台端「租户管理」为**只读列表**。
// 租户行是 IAM 组织镜像（tenants.id = IAM 组织 ID），由「商户管理」创建商户
// （POST /api/merchants）时自动生成；后端从未注册 /system/tenants 写端点，
// 本页原有「创建租户」入口只会得到 404 并提示「租户创建成功」（假成功），已移除。
const STATUS_LABELS = {
  active: { text: '启用', color: 'green' },
  inactive: { text: '停用', color: 'red' },
}

export default function TenantManagement() {
  const [tenants, setTenants] = useState([])
  const [loading, setLoading] = useState(false)
  // #2160：服务端分页
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)

  useEffect(() => {
    fetchTenants()
  }, [page, pageSize])

  const fetchTenants = async () => {
    try {
      setLoading(true)
      const res = await api.get('/system/tenants', { params: { page, page_size: pageSize } })
      // #2516：request() 对 404/409 返回对象而非抛错，必须终判 code；
      // 失败时清空列表，禁止沿用上一次数据造成「静默空表/假成功」。
      if (!isApiOk(res)) {
        message.error(apiFailureMessage(res, '加载租户列表失败'))
        setTenants([])
        setTotal(0)
        return
      }
      setTenants(res.data?.list || [])
      setTotal(res.data?.total || 0)
    } catch (error) {
      console.error('Failed to fetch tenants:', error)
      message.error('加载租户列表失败')
      setTenants([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }

  const columns = [
    {
      title: '租户名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '租户ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const label = STATUS_LABELS[status] || { text: '未知', color: 'default' }
        return <Tag color={label.color}>{label.text}</Tag>
      },
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text) => formatBeijingDate(text),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">租户管理</h1>
      </div>

      <Alert
        type="info"
        showIcon
        className="mb-4"
        message="租户随商户创建自动生成；新建租户请在『商户管理』完成"
      />

      <Table
        columns={columns}
        dataSource={tenants || []}
        rowKey="id"
        loading={loading}
        pagination={{
          current: page, pageSize, total, showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
      />
    </div>
  )
}
