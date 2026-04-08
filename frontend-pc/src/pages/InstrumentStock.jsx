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
      title: '图片',
      dataIndex: 'images',
      key: 'images',
      width: 80,
      render: (images) => {
        const imageList = Array.isArray(images) ? images : (images ? JSON.parse(images) : [])
        const src = imageList && imageList.length > 0 ? imageList[0] : '/images/default-instrument.jpg'
        return <img src={src} alt="" style={{ width: 50, height: 50, objectFit: 'cover', borderRadius: 4 }} />
      }
    },
    {
      title: '资产ID',
      dataIndex: 'id',
      key: 'id',
      width: 180,
      render: (id) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{id?.slice(0, 8)}...</span>
    },
    {
      title: '乐器名称',
      dataIndex: 'name',
      key: 'name',
      width: 180,
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
      width: 120,
    },
    {
      title: '级别',
      dataIndex: 'level_name',
      key: 'level_name',
      width: 80,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status, record) => {
        const getUnifiedStatus = (status) => {
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
        
        const info = getUnifiedStatus(status)
        return <Tag color={info.color}>{info.text}</Tag>
      }
    },
    {
      title: '所属网点',
      dataIndex: 'site',
      key: 'site',
      width: 150,
    },
    {
      title: '估值',
      dataIndex: 'value',
      key: 'value',
      width: 120,
      align: 'right',
      render: (value) => value ? `¥${value.toLocaleString()}` : '-'
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
    <div className="p-6">
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
        dataSource={filteredAssets || []} 
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
