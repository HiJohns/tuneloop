import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Spin, Empty } from 'antd'
import { api } from '../services/api'

export default function SiteManagement() {
  const [modalOpen, setModalOpen] = useState(false)
  const [selectedSite, setSelectedSite] = useState(null)
  const [sites, setSites] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    fetchSites()
  }, [])

  const fetchSites = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await api.get('/admin/sites')
      setSites(response || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const columns = [
    {
      title: '网点ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '网点名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '地址',
      dataIndex: 'address',
      key: 'address',
    },
    {
      title: '负责人',
      dataIndex: 'manager',
      key: 'manager',
    },
    {
      title: '联系电话',
      dataIndex: 'phone',
      key: 'phone',
    },
    {
      title: '乐器数量',
      dataIndex: 'instruments',
      key: 'instruments',
      align: 'right',
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Button 
          type="link" 
          onClick={() => {
            setSelectedSite(record)
            setModalOpen(true)
          }}
        >
          查看地图
        </Button>
      )
    }
  ]

  if (loading) {
    return (
      <div className="text-center py-16">
        <Spin size="large" />
        <div className="mt-4 text-gray-500">数据正在同步中...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-16">
        <Empty description="数据加载失败" />
        <Button type="primary" onClick={fetchSites} className="mt-4">
          重试
        </Button>
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Site网点管理</h2>
      
      <Card title="网点列表" className="mb-4">
        <Table 
          columns={columns} 
          dataSource={sites} 
          rowKey="id"
          pagination={{ total: sites.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
          locale={{ emptyText: '暂无网点数据' }}
        />
      </Card>

      <Modal
        title={`${selectedSite?.name} - 位置地图`}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        footer={[
          <Button key="close" onClick={() => setModalOpen(false)}>
            关闭
          </Button>
        ]}
        width={800}
      >
        {selectedSite && (
          <div className="text-center">
            <div className="mb-4">
              <p>地址: {selectedSite.address}</p>
              <p>坐标: {selectedSite.lat.toFixed(4)}, {selectedSite.lng.toFixed(4)}</p>
            </div>
            <div className="bg-gray-100 p-4 rounded">
              <img 
                src={`https://picsum.photos/seed/site-${selectedSite.id}/720/400`}
                alt={`${selectedSite.name} 地图`}
                className="w-full rounded shadow"
              />
              <p className="text-gray-500 text-sm mt-2">
                静态地图占位图（实际应用可接入百度/高德地图API）
              </p>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}