import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Button, Space, Descriptions, message, Modal, Form, Input, Radio } from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, CreditCardOutlined } from '@ant-design/icons'
import { api } from '../services/api'

const { TextArea } = Input

export default function OrderPayment() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchOrder()
  }, [id])

  const fetchOrder = async () => {
    setLoading(true)
    try {
      const data = await api.get(`/user/orders/${id}`)
      setOrder(data)
    } catch (error) {
      console.error('Failed to fetch order:', error)
      message.error('加载订单失败')
    } finally {
      setLoading(false)
    }
  }

  const handlePayment = async (values) => {
    try {
      await api.post(`/user/orders/${id}/pay`, {
        payment_method: values.payment_method,
        delivery_address: values.delivery_address
      })
      message.success('支付成功！')
      navigate('/user/rentals')
    } catch (error) {
      console.error('Payment failed:', error)
      message.error('支付失败')
    }
  }

  const handleCancel = () => {
    Modal.confirm({
      title: '确认取消订单？',
      content: '取消后订单将无法恢复',
      onOk: async () => {
        try {
          await api.post(`/user/orders/${id}/cancel`)
          message.success('订单已取消')
          navigate('/instruments')
        } catch (error) {
          message.error('取消失败')
        }
      }
    })
  }

  if (loading || !order) return <div className="p-6">加载中...</div>

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <Button 
        icon={<ArrowLeftOutlined />} 
        className="mb-6"
        onClick={() => navigate(-1)}
      >
        返回
      </Button>

      <Card title="订单支付" className="mb-6">
        <Descriptions bordered column={2}>
          <Descriptions.Item label="订单号">{order.id?.slice(0, 8)}</Descriptions.Item>
          <Descriptions.Item label="乐器">{order.instrument_name}</Descriptions.Item>
          <Descriptions.Item label="租赁开始">{order.start_date}</Descriptions.Item>
          <Descriptions.Item label="租赁结束">{order.end_date}</Descriptions.Item>
          <Descriptions.Item label="天数">{order.lease_term} 天</Descriptions.Item>
          <Descriptions.Item label="月租金">¥{order.monthly_rent}</Descriptions.Item>
          <Descriptions.Item label="押金">¥{order.deposit}</Descriptions.Item>
          <Descriptions.Item label="总计">
            <span className="text-2xl font-bold text-blue-600">
              ¥{order.total_amount || order.monthly_rent}
            </span>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="配送信息" className="mb-6">
        <Form
          form={form}
          layout="vertical"
          onFinish={handlePayment}
          initialValues={{
            payment_method: 'wechat',
            delivery_address: ''
          }}
        >
          <Form.Item
            name="delivery_address"
            label="配送地址"
            rules={[{ required: true, message: '请输入配送地址' }]}
          >
            <TextArea 
              rows={3}
              placeholder="请输入详细的配送地址（包括街道、门牌号等）"
            />
          </Form.Item>

          <Form.Item
            name="payment_method"
            label="支付方式"
            rules={[{ required: true }]}
          >
            <Radio.Group>
              <Space direction="vertical">
                <Radio value="wechat">
                  <CreditCardOutlined className="mr-2" />
                  微信支付
                </Radio>
                <Radio value="alipay">
                  <CreditCardOutlined className="mr-2" />
                  支付宝
                </Radio>
                <Radio value="card">
                  <CreditCardOutlined className="mr-2" />
                  银行卡
                </Radio>
              </Space>
            </Radio.Group>
          </Form.Item>
        </Form>
      </Card>

      <Space className="w-full justify-center">
        <Button 
          size="large"
          danger
          onClick={handleCancel}
        >
          取消订单
        </Button>
        <Button 
          type="primary" 
          size="large"
          icon={<CheckCircleOutlined />}
          onClick={() => form.submit()}
        >
          确认支付
        </Button>
      </Space>
    </div>
  )
}