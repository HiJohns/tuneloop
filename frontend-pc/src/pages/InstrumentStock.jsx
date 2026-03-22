import { useState, useMemo, useEffect } from 'react'
import { Table, Tag, Button, Space, Spin } from 'antd'
import { EyeOutlined, EditOutlined } from '@ant-design/icons'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { inventoryApi } from '../services/api'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange",
  "已熔断": "red",
  "rented": "green",
  "available": "blue",
  "maintenance": "orange",
}

export default function InstrumentStock() {
  const [searchParams] = useSearchParams()
  const statusParam = searchParams.get('status')
  const overdueParam = searchParams.get('overdue')
  const navigate = useNavigate()
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    setLoading(true)
    try {
      const data = await inventoryApi.list()
      setAssets(data || [])
    } catch (error) {
      console.error('Failed to load inventory:', error)
    } finally {
      setLoading(false)
    }
  }

  const filteredAssets = useMemo(() => {
    let result = assets
    
    if (statusParam) {
      result = result.filter(a => a.status === statusParam)
    }
    
    if (overdueParam === 'true') {
      const today = new Date().toISOString().split('T')[0]
      result = result.filter(a => a.leaseEnd && a.leaseEnd < today && (a.status === '在租' || a.status === 'rented'))
    }
    
    return result
  }, [assets, statusParam, overdueParam])
  
  const columns = [
    {
      title: '资产ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '乐器名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '类别',
      dataIndex: 'category',
      key: 'category',
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => {
        const getUnifiedStatus = (record) => {
          const statusMap = {
            "在租": { color: 'green', text: '在租' },
            "rented": { color: 'green', text: '在租' },
            "待租": { color: 'blue', text: '待租' },
            "available": { color: 'blue', text: '待租' },
            "维修中": { color: 'orange', text: '维修中' },
            "maintenance": { color: 'orange', text: '维修中' },
            "已熔断": { color: 'red', text: '已熔断' }
          }
          return statusMap[status] || { color: 'default', text: status }
        }
        
        const info = getUnifiedStatus(record)
        return <Tag color={info.color}>{info.text}</Tag>
      }
    },
    {
      title: '所属网点',
      dataIndex: 'site',
      key: 'site',
    },
    {
      title: '估值',
      dataIndex: 'value',
      key: 'value',
      align: 'right',
      render: (value) => `¥${(value || 0).toLocaleString()}`
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" icon={<EyeOutlined />}>详情</Button>
          <Button type="link" size="small" icon={<EditOutlined />}>编辑</Button>
        </Space>
      )
    }
  ]

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">乐器库存</h2>
      {statusParam && (
        <div className="mb-4 p-3 bg-blue-50 rounded">
          <Space>
            <span>当前筛选: 状态 = {statusParam}</span>
            <Button type="link" size="small" onClick={() => window.location.href = '/site/stock'}>清除筛选</Button>
          </Space>
        </div>
      )}
      {overdueParam === 'true' && (
        <div className="mb-4 p-3 bg-orange-50 rounded">
          <Space>
            <span>当前筛选: 逾期未归还</span>
            <Button type="link" size="small" onClick={() => window.location.href = '/site/stock'}>清除筛选</Button>
          </Space>
        </div>
      )}
       <Table 
         columns={columns} 
         dataSource={filteredAssets} 
         rowKey="id"
         pagination={{ total: filteredAssets.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
         onRow={(record) => ({
           onClick: () => navigate(`/site/stock/${record.id}`),
           style: { cursor: 'pointer' }
         })}
       />
    </div>
  )
}
