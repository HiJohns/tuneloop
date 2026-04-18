import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Button, Space, Descriptions, Tag, message } from 'antd'
import { ArrowLeftOutlined, DownloadOutlined, FileTextOutlined } from '@ant-design/icons'
import { api } from '../services/api'

export default function ContractView() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [contract, setContract] = useState(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    fetchContract()
  }, [id])

  const fetchContract = async () => {
    setLoading(true)
    try {
      const data = await api.get(`/user/contracts/${id}`)
      setContract(data)
    } catch (error) {
      console.error('Failed to fetch contract:', error)
      message.error('加载合同失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDownload = () => {
    if (contract?.contract_url) {
      window.open(contract.contract_url, '_blank')
    } else {
      message.error('合同文件暂无')
    }
  }

  if (loading || !contract) return <div className="p-6">加载中...</div>

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <Button 
        icon={<ArrowLeftOutlined />} 
        className="mb-6"
        onClick={() => navigate(-1)}
      >
        返回
      </Button>

      <Card 
        title={
          <Space>
            <FileTextOutlined />
            <span>电子租赁合同</span>
          </Space>
        }
        extra={
          <Button 
            icon={<DownloadOutlined />}
            onClick={handleDownload}
          >
            下载PDF
          </Button>
        }
      >
        <div className="mb-8 text-center border-b pb-6">
          <h1 className="text-2xl font-bold mb-2">乐器租赁合同</h1>
          <div className="text-gray-600">
            合同编号: {contract.contract_number || '-'}
          </div>
          <div className="text-gray-500 text-sm mt-2">
            签约日期: {contract.signed_at?.slice(0, 10) || '-'}
          </div>
        </div>

        <Descriptions bordered column={2} className="mb-6">
          <Descriptions.Item label="出租方">TuneLoop 乐器租赁平台</Descriptions.Item>
          <Descriptions.Item label="承租方">{contract.user_name || '用户'}</Descriptions.Item>
          <Descriptions.Item label="乐器名称">{contract.instrument_name}</Descriptions.Item>
          <Descriptions.Item label="品牌型号">{contract.brand} {contract.model}</Descriptions.Item>
          <Descriptions.Item label="租赁开始">{contract.start_date?.slice(0, 10)}</Descriptions.Item>
          <Descriptions.Item label="租赁结束">{contract.end_date?.slice(0, 10)}</Descriptions.Item>
          <Descriptions.Item label="月租金">¥{contract.monthly_rent}</Descriptions.Item>
          <Descriptions.Item label="押金">¥{contract.deposit}</Descriptions.Item>
          <Descriptions.Item label="合同状态" span={2}>
            <Tag color={contract.status === 'active' ? 'green' : 'default'}>
              {contract.status === 'active' ? '生效中' : '已完成'}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="租赁条款" span={2}>
            <div className="text-sm text-gray-600 space-y-1">
              <div>1. 承租方应妥善保管乐器，如有损坏需照价赔偿</div>
              <div>2. 租赁期满后应及时归还，逾期将收取滞纳金</div>
              <div>3. 如需续租，请提前7天联系平台</div>
              <div>4. 押金在归还验收合格后7个工作日内退还</div>
            </div>
          </Descriptions.Item>
        </Descriptions>

        <div className="text-center text-gray-500 text-sm mt-8 pt-6 border-t">
          <div>本合同由 TuneLoop 平台自动生成，具有法律效力</div>
          <div>如有疑问请联系客服: 400-123-4567</div>
        </div>
      </Card>
    </div>
  )
}