import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Tabs, Timeline, Button, Tag, Space } from 'antd'
import { ArrowLeft, EyeOutlined, EditOutlined } from '@ant-design/icons'
import { assets } from '../data/mockData'

const { TabPane } = Tabs

export default function AssetDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const asset = assets.find(a => a.id === id)

  if (!asset) {
    return <div className="p-6">资产不存在</div>
  }

  const statusColors = {
    "在租": "green",
    "待租": "blue",
    "维修中": "orange"
  }

  const levelColors = {
    "入门级": "default",
    "专业级": "blue",
    "大师级": "gold"
  }

  return (
    <div className="p-6">
      {/* Header with Back Button */}
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

      {/* Asset Summary Card */}
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
                <p className="font-bold text-lg">¥{asset.value.toLocaleString()}</p>
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

      {/* Tabs */}
      <Tabs defaultActiveKey="timeline">
        <TabPane tab="流转轨迹" key="timeline">
          <Card>
            <Timeline>
              {asset.history.map((h, idx) => (
                <Timeline.Item 
                  key={idx}
                  color={h.action === '维修' ? 'orange' : 'green'}
                >
                  <div>
                    <p className="font-medium">{h.date} - {h.action}</p>
                    {h.renter && <p className="text-gray-600 text-sm">租户: {h.renter}</p>}
                    {h.note && <p className="text-gray-600 text-sm">备注: {h.note}</p>}
                  </div>
                </Timeline.Item>
              ))}
              <Timeline.Item>
                <p className="text-gray-500 text-sm">当前状态: {asset.status}（已租 {asset.rentMonths || 3} 个月）</p>
                <p className="text-gray-500 text-sm">维修次数: {asset.repairCount} 次</p>
              </Timeline.Item>
            </Timeline>
          </Card>
        </TabPane>

        <TabPane tab="关联单据" key="documents">
          <Card>
            <div className="space-y-4">
              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">租约合同</h4>
                <p className="text-gray-600 text-sm mb-2">合同编号: {asset.id.replace(/-/g, '')}-LEASE-2026</p>
                <p className="text-gray-600 text-sm">起租日期: {asset.history.find(h => h.action === '出租')?.date || '2026-01-01'}</p>
                <Button type="link" size="small" className="mt-2">查看合同</Button>
              </div>
              
              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">维保工单</h4>
                {asset.repairCount > 0 ? (
                  <div className="space-y-2">
                    <p className="text-gray-600 text-sm">共 {asset.repairCount} 条维保记录</p>
                    {asset.history.filter(h => h.action === '维修').map((h, idx) => (
                      <div key={idx} className="text-gray-600 text-sm pl-4">
                        • {h.date}: {h.note}
                      </div>
                    ))}
                    <Button type="link" size="small" className="mt-2">查看工单</Button>
                  </div>
                ) : (
                  <p className="text-gray-500 text-sm">暂无维保记录</p>
                )}
              </div>

              <div className="p-4 border rounded-lg">
                <h4 className="font-medium mb-2">财务单据</h4>
                <p className="text-gray-600 text-sm mb-2">押金: ¥{((asset.value * 0.2) || 10000).toLocaleString()}</p>
                <p className="text-gray-600 text-sm">月租金: ¥{Math.round(asset.value * 0.02).toLocaleString()}</p>
                <Button type="link" size="small" className="mt-2">查看账单</Button>
              </div>
            </div>
          </Card>
        </TabPane>
      </Tabs>
    </div>
  )
}