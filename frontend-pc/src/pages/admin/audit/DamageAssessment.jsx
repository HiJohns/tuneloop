import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Checkbox, message, Card, Row, Col, Image, Divider } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import SignatureCanvas from 'react-signature-canvas'
import { api } from '../../../services/api'

export default function DamageAssessment() {
  const { orderId } = useParams()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [outboundPhotos, setOutboundPhotos] = useState([])
  const [returnPhotos, setReturnPhotos] = useState([])
  const [hasDamage, setHasDamage] = useState(false)
  const [sigPad, setSigPad] = useState(null)

  useEffect(() => {
    fetchAssessmentData()
  }, [orderId])

  const fetchAssessmentData = async () => {
    try {
      setLoading(true)
      const result = await api.get(`/orders/${orderId}/assessment`)
      if (result.code === 20000) {
        setOrder(result.data.order)
        setInstrument(result.data.instrument)
        setOutboundPhotos(result.data.outboundPhotos || [])
        setReturnPhotos(result.data.returnPhotos || [])
      } else {
        message.error('获取数据失败: ' + result.message)
      }
    } catch (error) {
      message.error('获取数据失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async () => {
    try {
      setLoading(true)
      
      const signature = sigPad ? sigPad.toDataURL() : null
      
      const payload = {
        hasDamage,
        signature,
        assessedAt: new Date().toISOString()
      }
      
      const result = await api.post(`/orders/${orderId}/assessment`, payload)
      
      if (result.code === 20000) {
        message.success('定损报告已提交')
        navigate('/admin/orders')
      } else {
        message.error('提交失败: ' + result.message)
      }
    } catch (error) {
      message.error('提交失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleBack = () => {
    navigate(-1)
  }

  return (
    <div className="p-6 min-h-screen bg-gray-50">
      <div className="mb-6">
        <Button icon={<ArrowLeftOutlined />} onClick={handleBack} className="mr-4">
          返回
        </Button>
        <h1 className="text-2xl font-bold text-gray-900 inline-block">归还定损鉴定</h1>
      </div>

      {instrument && (
        <Card className="mb-6" title="资产信息">
          <Row gutter={16}>
            <Col span={8}>
              <p><strong>乐器名称:</strong> {instrument.name}</p>
            </Col>
            <Col span={8}>
              <p><strong>品牌:</strong> {instrument.brand}</p>
            </Col>
            <Col span={8}>
              <p><strong>型号:</strong> {instrument.model}</p>
            </Col>
          </Row>
        </Card>
      )}

      <Row gutter={16} className="mb-6">
        <Col span={12}>
          <Card title="出库照片">
            <div className="grid grid-cols-2 gap-4">
              {outboundPhotos.map((photo, index) => (
                <Image key={index} src={photo} alt={`出库照片${index + 1}`} className="rounded" />
              ))}
            </div>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="归还照片">
            <div className="grid grid-cols-2 gap-4">
              {returnPhotos.map((photo, index) => (
                <Image key={index} src={photo} alt={`归还照片${index + 1}`} className="rounded" />
              ))}
            </div>
          </Card>
        </Col>
      </Row>

      <Card className="mb-6" title="定损评估">
        <div className="mb-4">
          <Checkbox 
            checked={hasDamage} 
            onChange={e => setHasDamage(e.target.checked)}
          >
            有损坏
          </Checkbox>
        </div>

        <div className="mb-4">
          <p className="font-semibold mb-2">电子签名:</p>
          <div className="border rounded p-4 bg-white">
            <SignatureCanvas
              ref={ref => setSigPad(ref)}
              canvasProps={{ className: 'sigCanvas', width: 500, height: 200 }}
              backgroundColor="#f0f0f0"
            />
          </div>
          <Button 
            size="small" 
            onClick={() => sigPad && sigPad.clear()}
            className="mt-2"
          >
            清除签名
          </Button>
        </div>
      </Card>

      <div className="text-center">
        <Button 
          type="primary" 
          size="large" 
          loading={loading}
          onClick={handleSubmit}
        >
          提交定损报告
        </Button>
      </div>
    </div>
  )
}