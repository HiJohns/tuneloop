import { Table, Tag, Alert } from 'antd'
import { assets } from '../data/mockData'

export default function WorkOrderList() {
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
    },
    {
      title: '服务人员',
      dataIndex: 'technician',
      key: 'technician',
    },
  ]

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">工单列表</h2>
      
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
        pagination={false}
      />
    </div>
  )
}
