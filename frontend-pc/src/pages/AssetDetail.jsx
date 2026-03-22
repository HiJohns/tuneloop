import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Tabs, Timeline, Button, Tag, Space, Spin } from 'antd'
import { EyeOutlined, EditOutlined } from '@ant-design/icons'
import { ArrowLeft } from 'lucide-react'
import { inventoryApi } from '../services/api'

const { TabPane } = Tabs

export default function AssetDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [asset, setAsset] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadAsset()
  }, [id])

  const loadAsset = async () => {
    setLoading(true)
    try {
      const data = await inventoryApi.list()
      const found = (data || []).find(a => a.id === id)
      setAsset(found || null)
    } catch (error) {
      console.error('Failed to load asset:', error)
      setAsset(null)
    } finally {
      setLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (!asset) {
    return <div className="p-6">资产不存在</div>
  }

  const statusColors = {
    "在租": "green",
    "待租": "blue",
    "维修中": "orange",
    "rented": "green",
    "available": "blue",
    "maintenance": "orange",
  }

  const levelColors = {
    "入门级": "default",
    "专业级": "blue",
    "大师级": "gold",
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <Button 
          type="link" 
          icon={<ArrowLeft />}
          onClick={() => navigate('/site/stock')}
          className="mb-4"
        >
          返回
        </Button>
      </div>

      <Card className="mb-6 shadow-sm">
        <div className="flex gap-6">
          <div className="flex-1">
            <h2 className="text-2xl font-bold mb-4">{asset.name}</h2>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <span className="text-gray-500 text-sm">资产ID</span>
                <p className="font-mono font-medium">{asset.id}</p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">类别</span>
                <p className="font-medium">{asset.category}</p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">级别</span>
                <p><Tag color={levelColors[asset.level]}>{asset.level}</Tag></p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">状态</span>
                <p><Tag color={statusColors[asset.status]}>{asset.status}</Tag></p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">所属网点</span>
                <p className="font-medium">{asset.site}</p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">估值</span>
                <p className="font-bold text-lg">¥{(asset.value || 0).toLocaleString()}</p>
              </div>
              {asset.leaseEnd && (
                <div>
                  <span className="text-gray-500 text-sm">到期日</span>
                  <p className="font-medium">{asset.leaseEnd}</p>
                </div>
              )}
            </div>
          </div>
          
          <div className="flex flex-col gap-2">
            <Button type="primary" icon={<EyeOutlined />}>查看详情</Button>
            <Button icon={<EditOutlined />}>编辑</Button>
          </div>
        </div>
      </Card>

      <Tabs defaultActiveKey="timeline">
        <TabPane tab="流转轨迹" key="timeline">
          <Card>
            <Timeline>
              <Timeline.Item>
                <p className="text-gray-500 text-sm">当前状态: {asset.status}</p>
              </Timeline.Item>
            </Timeline>
          </Card>
        </TabPane>

        <TabPane tab="关联单据" key="documents">
          <Card>
            <div className="space-y-4">
              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">租约合同</h4>
                <p className="text-gray-600 text-sm mb-2">暂无租约数据</p>
              </div>
              
              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">维保工单</h4>
                <p className="text-gray-500 text-sm">暂无维保记录</p>
              </div>

              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">财务单据</h4>
                <p className="text-gray-500 text-sm">暂无财务数据</p>
              </div>
            </div>
          </Card>
        </TabPane>
      </Tabs>
    </div>
  )
}
