import { useState } from 'react'
import { Card, Table, Button, Modal } from 'antd'

const sites = [
  {
    id: 'Site-001',
    name: '北京总店',
    address: '北京市朝阳区建国路88号',
    lat: 39.9042,
    lng: 116.4074,
    manager: '张经理',
    phone: '138****8888',
    instruments: 156
  },
  {
    id: 'Site-002',
    name: '上海分店',
    address: '上海市浦东新区陆家嘴东路100号',
    lat: 31.2304,
    lng: 121.4737,
    manager: '李经理',
    phone: '139****9999',
    instruments: 89
  },
  {
    id: 'Site-003',
    name: '维修供应商',
    address: '天津市滨海新区经济技术开发区',
    lat: 39.0851,
    lng: 117.7445,
    manager: '王师傅',
    phone: '136****6666',
    instruments: 23
  }
]

export default function SiteManagement() {
  const [modalOpen, setModalOpen] = useState(false)
  const [selectedSite, setSelectedSite] = useState(null)

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

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Site网点管理</h2>
      
      <Card title="网点列表" className="mb-4">
        <Table 
          columns={columns} 
          dataSource={sites} 
          rowKey="id"
          pagination={{ total: sites.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
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