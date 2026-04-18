import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Button, Space, Form, Input, message, Modal } from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, TruckOutlined } from '@ant-design/icons'
import { api } from '../services/api'

const { TextArea } = Input

export default function ReturnProcess() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [rental, setRental] = useState(null)
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchRental()
  }, [id])

  const fetchRental = async () => {
    setLoading(true)
    try {
      const data = await api.get(`/user/rentals/${id}`)
      setRental(data)
    } catch (error) {
      console.error('Failed to fetch rental:', error)
      message.error('加载租赁信息失败')
    } finally {
      setLoading(false)
    }
  }

  const handleReturn = async (values) => {
    try {
      await api.post(`/user/rentals/${id}/return`, {
        return_method: values.return_method,
        return_tracking: values.return_tracking,
        return_notes: values.return_notes
      })
      message.success('归还申请已提交成功！')
      navigate('/user/rentals')
    } catch (error) {
      console.error('Return failed:', error)
      message.error('提交归还申请失败')
    }
  }

  if (loading || !rental) return <div className="p-6">加载中...</div>

  return (
    <div className="p-6 max-w-2xl mx-auto">
      <Button 
        icon={<ArrowLeftOutlined />} 
        className="mb-6"
        onClick={() => navigate(-1)}
      >
        返回
      </Button>

      <Card title="归还乐器" className="mb-6">
        <div className="mb-6 p-4 bg-blue-50 rounded">
          <h3 className="font-semibold mb-2">归还信息</h3>
          <div className="text-sm text-gray-600 space-y-1">
            <div>乐器: {rental.instrument_name}</div>
            <div>租赁单号: {rental.id?.slice(0, 8)}</div>
            <div>应还日期: {rental.end_date?.slice(0, 10)}</div>
          </div>
        </div>

        <Form
          form={form}
          layout="vertical"
          onFinish={handleReturn}
          initialValues={{
            return_method: 'courier'
          }}
        >
          <Form.Item
            name="return_method"
            label="归还方式"
            rules={[{ required: true, message: '请选择归还方式' }]}
          >
            <Radio.Group>
              <Space direction="vertical">
                <Radio value="courier">
                  <TruckOutlined className="mr-2" />
                  快递寄回
                </Radio>
                <Radio value="self_return">
                  <TruckOutlined className="mr-2" />
                  自行送回网点
                </Radio>
                <Radio value="pickup">
                  <TruckOutlined className="mr-2" />
                  预约上门取件
                </Radio>
              </Space>
            </Radio.Group>
          </Form.Item>

          <Form.Item
            name="return_tracking"
            label="物流单号"
            rules={[{ required: true, message: '请输入物流单号' }]}
          >
            <Input placeholder="请输入快递单号（如SF1234567890）" />
          </Form.Item>

          <Form.Item
            name="return_notes"
            label="归还说明"
          >
            <TextArea 
              rows={3}
              placeholder="请描述乐器的使用情况和归还状态..."
            />
          </Form.Item>

          <Form.Item>
            <div className="bg-yellow-50 p-4 rounded mb-4">
              <div className="text-sm text-gray-600 space-y-1">
                <div>• 请确保乐器清洁并完整归还</div>
                <div>• 配件（琴盒、谱架等）需一并归还</div>
                <div>• 我们将在3个工作日内完成验收</div>
                <div>• 押金将在验收合格后7个工作日内退还</div>
              </div>
            </div>
          </Form.Item>

          <Form.Item>
            <Space className="w-full justify-center">
              <Button 
                size="large"
                onClick={() => navigate(-1)}
              >
                取消
              </Button>
              <Button 
                type="primary" 
                size="large"
                icon={<CheckCircleOutlined />}
                onClick={() => form.submit()}
              >
                提交归还申请
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}