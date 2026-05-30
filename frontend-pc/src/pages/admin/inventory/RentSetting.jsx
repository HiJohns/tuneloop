import { useState, useEffect } from 'react'
import { Table, InputNumber, Button, Space, Form, message, Card, Input, Select, Tabs, Modal, Tag, Tooltip } from 'antd'
import { InfoCircleOutlined, EditOutlined, UndoOutlined } from '@ant-design/icons'
import { inventoryApi, api } from '../../../services/api'

const { Search } = Input
const { TabPane } = Tabs

function PricingPreview({ baseDailyRate, config }) {
  if (!config || !config.tiers || baseDailyRate <= 0) return null
  return (
    <div className="bg-gray-50 p-3 rounded text-sm mt-2">
      <p className="font-medium mb-1">阶梯价格（基于第一阶梯日均价 ¥{baseDailyRate}）：</p>
      {config.tiers.map((tier, i) => {
        const rate = tier.discount_percent > 0
          ? baseDailyRate * (1 - tier.discount_percent / 100)
          : baseDailyRate
        const prevDays = i > 0 ? config.tiers[i - 1].days_max : 0
        const daysDisplay = tier.days_max === -1
          ? `${prevDays + 1}天以上`
          : `1-${tier.days_max}天`
        return (
          <div key={i} className="py-0.5">
            <span className="font-medium">{tier.name}</span>
            <span className="text-gray-400 ml-1">({daysDisplay})</span>
            <span className="ml-2">¥{rate.toFixed(0)}/天</span>
            {tier.discount_percent > 0 && (
              <Tag color="green" className="ml-2">{tier.discount_percent}%折扣</Tag>
            )}
          </div>
        )
      })}
      <div className="border-t mt-1 pt-1 text-gray-500">
        押金: ¥{config.deposit_mode === 'ratio'
          ? (baseDailyRate * (config.deposit_ratio || 2)).toFixed(0)
          : (config.deposit_fixed || 0)}
      </div>
    </div>
  )
}

function OverrideModal({ visible, record, onClose, onSave }) {
  const [dailyRent, setDailyRent] = useState(null)
  const [deposit, setDeposit] = useState(null)

  useEffect(() => {
    if (record) {
      setDailyRent(record.base_daily_rate || null)
      setDeposit(null)
    }
  }, [record])

  const handleSave = () => {
    onSave(record.id, { daily_rent: dailyRent, deposit })
    onClose()
  }

  return (
    <Modal
      title={`手动覆盖 — ${record?.sn || ''}`}
      open={visible}
      onOk={handleSave}
      onCancel={onClose}
      okText="保存覆盖"
      cancelText="取消"
    >
      <Form layout="vertical">
        <Form.Item label="日均价（留空=按商户公式计算）">
          <InputNumber
            min={0}
            precision={2}
            value={dailyRent}
            onChange={setDailyRent}
            style={{ width: '100%' }}
            formatter={(val) => val ? `¥ ${val}` : ''}
            parser={(val) => parseFloat(val?.replace(/¥\s?/g, '') || '')}
          />
        </Form.Item>
        <Form.Item label="押金（留空=按商户公式计算）">
          <InputNumber
            min={0}
            precision={2}
            value={deposit}
            onChange={setDeposit}
            style={{ width: '100%' }}
            formatter={(val) => val ? `¥ ${val}` : ''}
            parser={(val) => parseFloat(val?.replace(/¥\s?/g, '') || '')}
          />
        </Form.Item>
        <p className="text-gray-400 text-xs">设置后该乐器不再按商户阶梯公式自动计算</p>
      </Form>
    </Modal>
  )
}

export default function InventoryRentSetting() {
  const [activeTab, setActiveTab] = useState('instruments')
  const [loading, setLoading] = useState(false)
  const [instruments, setInstruments] = useState([])
  const [editedIds, setEditedIds] = useState(new Set())
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20,
    total: 0,
  })
  const [filters, setFilters] = useState({ brand: '', model: '', category_id: '', level_id: '' })
  const [tableData, setTableData] = useState([])
  const [overrideModal, setOverrideModal] = useState({ visible: false, record: null })
  const [merchantConfig, setMerchantConfig] = useState(null)
  const [expandedRows, setExpandedRows] = useState([])

  // Batch pricing state
  const [batchItems, setBatchItems] = useState([])
  const [batchBaseRate, setBatchBaseRate] = useState(null)

  useEffect(() => {
    loadPricingConfig()
  }, [])

  useEffect(() => {
    if (activeTab === 'instruments') loadData()
  }, [pagination.current, pagination.pageSize, filters, activeTab])

  useEffect(() => {
    setTableData(instruments)
  }, [instruments])

  const loadPricingConfig = async () => {
    try {
      const response = await api.get('/pricing/merchant-config')
      if (response.code === 20000 && response.data.configured) {
        setMerchantConfig(response.data.config)
      }
    } catch (e) { /* config not configured yet, ignore */ }
  }

  const loadData = async () => {
    setLoading(true)
    try {
      const response = await inventoryApi.getRentSetting({
        page: pagination.current,
        pageSize: pagination.pageSize,
        ...filters,
      })
      if (response.code === 20000) {
        setInstruments(response.data.list || [])
        setPagination({ ...pagination, total: response.data.total })
      } else {
        message.error('加载数据失败: ' + response.message)
      }
    } catch (error) {
      message.error('加载数据失败: ' + error.message)
    }
    setLoading(false)
  }

  const handleRentChange = (id, field, value) => {
    setEditedIds(prev => new Set(prev).add(id))
    setTableData(prev => prev.map(item => {
      if (item.id !== id) return item
      const next = { ...item, [field]: value }
      if (field === 'daily_rent') {
        const newDaily = parseFloat(value) || 0
        const oldOverdue = parseFloat(item.overdue_daily_fee) || 0
        if (newDaily > 0 && (oldOverdue === 0 || oldOverdue === parseFloat(item.daily_rent) || 0)) {
          next.overdue_daily_fee = newDaily
        }
      }
      return next
    }))
  }

  const handleSave = async () => {
    if (editedIds.size === 0) {
      message.info('没有需要保存的修改')
      return
    }
    const itemsToUpdate = tableData
      .filter(item => editedIds.has(item.id))
      .map(item => ({
        id: item.id,
        daily_rent: item.daily_rent,
        deposit: item.deposit || 0,
        shipping_fee: item.shipping_fee || 0,
        overdue_daily_fee: item.overdue_daily_fee || item.daily_rent,
      }))
    setLoading(true)
    try {
      const response = await inventoryApi.batchUpdateRent({ items: itemsToUpdate })
      if (response.code === 20000) {
        message.success(`成功更新 ${response.data.updated} 条记录`)
        setEditedIds(new Set())
        await loadData()
      } else {
        message.error('保存失败: ' + response.message)
      }
    } catch (error) {
      message.error('保存失败: ' + error.message)
    }
    setLoading(false)
  }

  const handleSaveBatch = async () => {
    const items = batchItems.filter(item => item.base_daily_rate > 0)
    if (items.length === 0) {
      message.info('没有需要保存的定价')
      return
    }
    setLoading(true)
    try {
      const response = await api.put('/instruments/batch-pricing', {
        items: items.map(item => ({
          id: item.id,
          base_daily_rate: parseFloat(item.base_daily_rate) || 0,
        })),
      })
      if (response.code === 20000) {
        message.success(`成功更新 ${response.data.updated} 条定价`)
        loadBatchInstruments()
      } else {
        message.error('保存失败: ' + response.message)
      }
    } catch (error) {
      message.error('保存失败: ' + error.message)
    }
    setLoading(false)
  }

  const handleOverrideSave = async (id, overrides) => {
    setLoading(true)
    try {
      const response = await api.put('/instruments/batch-pricing', {
        items: [{ id, overrides }],
      })
      if (response.code === 20000) {
        message.success('覆盖已保存')
        loadData()
      }
    } catch (error) {
      message.error('保存失败: ' + error.message)
    }
    setLoading(false)
  }

  const loadBatchInstruments = async () => {
    setLoading(true)
    try {
      const response = await inventoryApi.getRentSetting({ page: 1, pageSize: 200 })
      if (response.code === 20000) {
        setBatchItems((response.data.list || []).map(item => ({
          id: item.id,
          sn: item.sn,
          category_name: item.category_name,
          site_name: item.site_name,
          base_daily_rate: item.base_daily_rate || '',
        })))
      }
    } catch (error) {
      message.error('加载数据失败: ' + error.message)
    }
    setLoading(false)
  }

  useEffect(() => {
    if (activeTab === 'batch') loadBatchInstruments()
  }, [activeTab])

  const instrumentColumns = [
    { title: '识别码', dataIndex: 'sn', key: 'sn', width: 120 },
    { title: '分类', dataIndex: 'category_name', key: 'category_name', width: 100 },
    { title: '级别', dataIndex: 'level_name', key: 'level_name', width: 80 },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 100, render: (text) => text || '-' },
    { title: '型号', dataIndex: 'model', key: 'model', width: 100, render: (text) => text || '-' },
    { title: '网点', dataIndex: 'site_name', key: 'site_name', width: 120 },
    {
      title: '第一阶梯日均价',
      dataIndex: 'daily_rent',
      key: 'daily_rent',
      width: 140,
      render: (value, record) => (
        <InputNumber
          min={0}
          precision={2}
          value={value}
          onChange={(val) => handleRentChange(record.id, 'daily_rent', val)}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`}
          parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
    {
      title: '阶梯价格',
      key: 'tiers',
      width: 100,
      render: (_, record) => (
        <Button type="link" size="small"
          onClick={() => setExpandedRows(prev =>
            prev.includes(record.id) ? prev.filter(id => id !== record.id) : [...prev, record.id]
          )}
        >
          {expandedRows.includes(record.id) ? '收起' : '查看'} ▼
        </Button>
      ),
    },
    {
      title: '押金',
      dataIndex: 'deposit',
      key: 'deposit', width: 120,
      render: (value, record) => (
        <InputNumber min={0} precision={2} value={value}
          onChange={(val) => handleRentChange(record.id, 'deposit', val)}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`} parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
    {
      title: '物流费', dataIndex: 'shipping_fee', key: 'shipping_fee', width: 120,
      render: (value, record) => (
        <InputNumber min={0} precision={2} value={value}
          onChange={(val) => handleRentChange(record.id, 'shipping_fee', val)}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`} parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
    {
      title: '逾期日费', dataIndex: 'overdue_daily_fee', key: 'overdue_daily_fee', width: 120,
      render: (value, record) => (
        <InputNumber min={0} precision={2} value={value}
          onChange={(val) => handleRentChange(record.id, 'overdue_daily_fee', val)}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`} parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
    {
      title: '操作', key: 'action', width: 100,
      render: (_, record) => (
        <Tooltip title="手动覆盖阶梯定价公式">
          <Button type="link" icon={<EditOutlined />} onClick={() => setOverrideModal({ visible: true, record })}>
            覆盖
          </Button>
        </Tooltip>
      ),
    },
  ]

  const batchColumns = [
    { title: '识别码', dataIndex: 'sn', key: 'sn', width: 120 },
    { title: '分类', dataIndex: 'category_name', key: 'category_name', width: 100 },
    { title: '网点', dataIndex: 'site_name', key: 'site_name', width: 120 },
    {
      title: '第一阶梯日均价',
      dataIndex: 'base_daily_rate',
      key: 'base_daily_rate',
      width: 180,
      render: (value, record) => (
        <InputNumber
          min={0}
          precision={2}
          value={value}
          onChange={(val) => setBatchItems(prev => prev.map(item =>
            item.id === record.id ? { ...item, base_daily_rate: val } : item
          ))}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`} parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
    {
      title: '阶梯预览',
      key: 'preview',
      width: 250,
      render: (_, record) => {
        const rate = parseFloat(record.base_daily_rate) || 0
        if (!rate || !merchantConfig) return <span className="text-gray-400">填日均价后预览</span>
        return <PricingPreview baseDailyRate={rate} config={merchantConfig} />
      },
    },
  ]

  const expandedRowRender = (record) => {
    const rate = parseFloat(record.daily_rent) || parseFloat(record.base_daily_rate) || 0
    if (!rate || !merchantConfig) return <span className="text-gray-400">未设置日均价或无商户定价策略</span>
    return <PricingPreview baseDailyRate={rate} config={merchantConfig} />
  }

  return (
    <div className="p-6">
      <Card>
        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          <TabPane tab="全部乐器" key="instruments">
            <div className="mb-4">
              <Space wrap>
                <Search
                  placeholder="品牌" value={filters.brand}
                  onChange={(e) => setFilters({ ...filters, brand: e.target.value })}
                  style={{ width: 150 }}
                  onSearch={() => setPagination({ ...pagination, current: 1 })}
                />
                <Search
                  placeholder="型号" value={filters.model}
                  onChange={(e) => setFilters({ ...filters, model: e.target.value })}
                  style={{ width: 150 }}
                  onSearch={() => setPagination({ ...pagination, current: 1 })}
                />
                <Search
                  placeholder="分类" value={filters.category_id}
                  onChange={(e) => setFilters({ ...filters, category_id: e.target.value })}
                  style={{ width: 150 }}
                  onSearch={() => setPagination({ ...pagination, current: 1 })}
                />
                <Search
                  placeholder="等级" value={filters.level_id}
                  onChange={(e) => setFilters({ ...filters, level_id: e.target.value })}
                  style={{ width: 150 }}
                  onSearch={() => setPagination({ ...pagination, current: 1 })}
                />
              </Space>
            </div>
            <div className="mb-4 text-right">
              <Space>
                <Button onClick={loadData} loading={loading}>刷新</Button>
                <Button type="primary" onClick={handleSave} disabled={editedIds.size === 0} loading={loading}>
                  保存修改 ({editedIds.size})
                </Button>
              </Space>
            </div>
            <Table
              columns={instrumentColumns}
              dataSource={tableData}
              rowKey="id"
              expandable={{
                expandedRowRender,
                expandedRowKeys: expandedRows,
                onExpandedRowKeysChange: setExpandedRows,
              }}
              pagination={{
                ...pagination,
                showSizeChanger: true, showQuickJumper: true,
                onChange: (page, pageSize) => setPagination({ ...pagination, current: page, pageSize }),
              }}
              loading={loading}
              rowClassName={(record) => editedIds.has(record.id) ? 'bg-blue-50' : ''}
              scroll={{ x: true }}
            />
          </TabPane>

          <TabPane tab="批量定价" key="batch">
            <div className="mb-4">
              <Space>
                <span>第一阶梯日均价统一设为：</span>
                <InputNumber
                  min={0} precision={2} value={batchBaseRate}
                  onChange={setBatchBaseRate}
                  formatter={(val) => `¥ ${val}`} parser={(val) => val.replace(/\¥\s?/g, '')}
                />
                <Button onClick={() => {
                  if (batchBaseRate > 0) {
                    setBatchItems(prev => prev.map(item => ({ ...item, base_daily_rate: batchBaseRate })))
                    message.success(`已批量填入 ¥${batchBaseRate}`)
                  }
                }}>批量填入</Button>
              </Space>
            </div>
            <Table
              columns={batchColumns}
              dataSource={batchItems}
              rowKey="id"
              pagination={{ pageSize: 50 }}
              loading={loading}
              scroll={{ x: true }}
            />
            <div className="mt-4 text-right">
              <Button type="primary" onClick={handleSaveBatch} loading={loading}>
                保存全部定价
              </Button>
            </div>
          </TabPane>

          <TabPane tab="定价策略查看" key="view">
            {merchantConfig ? (
              <div>
                <h3 className="text-base font-medium mb-3">当前商户定价策略</h3>
                {merchantConfig.tiers?.map((tier, i) => {
                  const prevDays = i > 0 ? merchantConfig.tiers[i - 1].days_max : 0
                  const daysDisplay = tier.days_max === -1
                    ? `${prevDays + 1}天以上`
                    : `1-${tier.days_max}天`
                  return (
                    <Card key={i} size="small" className="mb-2" title={tier.name}>
                      <p>租赁区间: {daysDisplay}</p>
                      {tier.discount_percent > 0 && <p>折扣: {tier.discount_percent}%</p>}
                    </Card>
                  )
                })}
                <Card size="small" title="押金" className="mt-2">
                  {merchantConfig.deposit_mode === 'ratio'
                    ? `第一阶梯日均价 × ${merchantConfig.deposit_ratio || 2}倍`
                    : `固定 ¥${merchantConfig.deposit_fixed || 0}`}
                </Card>
              </div>
            ) : (
              <div className="text-center py-12">
                <p className="text-gray-400 text-lg">商户尚未配置定价策略</p>
                <p className="text-gray-400 mt-2">请联系商户管理员进行配置</p>
              </div>
            )}
          </TabPane>
        </Tabs>
      </Card>

      <OverrideModal
        visible={overrideModal.visible}
        record={overrideModal.record}
        onClose={() => setOverrideModal({ visible: false, record: null })}
        onSave={handleOverrideSave}
      />
    </div>
  )
}
