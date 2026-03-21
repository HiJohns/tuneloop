import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { Card, Steps, Tag, Button } from 'antd';
import { CheckCircleFilled } from '@ant-design/icons';

const API_BASE = process.env.REACT_APP_API_BASE || 'http://localhost:5553';

const STEPS = [
  { title: '提交报修', key: 'pending' },
  { title: '待接单', key: 'accepted' },
  { title: '已指派', key: 'assigned' },
  { title: '取琴中', key: 'pickup' },
  { title: '维修中', key: 'processing' },
  { title: '已修复', key: 'completed' }
];

export default function MaintenanceProgress() {
  const { id } = useParams();
  const [ticket, setTicket] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchTicket();
  }, [id]);

  const fetchTicket = async () => {
    try {
      const response = await fetch(`${API_BASE}/api/maintenance/${id}`);
      const result = await response.json();
      if (result.code === 20000) {
        setTicket(result.data);
      }
    } catch (error) {
      console.error('Failed to fetch ticket:', error);
    } finally {
      setLoading(false);
    }
  };

  const getStepIndex = (status) => {
    const statusMap = {
      'pending': 0,
      'accepted': 1,
      'assigned': 2,
      'pickup': 3,
      'processing': 4,
      'completed': 5
    };
    return statusMap[status] ?? 0;
  };

  const currentStep = ticket ? getStepIndex(ticket.status) : 0;

  if (loading) return <div className="p-4">加载中...</div>;
  if (!ticket) return <div className="p-4">工单不存在</div>;

  return (
    <div className="min-h-screen bg-gray-50 p-4">
      <Card className="mb-4">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-medium">工单详情</h2>
          <Tag color={ticket.status === 'completed' ? 'green' : 'blue'}>
            {ticket.status === 'completed' ? '已完成' : '处理中'}
          </Tag>
        </div>
        
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">工单编号</span>
            <span>{ticket.id?.slice(0, 8)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">问题描述</span>
          </div>
          <p className="text-gray-800 bg-gray-50 p-2 rounded">{ticket.problem_description}</p>
        </div>
      </Card>

      <Card>
        <h3 className="font-medium mb-4">进度追踪</h3>
        <Steps
          current={currentStep}
          direction="vertical"
          items={STEPS.map((step, index) => ({
            title: step.title,
            icon: index < currentStep ? <CheckCircleFilled style={{ color: '#52c41a' }} /> : undefined,
            description: index === currentStep ? '当前状态' : undefined
          }))}
        />
      </Card>

      {ticket.estimated_cost > 0 && (
        <Card className="mt-4">
          <h3 className="font-medium mb-2">维修报价</h3>
          <p className="text-2xl font-bold text-orange-500">¥{ticket.estimated_cost}</p>
        </Card>
      )}
    </div>
  );
}
