import { Table, Tag, Space } from 'antd'
import { assets } from '../data/mockData'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange",
  "待清理": "red"
}

const levelColors = {
  "入门级": "default",
  "专业级": "blue",
  "大师级": "gold"
}

export default function Dashboard() {
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
      render: (level) => (
        <Tag color={levelColors[level]}>{level}</Tag>
      )
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={statusColors[status]}>{status}</Tag>
      )
    },
    {
      title: '所属网点',
      dataIndex: 'site',
      key: 'site',
    },
  ]

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">资产看板</h2>
      <Table 
        columns={columns} 
        dataSource={assets} 
        rowKey="id"
        pagination={false}
      />
    </div>
  )
}
