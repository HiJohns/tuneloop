import { useState } from 'react'
import { Table, Tag, Alert, Button, Modal, message, Timeline } from 'antd'
import { LockOutlined, HistoryOutlined } from '@ant-design/icons'
import { assets } from '../data/mockData'

export default function WorkOrderList() {
  const [unlockModalOpen, setUnlockModalOpen] = useState(false)
  const [trackModalOpen, setTrackModalOpen] = useState(false)
  const [selectedOrder, setSelectedOrder] = useState(null)

  const workOrders = assets
    .filter(a => a.workOrder)
    .map(a => ({
      key: a.workOrder.id,
      assetId: a.id,
      assetName: a.name,
      workOrderId: a.workOrder.id,
      jumps: a.workOrder.jumps,
      technician: a.workOrder.technician,
      status: a.status
    }))

  const columns = [
    {
      title: '工单ID',
      dataIndex: 'workOrderId',
      key: 'workOrderId',
    },
    {
      title: '资产ID',
      dataIndex: 'assetId',
      key: 'assetId',
    },
    {
      title: '资产名称',
      dataIndex: 'assetName',
      key: 'assetName',
    },
    {
      title: '工单跳数',
      dataIndex: 'jumps',
      key: 'jumps',
      render: (jumps) => <Tag color={jumps >= 3 ? 'red' : 'orange'}>H = {jumps}</Tag>
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const statusMap = {
          "在租": { color: 'green', icon: null, text: '在线' },
          "待租": { color: 'blue', icon: null, text: '在线' },
          "维修中": { color: 'blue', icon: '🔧', text: '维修中' },
          "已熔断": { color: 'red', icon: '⛔', text: '已熔断' }
        }
        const info = statusMap[status] || { color: 'default', icon: null, text: status }
        return (
          <span>
            {info.icon && <span className="mr-1">{info.icon}</span>}
            <Tag color={info.color}>{info.text}</Tag>
          </span>
        )
      }
    },
    {
      title: '服务人员',
      dataIndex: 'technician',
      key: 'technician',
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 120,
      render: (_, record) => (
        record.jumps >= 3 ? (
          <Button 
            type="link" 
            danger 
            icon={<LockOutlined />}
            onClick={() => {
              setSelectedOrder(record)
              setUnlockModalOpen(true)
            }}
          >
            强制解锁
          </Button>
        ) : (
          <Button 
            type="link" 
            icon={<HistoryOutlined />}
            onClick={() => {
              setSelectedOrder(record)
              setTrackModalOpen(true)
            }}
          >
            查看轨迹
          </Button>
        )
      )
    }
  ]

  return (
    <div>
      <h2 style={{ 
        fontSize: 20, 
        fontWeight: 'bold', 
        borderLeft: '4px solid #002140',
        paddingLeft: 12,
        marginBottom: 16
      }}>
        工单列表
      </h2>
      
      {workOrders.some(wo => wo.jumps >= 3) && (
        <Alert
          message="工单锁定提醒"
          description="以下工单已达到最大跳数(H=3)，系统将强制锁定，仅限当前工人执行。"
          type="warning"
          showIcon
          className="mb-4"
        />
      )}
      
      <Table 
        columns={columns} 
        dataSource={workOrders}
        pagination={{
          total: workOrders.length,
          pageSize: 10,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`
        }}
        rowKey="key"
        onRow={(record) => ({
          style: record.jumps >= 3 ? { backgroundColor: '#fff1f0' } : {}
        })}
      />

      <Modal
        title="强制解锁确认"
        open={unlockModalOpen}
        onOk={() => {
          message.success('已打破熔断锁，工单已解锁')
          setUnlockModalOpen(false)
        }}
        onCancel={() => setUnlockModalOpen(false)}
        okText="确认解锁"
        cancelText="取消"
      >
        <p>确定要强制解锁该工单吗？</p>
        <p>此操作将打破熔断机制，允许重新分配。</p>
      </Modal>

      <Modal
        title="工单流转轨迹"
        open={trackModalOpen}
        onCancel={() => setTrackModalOpen(false)}
        footer={null}
        width={600}
      >
        {selectedOrder && (
          <Timeline
            items={[
              { color: 'green', children: '2024-01-15 - 工单创建' },
              { color: 'blue', children: '2024-01-16 - 派单至北京总店' },
              { color: 'orange', children: '2024-01-17 - 开始维修' },
            ]}
          />
        )}
      </Modal>
    </div>
  )
}
