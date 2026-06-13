import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components';
import { apiFetch } from '../services/api';
import { Card, Steps, Tag, Button as AntButton } from 'antd';
import { CheckCircleFilled } from '@ant-design/icons';

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api';

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

  const fetchTicket = useCallback(async () => {
    try {
      const response = await apiFetch(`${API_BASE}/maintenance/${id}`);
      const result = await response.json();
      if (result.code === 20000) {
        setTicket(result.data);
      }
    } catch (error) {
      console.error('Failed to fetch ticket:', error);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchTicket();
  }, [fetchTicket]);

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

  if (loading) return <View className="p-4">加载中...</View>;
  if (!ticket) return <View className="p-4">工单不存在</View>;

  return (
    <View className="min-h-screen bg-gray-50 p-4">
      <Card className="mb-4">
        <View className="flex justify-between items-center mb-4">
          <Text className="text-lg font-medium">工单详情</Text>
          <Tag color={ticket.status === 'completed' ? 'green' : 'blue'}>
            {ticket.status === 'completed' ? '已完成' : '处理中'}
          </Tag>
        </View>
        
        <View className="space-y-2 text-sm">
          <View className="flex justify-between">
            <Text className="text-gray-500">工单编号</Text>
            <Text>{ticket.id?.slice(0, 8)}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">问题描述</Text>
          </View>
          <Text className="text-gray-800 bg-gray-50 p-2 rounded">{ticket.problem_description}</Text>
        </View>
      </Card>

      <Card>
        <Text className="font-medium mb-4">进度追踪</Text>
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
          <Text className="font-medium mb-2">维修报价</Text>
          <Text className="text-2xl font-bold text-orange-500">¥{ticket.estimated_cost}</Text>
        </Card>
      )}
    </View>
  );
}
