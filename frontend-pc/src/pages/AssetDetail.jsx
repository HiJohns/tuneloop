import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Tabs, Timeline, Button, Tag, Space, Spin } from 'antd'
import { EyeOutlined, EditOutlined } from '@ant-design/icons'
import { ArrowLeft } from 'lucide-react'
import { instrumentsApi } from '../services/api'
import { INSTRUMENT_IMAGE_PLACEHOLDER, formatDailyRate } from '../utils/instrumentDisplay'
import { ASSET_DETAIL_STATUS, assetDetailState, assetHeroImage } from '../utils/assetDetail'

const { TabPane } = Tabs

export default function AssetDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [asset, setAsset] = useState(null)
  const [status, setStatus] = useState('loading')
  const [errorMessage, setErrorMessage] = useState('')

  useEffect(() => {
    loadAsset()
    // #2529: 详情按 :id 单条取数，路由参数变化时重新加载
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  // #2529: 单条详情接口（GET /instruments/:id）。
  // 旧实现无参拉列表首页再内存查找
  // → 第 2 页之后的乐器一律「资产不存在」。
  const loadAsset = async () => {
    setStatus('loading')
    setErrorMessage('')
    try {
      const response = await instrumentsApi.get(id)
      const next = assetDetailState(response)
      if (next.status === ASSET_DETAIL_STATUS.OK) {
        setAsset(next.asset)
        setStatus(ASSET_DETAIL_STATUS.OK)
        return
      }
      setAsset(null)
      if (next.status === ASSET_DETAIL_STATUS.MISSING) {
        setStatus(ASSET_DETAIL_STATUS.MISSING)
        return
      }
      setErrorMessage(next.message || '请求失败，请重试')
      setStatus(ASSET_DETAIL_STATUS.ERROR)
    } catch (error) {
      console.error('Failed to load asset:', error)
      setAsset(null)
      setErrorMessage(error?.message || '请求失败，请重试')
      setStatus(ASSET_DETAIL_STATUS.ERROR)
    }
  }

  if (status === 'loading') {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (status === ASSET_DETAIL_STATUS.ERROR) {
    return (
      <div className="p-6">
        <p className="mb-4">加载失败：{errorMessage}</p>
        <Space>
          <Button type="primary" onClick={loadAsset}>重试</Button>
          <Button onClick={() => navigate('/site/stock')}>返回库存监控</Button>
        </Space>
      </div>
    )
  }

  if (status === ASSET_DETAIL_STATUS.MISSING || !asset) {
    return (
      <div className="p-6">
        <p className="mb-4">资产不存在或已删除</p>
        <Button type="primary" onClick={() => navigate('/site/stock')}>返回库存监控</Button>
      </div>
    )
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
          {/* #2529: 页头补图（{取源与 #2528 列表同源：media.display → poster → images → 内置占位}） */}
          <div className="shrink-0">
            <img
              src={assetHeroImage(asset)}
              alt={asset.sn || '乐器图片'}
              style={{ width: 160, height: 160, objectFit: 'cover', borderRadius: 8 }}
              onError={(e) => { e.currentTarget.src = INSTRUMENT_IMAGE_PLACEHOLDER }}
            />
          </div>
          <div className="flex-1">
            <h2 className="text-2xl font-bold mb-4">{asset.sn || asset.id?.slice(0, 8) || '—'}</h2>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <span className="text-gray-500 text-sm">资产ID</span>
                <p className="font-mono font-medium">{asset.id}</p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">类别</span>
                <p className="font-medium">{asset.category_name || '—'}</p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">级别</span>
                <p><Tag color={levelColors[asset.level_name]}>{asset.level_name || '—'}</Tag></p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">状态</span>
                <p><Tag color={statusColors[asset.status]}>{asset.status}</Tag></p>
              </div>
              <div>
                <span className="text-gray-500 text-sm">所属网点</span>
                <p className="font-medium">{asset.site_name || '—'}</p>
              </div>
              <div>
                {/* #2529: 「估值」实为标准日租金，API 单位是分 → 必须经 formatCents 换算 */}
                <span className="text-gray-500 text-sm">标准日租金</span>
                <p className="font-bold text-lg">{formatDailyRate(asset)}</p>
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
