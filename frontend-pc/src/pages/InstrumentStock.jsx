import { Table, Tag } from 'antd'
import { assets } from '../data/mockData'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange"
}

const ownershipColors = {
  "租赁中": "blue",
  "已转售": "green"
}

export default function InstrumentStock() {
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
      render: (status) => (
        <Tag color={statusColors[status] || 'default'}>{status}</Tag>
      )
    },
    {
      title: '所有权状态',
      dataIndex: 'ownershipStatus',
      key: 'ownershipStatus',
      render: (status) => (
        <Tag color={ownershipColors[status] || 'default'}>{status}</Tag>
      )
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
      render: (value) => `¥${value.toLocaleString()}`
    }
  ]

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">乐器库存</h2>
      <Table 
        columns={columns} 
        dataSource={assets} 
        rowKey="id"
        pagination={{ total: assets.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
      />
    </div>
  )
}