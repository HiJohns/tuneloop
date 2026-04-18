import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Button, Space, Tag, Image, Descriptions, message, Modal } from 'antd'
import { ShoppingCartOutlined, ArrowLeftOutlined, FileTextOutlined } from '@ant-design/icons'
import { api } from '../services/api'

export default function InstrumentDetailUser() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(false)
  const [selectedImage, setSelectedImage] = useState(0)
  const [orderModalVisible, setOrderModalVisible] = useState(false)
  const [rentalDays, setRentalDays] = useState(7)

  useEffect(() => {
    fetchInstrumentDetail()
  }, [id])

  const fetchInstrumentDetail = async () => {
    setLoading(true)
    try {
      const data = await api.get(`/user/instruments/${id}`)
      setInstrument(data)
    } catch (error) {
      console.error('Failed to fetch instrument:', error)
      message.error('加载乐器详情失败')
    } finally {
      setLoading(false)
    }
  }

  const handleRent = async () => {
    try {
      // Calculate dates
      const startDate = new Date()
      const endDate = new Date()
      endDate.setDate(startDate.getDate() + rentalDays)
      
      const orderData = {
        instrument_id: id,
        start_date: startDate.toISOString().split('T')[0],
        end_date: endDate.toISOString().split('T')[0],
        delivery_address: {
          street: '用户地址',
          city: '城市',
          phone: '联系电话'
        }
      }
      
      const response = await api.post('/user/orders', orderData)
      message.success('订单创建成功，跳转到支付页面...')
      
      // Navigate to payment page
      navigate(`/orders/${response.order_id}/payment`)
    } catch (error) {
      console.error('Failed to create order:', error)
      message.error('创建订单失败')
    }
  }

  if (loading || !instrument) {
    return (
      <div className="p-6 text-center">
        <div className="text-lg text-gray-500">加载中...</div>
      </div>
    )
  }

  const images = instrument.images || []

  return (
    <div className="p-6">
      <Button 
        icon={<ArrowLeftOutlined />} 
        className="mb-6"
        onClick={() => navigate('/instruments')}
      >
        返回列表
      </Button>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Images Section */}
        <Card>
          <div className="space-y-4">
            {/* Main Image */}
            <div className="aspect-square bg-gray-100 rounded-lg overflow-hidden">
              {images.length > 0 ? (
                <Image
                  src={images[selectedImage]}
                  alt={instrument.name}
                  className="w-full h-full object-cover"
                  preview={{
                    mask: <div className="text-white">点击查看大图</div>
                  }}
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center">
                  <div className="text-gray-400 text-center">
                    <div className="text-6xl mb-4">🎸</div>
                    <div>暂无图片</div>
                  </div>
                </div>
              )}
            </div>
            
            {/* Thumbnail Images */}
            {images.length > 1 && (
              <div className="flex gap-2 overflow-x-auto">
                {images.map((img, idx) => (
                  <div
                    key={idx}
                    className={`flex-shrink-0 w-20 h-20 rounded-lg overflow-hidden cursor-pointer border-2 ${
                      selectedImage === idx ? 'border-blue-500' : 'border-gray-200'
                    }`}
                    onClick={() => setSelectedImage(idx)}
                  >
                    <Image
                      src={img}
                      alt={`${instrument.name} ${idx + 1}`}
                      className="w-full h-full object-cover"
                      preview={false}
                    />
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>

        {/* Info Section */}
        <Space direction="vertical" size="large" className="w-full">
          <Card>
            <div className="mb-4">
              <h1 className="text-3xl font-bold mb-2">
                {instrument.brand} {instrument.name}
              </h1>
              <div className="text-gray-600">
                {instrument.category_name} | {instrument.level_name}
              </div>
            </div>

            <Descriptions bordered column={1}>
              <Descriptions.Item label="品牌">{instrument.brand}</Descriptions.Item>
              <Descriptions.Item label="型号">{instrument.model || '-'}</Descriptions.Item>
              <Descriptions.Item label="序列号">{instrument.sn || '-'}</Descriptions.Item>
              <Descriptions.Item label="库存状态">
                <Tag color={instrument.stock_status === 'available' ? 'green' : 'red'}>
                  {instrument.stock_status === 'available' ? '可租' : '已租出'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="当前位置">{instrument.site_name || '-'}</Descriptions.Item>
            </Descriptions>
          </Card>

          {/* Pricing Card */}
          <Card>
            <div className="text-center mb-6">
              <div className="text-4xl font-bold text-blue-600 mb-2">
                ¥{instrument.daily_rent}
                <span className="text-lg text-gray-500 ml-2">/天</span>
              </div>
              <div className="text-sm text-gray-500">
                周租 ¥{instrument.weekly_rent} | 月租 ¥{instrument.monthly_rent}
              </div>
              <div className="text-sm text-gray-500 mt-1">
                押金 ¥{instrument.deposit}
              </div>
            </div>

            <div className="space-y-4">
              <div>
                <div className="text-sm text-gray-600 mb-2">租赁天数</div>
                <div className="flex gap-2">
                  {[3, 7, 15, 30].map(days => (
                    <Button
                      key={days}
                      type={rentalDays === days ? 'primary' : 'default'}
                      onClick={() => setRentalDays(days)}
                    >
                      {days}天
                    </Button>
                  ))}
                </div>
              </div>

              <Button
                type="primary"
                size="large"
                block
                icon={<ShoppingCartOutlined />}
                disabled={instrument.stock_status !== 'available'}
                onClick={() => setOrderModalVisible(true)}
                className="h-12 text-lg"
              >
                {instrument.stock_status === 'available' ? '立即租赁' : '暂不可租'}
              </Button>
            </div>
          </Card>
        </Space>
      </div>

      {/* Order Confirmation Modal */}
      <Modal
        title="确认租赁"
        open={orderModalVisible}
        onCancel={() => setOrderModalVisible(false)}
        onOk={handleRent}
        okText="确认订单"
        cancelText="取消"
      >
        <div className="space-y-4">
          <div className="text-center">
            <div className="text-lg font-semibold">
              {instrument.brand} {instrument.name}
            </div>
            <div className="text-gray-600">
              租赁 {rentalDays} 天
            </div>
          </div>
          
          <div className="bg-gray-50 p-4 rounded">
            <div className="flex justify-between mb-2">
              <span>日租金:</span>
              <span>¥{instrument.daily_rent} × {rentalDays}天</span>
            </div>
            <div className="flex justify-between font-bold text-lg">
              <span>总计:</span>
              <span className="text-blue-600">¥{(instrument.daily_rent * rentalDays).toFixed(2)}</span>
            </div>
            <div className="text-sm text-gray-500 mt-2">
              * 押金: ¥{instrument.deposit} (还琴后退还)
            </div>
          </div>
        </div>
      </Modal>
    </div>
  )
}