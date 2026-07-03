import { useState, useEffect } from 'react'
import { Card, Table, Button, Input, InputNumber, Select, Form, Space, message, Modal, Popconfirm } from 'antd'
import { PlusOutlined, DeleteOutlined, SaveOutlined, UndoOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'

const DEFAULT_CONFIG = {
  tiers: [
    { name: '短期租赁', days_max: 30, discount_percent: 0 },
    { name: '中期租赁', days_max: 180, discount_percent: 5 },
    { name: '长期租赁', days_max: -1, discount_percent: 10 },
  ],
  deposit_mode: 'ratio',
  deposit_ratio: 0.3,
  deposit_fixed: 0,
}

export default function MerchantPricingConfig() {
  const [loading, setLoading] = useState(false)
  const [tiers, setTiers] = useState([])
  const [depositMode, setDepositMode] = useState('ratio')
  const [depositRatio, setDepositRatio] = useState(2.0)
  const [depositFixed, setDepositFixed] = useState(0)
  const [templateId, setTemplateId] = useState(null)
  const [configured, setConfigured] = useState(false)
  const [templateName, setTemplateName] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    loadConfig()
  }, [])

  const loadConfig = async () => {
    setLoading(true)
    try {
      const response = await api.get('/pricing/merchant-config')
      if (response.code === 20000) {
        const data = response.data
        setTemplateId(data.template_id)
        setConfigured(data.configured || false)
        setTemplateName(data.template_name || '')
        if (data.config) {
          applyConfig(data.config)
        } else {
          applyConfig(DEFAULT_CONFIG)
        }
      } else {
        message.error('加载定价配置失败: ' + response.message)
      }
    } catch (error) {
      message.error('加载定价配置失败: ' + error.message)
    }
    setLoading(false)
  }

  const applyConfig = (config) => {
    const tiers = (config.tiers || []).map((t, i) => ({
      name: t.name || `第${i + 1}段`,
      days_max: t.days_max ?? DEFAULT_CONFIG.tiers[i]?.days_max ?? -1,
      discount_percent: t.discount_percent ?? 0,
    }))
    setTiers(tiers)
    setDepositMode(config.deposit_mode || 'ratio')
    setDepositRatio(config.deposit_ratio || 2.0)
    setDepositFixed(config.deposit_fixed || 0)
  }

  const handleTierChange = (index, field, value) => {
    setTiers(prev => prev.map((tier, i) =>
      i === index ? { ...tier, [field]: value } : tier
    ))
  }

  const addTier = () => {
    const lastTier = tiers[tiers.length - 1]
    const prevMax = lastTier ? lastTier.days_max : 0
    const newDaysMax = prevMax === -1 ? 365 : prevMax + 30
    const newName = `第${tiers.length + 1}阶梯`
    setTiers(prev => [
      ...(prev.map((t, i) => i === prev.length - 1 && t.days_max === -1
        ? { ...t, days_max: newDaysMax } : t)),
      { name: newName, days_max: -1, discount_percent: 0 }
    ])
  }

  const removeTier = (index) => {
    if (tiers.length <= 1) return
    setTiers(prev => prev.filter((_, i) => i !== index))
  }

  const handleSave = async () => {
    // Validation
    for (let i = 0; i < tiers.length; i++) {
      if (!tiers[i].name.trim()) {
        message.error(`第${i + 1}阶梯名称不能为空`)
        return
      }
      if (i < tiers.length - 1 && (tiers[i].days_max <= 0 || tiers[i].days_max === -1)) {
        message.error(`第${i + 1}阶梯的天数上限需要大于 0（最后一行可以为不限）`)
        return
      }
      if (tiers[i].discount_percent < 0 || tiers[i].discount_percent > 90) {
        message.error(`第${i + 1}阶梯的折扣率必须在 0-90 之间`)
        return
      }
    }

    const config = {
      tiers,
      deposit_mode: depositMode,
      deposit_ratio: depositMode === 'ratio' ? depositRatio : 0,
      deposit_fixed: depositMode === 'fixed' ? depositFixed : 0,
    }

    setLoading(true)
    try {
      const response = await api.put('/pricing/merchant-config', {
        template_id: templateId,
        config,
      })
      if (response.code === 20000 || response.code === 20100) {
        message.success('定价策略配置已保存')
        setSaved(true)
      } else {
        message.error('保存失败: ' + response.message)
      }
    } catch (error) {
      message.error('保存失败: ' + error.message)
    }
    setLoading(false)
  }

  const resetToDefault = () => {
    Modal.confirm({
      title: '恢复默认配置',
      content: '恢复默认配置将丢失当前所有定价策略设置，确定要继续吗？',
      onOk: () => applyConfig(DEFAULT_CONFIG),
    })
  }

  const columns = [
    {
      title: '阶梯名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (text, record, index) => (
        <Input
          value={text}
          onChange={(e) => handleTierChange(index, 'name', e.target.value)}
          placeholder="请输入阶梯名称"
        />
      ),
    },
    {
      title: '天数上限',
      dataIndex: 'days_max',
      key: 'days_max',
      width: 150,
      render: (value, record, index) => {
        const isLast = index === tiers.length - 1
        return (
          <InputNumber
            min={1}
            value={isLast ? -1 : value}
            disabled={isLast}
            onChange={(val) => handleTierChange(index, 'days_max', val || 0)}
            style={{ width: '100%' }}
            formatter={(val) => isLast ? '不限' : `${val}天`}
            parser={(val) => parseInt(val?.replace(/天/g, '') || '0')}
          />
        )
      },
    },
    {
      title: '折扣率(%)',
      dataIndex: 'discount_percent',
      key: 'discount_percent',
      width: 150,
      render: (value, record, index) => (
        <InputNumber
          min={0}
          max={90}
          value={value}
          onChange={(val) => handleTierChange(index, 'discount_percent', val || 0)}
          style={{ width: '100%' }}
          formatter={(val) => `${val}%`}
          parser={(val) => parseInt(val?.replace('%', '') || '0')}
        />
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_, record, index) => (
        <Popconfirm
          title="确定删除此阶梯？"
          onConfirm={() => removeTier(index)}
          disabled={tiers.length <= 1}
        >
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            disabled={tiers.length <= 1}
          />
        </Popconfirm>
      ),
    },
  ]

  return (
    <div className="p-6">
      <Card
        title={
          <Space>
            <span>定价策略配置</span>
            {configured ? (
              <span style={{ fontSize: 12, color: '#1677ff', background: '#e6f4ff', padding: '2px 8px', borderRadius: 4 }}>自定义策略</span>
            ) : (
              <span style={{ fontSize: 12, color: '#52c41a', background: '#f6ffed', padding: '2px 8px', borderRadius: 4 }}>系统默认策略</span>
            )}
          </Space>
        }
        extra={
          <Space>
            <Button icon={<UndoOutlined />} onClick={resetToDefault}>恢复默认</Button>
            <Button type="primary" icon={<SaveOutlined />} onClick={handleSave} loading={loading}>
              保存配置
            </Button>
          </Space>
        }
      >
        <div className="mb-6">
          <h3 className="text-base font-medium mb-2">定价阶梯</h3>
          <p className="text-gray-500 text-sm mb-3">
            定义租赁天数区间和对应的折扣率。最后一行默认为"不限"，自动覆盖所有剩余天数。
          </p>
          <Table
            columns={columns}
            dataSource={tiers}
            rowKey={(_, index) => index}
            pagination={false}
            loading={loading}
            scroll={{ x: true }}
            footer={() => (
              <Button type="dashed" icon={<PlusOutlined />} onClick={addTier} block>
                添加阶梯
              </Button>
            )}
          />
        </div>

        <div className="mb-6">
          <h3 className="text-base font-medium mb-2">押金设置</h3>
          <Space align="start" size="large">
            <Form.Item label="押金模式">
              <Select value={depositMode} onChange={setDepositMode} style={{ width: 160 }}>
                <Select.Option value="ratio">按日均价倍率</Select.Option>
                <Select.Option value="fixed">固定金额</Select.Option>
              </Select>
            </Form.Item>

            {depositMode === 'ratio' && (
              <Form.Item label="押金倍率">
                <InputNumber
                  min={0.5}
                  max={5}
                  step={0.5}
                  value={depositRatio}
                  onChange={setDepositRatio}
                  style={{ width: 120 }}
                  formatter={(val) => `${val}倍`}
                  parser={(val) => parseFloat(val?.replace('倍', '') || '0')}
                />
              </Form.Item>
            )}

            {depositMode === 'fixed' && (
              <Form.Item label="固定押金金额">
                <InputNumber
                  min={0}
                  step={100}
                  value={depositFixed}
                  onChange={setDepositFixed}
                  style={{ width: 160 }}
                  formatter={(val) => `¥ ${val}`}
                  parser={(val) => parseFloat(val?.replace(/¥\s?/g, '') || '0')}
                />
              </Form.Item>
            )}
          </Space>
        </div>

        <div className="bg-gray-50 p-4 rounded">
          <h3 className="text-base font-medium mb-2">阶梯价格预览</h3>
          <p className="text-gray-500 text-sm mb-2">
            假设第一阶梯日均价 ¥100：
          </p>
          {tiers.map((tier, i) => {
            const rate = tier.discount_percent > 0
              ? 100 * (1 - tier.discount_percent / 100)
              : 100
            const daysDisplay = tier.days_max === -1 ? `${(tiers[i-1]?.days_max || 0) + 1}天以上` : `1-${tier.days_max}天`
            return (
              <div key={i} className="text-sm py-1">
                <span className="font-medium">{tier.name}</span>
                <span className="text-gray-400 ml-2">({daysDisplay})</span>
                <span className="ml-4">¥{rate.toFixed(0)}/天</span>
                {tier.discount_percent > 0 && (
                  <span className="text-green-600 ml-2">({tier.discount_percent}%折扣)</span>
                )}
              </div>
            )
          })}
          <div className="text-sm py-1 border-t mt-2 pt-2">
            押金: ¥{depositMode === 'ratio' ? `${depositRatio * 100}` : depositFixed}
          </div>
        </div>
      </Card>
    </div>
  )
}
