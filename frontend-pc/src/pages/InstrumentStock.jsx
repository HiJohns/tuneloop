import { useState, useEffect } from 'react'
import { formatCents, yuanToCents } from '../utils/money'
import { Table, Tag, Button, Space, Spin, Modal, Form, Input, InputNumber, Select, Radio, Upload, Descriptions, Image, message, Popconfirm } from 'antd'
import { EyeOutlined, EditOutlined, WarningOutlined, RollbackOutlined, FileTextOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { inventoryApi, lossApi, api } from '../services/api'
import PermissionGate from '../components/PermissionGate'
import { usePermission } from '../hooks/usePermission'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange",
  "已熔断": "red",
  "rented": "green",
  "available": "blue",
  "maintenance": "orange",
  "lost": "red",
}

export default function InstrumentStock() {
  const [searchParams] = useSearchParams()
  const statusParam = searchParams.get('status')
  const navigate = useNavigate()
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1) // #2140：服务端分页
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [selectedRowKeys, setSelectedRowKeys] = useState([]) // #2043 批量操作
  const [priceModalVisible, setPriceModalVisible] = useState(false) // #2043 批量设价
  const [priceForm] = Form.useForm()

  // #2043: 管理员级可见性代理（删除权限=管理级）
  const { hasCusPerm } = usePermission()
  const isManagerLike = hasCusPerm('instrument:delete')

  const loadData = async () => {
    setLoading(true)
    try {
      // #2140：真分页——服务端 page/pageSize + 状态筛选服务端化；
      // 非管理级状态范围（隐藏 下架/已售出/丢失）转服务端 exclude_status
      const params = { page, pageSize }
      if (statusParam) params.stock_status = statusParam
      if (!isManagerLike) params.exclude_status = 'lost,archived,sold'
      const response = await inventoryApi.list(params)
      setAssets(response?.data?.list || [])
      setTotal(response?.data?.total || 0)
    } catch (error) {
      console.error('Failed to load inventory:', error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [page, pageSize, statusParam, isManagerLike])
  
  const columns = [
    {
      title: '图片',
      dataIndex: 'images',
      key: 'images',
      width: 80,
      render: (images) => {
        const imageList = Array.isArray(images) ? images : (images ? JSON.parse(images) : [])
        const src = imageList && imageList.length > 0 ? imageList[0] : '/images/default-instrument.jpg'
        return <img src={src} alt="" style={{ width: 50, height: 50, objectFit: 'cover', borderRadius: 4 }} />
      }
    },
    {
      title: '识别码',
      dataIndex: 'sn',
      key: 'sn',
      width: 160,
      render: (sn, record) => (
        <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{sn || record.id?.slice(0, 8) || '-'}</span>
      )
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
      width: 120,
    },
    {
      title: '级别',
      dataIndex: 'level_name',
      key: 'level_name',
      width: 80,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status, record) => {
        const getUnifiedStatus = (status) => {
          const statusMap = {
            "在租": { color: 'green', text: '在租' },
            "rented": { color: 'green', text: '在租' },
            "待租": { color: 'blue', text: '待租' },
            "available": { color: 'blue', text: '待租' },
            "维修中": { color: 'orange', text: '维修中' },
            "maintenance": { color: 'orange', text: '维修中' },
            "已熔断": { color: 'red', text: '已熔断' },
            "lost": { color: 'red', text: '已丢失' },
            "archived": { color: 'default', text: '已下架' },
            "sold": { color: 'default', text: '已售出' }
          }
          return statusMap[status] || { color: 'default', text: status }
        }
        
        const info = getUnifiedStatus(status)
        return <Tag color={info.color}>{info.text}</Tag>
      }
    },
    {
      title: '所属网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 150,
    },
    // #2043: 估值 = 敏感字段，仅管理员/商户管理员可见
    ...(isManagerLike ? [{
      title: '估值',
      dataIndex: 'base_daily_rate',
      key: 'base_daily_rate',
      width: 120,
      align: 'right',
      render: (value) => value ? `¥${value.toLocaleString()}` : '-'
    }] : []),
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" icon={<EyeOutlined />}
            onClick={(e) => { e.stopPropagation(); navigate(`/site/stock/${record.id}`) }}>详情</Button>
          {/* #2043: 编辑/丢失/恢复 需 instrument:update（无权限不渲染） */}
          <PermissionGate code="instrument:update">
            <Button type="link" size="small" icon={<EditOutlined />}
              onClick={(e) => { e.stopPropagation(); navigate(`/instruments/list/edit/${record.id}`) }}>编辑</Button>
          </PermissionGate>
          <PermissionGate code="instrument:update">
            {record.status === 'lost' ? (
              <Button type="link" size="small" icon={<RollbackOutlined />}
                onClick={(e) => { e.stopPropagation(); openRestoreModal(record) }}>恢复</Button>
            ) : (
              <Button type="link" size="small" danger icon={<WarningOutlined />}
                onClick={(e) => { e.stopPropagation(); openLostModal(record) }}>丢失</Button>
            )}
          </PermissionGate>
          <PermissionGate code="instrument:delete">
            <Popconfirm title="确认删除该乐器？" okButtonProps={{ danger: true }}
              onConfirm={(e) => { e?.stopPropagation?.(); handleDeleteInstrument(record.id) }}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}
                onClick={(e) => e.stopPropagation()}>删除</Button>
            </Popconfirm>
          </PermissionGate>
        </Space>
      )
    }
  ]

  // #2043: 行级删除（需 instrument:delete；无权限按钮不渲染）
  const handleDeleteInstrument = async (id) => {
    try {
      const res = await api.delete(`/instruments/${id}`)
      if (res.code === 20000) {
        message.success('已删除')
        loadData()
      } else {
        message.error(res.message || '删除失败')
      }
    } catch (e) {
      message.error('删除失败')
    }
  }

  // #2043: 批量设价（服务端 RequireRole(ADMIN/OWNER/site_admin) 兜底）
  const handleBatchPrice = async () => {
    try {
      const values = await priceForm.validateFields()
      const res = await api.put('/instruments/batch/price', {
        ids: selectedRowKeys,
        price_type: values.price_type,
        amount: values.amount,
        operator: values.operator,
      })
      if (res.code === 20000) {
        message.success(`已修改 ${selectedRowKeys.length} 个乐器价格`)
        setPriceModalVisible(false)
        priceForm.resetFields()
        setSelectedRowKeys([])
        loadData()
      } else {
        message.error(res.message || '批量修改价格失败')
      }
    } catch (e) {
      if (!e?.errorFields) message.error(e?.message || '批量修改价格失败')
    }
  }

  // #2043/#2101: 导出 CSV（档案管理页已并入本页；按当前状态过滤直接导出）
  const handleExport = async () => {
    try {
      const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'
      const params = new URLSearchParams()
      if (statusParam) params.set('status', statusParam)
      const resp = await fetch(`${API_BASE_URL}/instruments/export?${params.toString()}`)
      if (!resp.ok) throw new Error('导出失败')
      const blob = await resp.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `instruments_export_${new Date().toISOString().split('T')[0]}.csv`
      a.click()
      window.URL.revokeObjectURL(url)
      message.success('导出成功')
    } catch (e) {
      message.error(e.message || '导出失败')
    }
  }

  // #2043: 批量删除（需 instrument:delete；工具栏按钮门控）
  const batchDelete = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的乐器')
      return
    }
    Modal.confirm({
      title: '批量删除确认',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个乐器吗？此操作不可恢复！`,
      okText: '确定删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          const result = await api.delete('/instruments/batch', { ids: selectedRowKeys })
          if (result.code === 20000) {
            message.success(`已成功删除 ${(result.deleted || []).length} 个乐器`)
            setSelectedRowKeys([])
            loadData()
          } else {
            message.error(result.message || '批量删除失败')
          }
        } catch (e) {
          message.error('批量删除失败')
        }
      },
    })
  }

  // ===== #1948 丢失登记 / 恢复 / 台账 =====
  const [view, setView] = useState('stock') // stock | loss
  const [lossRows, setLossRows] = useState([])
  const [lossLoading, setLossLoading] = useState(false)
  const [lossDetail, setLossDetail] = useState(null)
  const [lostTarget, setLostTarget] = useState(null)
  const [restoreTarget, setRestoreTarget] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [lossForm] = Form.useForm()
  const [restoreForm] = Form.useForm()
  const [restoreFiles, setRestoreFiles] = useState([])

  const openLostModal = (record) => {
    setLostTarget(record)
    lossForm.setFieldsValue({ responsible_party: 'user', user_ratio: 100 })
    setRestoreFiles([])
  }
  const openRestoreModal = (record) => {
    setRestoreTarget(record)
    restoreForm.setFieldsValue({ damaged: false })
    setRestoreFiles([])
  }

  const uploadOne = async (file) => {
    const fd = new FormData()
    fd.append('file', file)
    const resp = await api.post('/upload', fd, { headers: {} })
    if (resp.code === 20000) return resp.data?.url || resp.data?.file_key
    throw new Error(resp.message || 'upload failed')
  }

  const submitLost = async () => {
    const v = await lossForm.validateFields()
    setSubmitting(true)
    try {
      const resp = await lossApi.register(lostTarget.id, {
        description: v.description,
        responsible_party: v.responsible_party,
        user_ratio: v.user_ratio ?? 0,
        compensation_cents: yuanToCents(v.compensation_yuan || 0),
        ...(v.user_burden_yuan != null ? { user_burden_cents: yuanToCents(v.user_burden_yuan) } : {}),
      })
      if (resp.code === 20000) {
        message.success('丢失登记完成')
        setLostTarget(null)
        loadData()
      } else message.error(resp.message || '登记失败')
    } catch (e) { message.error(e.message || '登记失败') }
    setSubmitting(false)
  }

  const submitRestore = async () => {
    const v = await restoreForm.validateFields()
    setSubmitting(true)
    try {
      const photos = []
      for (const f of restoreFiles) photos.push(await uploadOne(f))
      const resp = await lossApi.restore(restoreTarget.id, {
        damaged: !!v.damaged, description: v.description || '', photos,
      })
      if (resp.code === 20000) {
        const d = resp.data || {}
        message.success(d.reversal === 'refunded'
          ? `恢复成功，冲正退款 ¥${formatCents(d.refund_cents || 0)}`
          : d.reversal === 'held_pending_assessment' ? '恢复成功（有损坏，赔偿暂扣待定损）' : '恢复成功')
        setRestoreTarget(null)
        loadData()
      } else message.error(resp.message || '恢复失败')
    } catch (e) { message.error(e.message || '恢复失败') }
    setSubmitting(false)
  }

  const loadLossRecords = async () => {
    setLossLoading(true)
    try {
      const resp = await lossApi.list()
      if (resp.code === 20000) setLossRows(resp.data?.list || [])
    } finally { setLossLoading(false) }
  }

  const switchView = (key) => {
    setView(key)
    if (key === 'loss') loadLossRecords()
  }

  const lossColumns = [
    { title: '乐器', dataIndex: 'instrument_id', render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v?.slice(0, 8)}…</span> },
    { title: '订单', dataIndex: 'order_id', render: v => v ? <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v.slice(0, 8)}…</span> : '-' },
    { title: '责任方', dataIndex: 'responsible_party', render: v => ({ user: '用户', logistics: '物流公司', platform: '平台', site: '网点' }[v] || v) },
    { title: '责任比例', dataIndex: 'user_ratio', render: v => `${v}%` },
    { title: '赔偿', dataIndex: 'compensation_cents', align: 'right', render: v => v ? `¥${formatCents(v)}` : '-' },
    { title: '用户承担', dataIndex: 'user_burden_cents', align: 'right', render: v => `¥${formatCents(v || 0)}` },
    {
      title: '状态', key: 'state',
      render: (_, r) => r.restored_at
        ? <Tag color="green">已恢复{r.restored_damaged ? '（有损坏）' : ''}</Tag>
        : <Tag color="red">丢失中</Tag>,
    },
    { title: '登记时间', dataIndex: 'created_at', render: v => v ? new Date(v).toLocaleString('zh-CN') : '-' },
    { title: '操作', key: 'op', render: (_, r) => <Button type="link" size="small" onClick={() => setLossDetail(r)}>详情</Button> },
  ]

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="p-6">
      {/* #2043: 批量设价模态 */}
      <Modal
        title="批量修改价格"
        open={priceModalVisible}
        onOk={handleBatchPrice}
        onCancel={() => setPriceModalVisible(false)}
        width={500}
      >
        <Form form={priceForm} layout="vertical">
          <Form.Item name="price_type" label="价格类型" rules={[{ required: true, message: '请选择价格类型' }]}>
            <Select placeholder="请选择价格类型">
              <Select.Option value="daily_rate">日租金</Select.Option>
              <Select.Option value="weekly_rate">周租金</Select.Option>
              <Select.Option value="monthly_rate">月租金</Select.Option>
              <Select.Option value="deposit">押金</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="operator" label="操作" rules={[{ required: true, message: '请选择操作' }]}>
            <Select placeholder="请选择操作">
              <Select.Option value="set">设置为</Select.Option>
              <Select.Option value="increase">增加</Select.Option>
              <Select.Option value="decrease">减少</Select.Option>
              <Select.Option value="multiply">乘以</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="amount" label="金额（元）" rules={[{ required: true, message: '请输入金额' }]}>
            <InputNumber style={{ width: '100%' }} min={0} />
          </Form.Item>
        </Form>
      </Modal>
      <Space style={{ marginBottom: 8 }}>
        <h2 className="text-xl font-bold" style={{ margin: 0 }}>{view === 'stock' ? '库存监控' : '丢失台账'}</h2>
        <Button icon={<WarningOutlined />} onClick={() => switchView(view === 'stock' ? 'loss' : 'stock')}>
          {view === 'stock' ? '丢失台账' : '返回库存'}
        </Button>
        {/* #2101/I-00：档案管理入口并入本页——新增乐器（instrument:create） */}
        <PermissionGate code="instrument:create">
          {view === 'stock' && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/instruments/list/add')}>新增乐器</Button>
          )}
        </PermissionGate>
        {/* #2043: 导出 CSV（需 instrument:read） */}
        <PermissionGate code="instrument:read">
          {view === 'stock' && (
            <Button onClick={handleExport}>导出 CSV</Button>
          )}
        </PermissionGate>
        {/* #2043: 批量导入（需 instrument:create） */}
        <PermissionGate code="instrument:create">
          {view === 'stock' && (
            <Button onClick={() => navigate('/instruments/batch-import')}>批量导入</Button>
          )}
        </PermissionGate>
        {/* #2043: 批量设价（仅选中后出现；服务端 RequireRole 兜底） */}
        <PermissionGate code="instrument:update">
          {view === 'stock' && selectedRowKeys.length > 0 && (
            <Button onClick={() => { priceForm.resetFields(); setPriceModalVisible(true) }}>批量设价（{selectedRowKeys.length}）</Button>
          )}
        </PermissionGate>
        {/* #2043: 批量删除（仅选中后出现；需 instrument:delete） */}
        <PermissionGate code="instrument:delete">
          {view === 'stock' && selectedRowKeys.length > 0 && (
            <Button danger icon={<DeleteOutlined />} onClick={batchDelete}>
              删除所选（{selectedRowKeys.length}）
            </Button>
          )}
        </PermissionGate>
      </Space>
      {view === 'stock' && (
        <div className="text-xs text-gray-500 mb-3">
          按状态查看乐器（待租 / 在租 / 维修中 / 已丢失）；导出/定价/删除等管理功能已并入本页工具栏。
        </div>
      )}
      {statusParam && (
        <div className="mb-4 p-3 bg-blue-50 rounded">
          <Space>
            <span>当前筛选: 状态 = {statusParam}</span>
            <Button type="link" size="small" onClick={() => navigate('/site/stock', { replace: true })}>清除筛选</Button>
          </Space>
        </div>
      )}
      {view === 'stock' ? (
        <Table
          columns={columns}
          dataSource={assets || []}
          rowKey="id"
          rowSelection={{ selectedRowKeys, onChange: setSelectedRowKeys }}
          pagination={{
            current: page, pageSize, total, showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
          }}
          onRow={(record) => ({
            onClick: () => navigate(`/site/stock/${record.id}`),
            style: { cursor: 'pointer' }
          })}
        />
      ) : (
        <Table columns={lossColumns} dataSource={lossRows} rowKey="id" loading={lossLoading} />
      )}

      {/* 丢失登记（LS-01/02） */}
      <Modal title={`丢失登记 — ${lostTarget?.name || ''}`} open={!!lostTarget}
        onCancel={() => setLostTarget(null)} onOk={submitLost} confirmLoading={submitting} okText="提交登记">
        <Form form={lossForm} layout="vertical">
          <Form.Item name="description" label="丢失描述" rules={[{ required: true, message: '请填写丢失描述' }]}>
            <Input.TextArea rows={3} placeholder="描述丢失经过" />
          </Form.Item>
          <Form.Item name="responsible_party" label="责任方" rules={[{ required: true }]}>
            <Select options={[
              { value: 'user', label: '用户' }, { value: 'logistics', label: '物流公司' },
              { value: 'platform', label: '平台' }, { value: 'site', label: '网点' },
            ]} onChange={(v) => lossForm.setFieldValue('user_ratio', v === 'user' ? 100 : 0)} />
          </Form.Item>
          <Form.Item name="user_ratio" label="用户责任比例（%）" rules={[{ required: true }]}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="compensation_yuan" label="赔偿金额（元，按乐器价值填写）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="user_burden_yuan" label="用户承担金额（元，留空=按比例计算）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 恢复（LS-05） */}
      <Modal title={`乐器恢复 — ${restoreTarget?.name || ''}`} open={!!restoreTarget}
        onCancel={() => setRestoreTarget(null)} onOk={submitRestore} confirmLoading={submitting} okText="确认恢复">
        <Form form={restoreForm} layout="vertical">
          <Form.Item name="damaged" label="是否有损坏">
            <Radio.Group>
              <Radio value={false}>无损坏</Radio>
              <Radio value={true}>有损坏（赔偿暂扣，待定损）</Radio>
            </Radio.Group>
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="找回情况说明（可选）" />
          </Form.Item>
          <Form.Item label="照片（可选，≤6）">
            <Upload
              listType="picture-card"
              maxCount={6}
              beforeUpload={() => false}
              fileList={restoreFiles.map((f, i) => ({ uid: String(i), name: f?.name || `photo${i}`, status: 'done' }))}
              onChange={({ fileList }) => setRestoreFiles(fileList.map(f => f.originFileObj || f).filter(Boolean))}
            >
              {restoreFiles.length < 6 && <span>＋</span>}
            </Upload>
          </Form.Item>
        </Form>
      </Modal>

      {/* 台账详情 */}
      <Modal title="丢失记录详情" open={!!lossDetail} onCancel={() => setLossDetail(null)} footer={null} width={640}>
        {lossDetail && (
          <Descriptions size="small" column={2} bordered>
            <Descriptions.Item label="乐器" span={2}><span style={{ fontFamily: 'monospace' }}>{lossDetail.instrument_id}</span></Descriptions.Item>
            <Descriptions.Item label="关联订单" span={2}><span style={{ fontFamily: 'monospace' }}>{lossDetail.order_id || '-'}</span></Descriptions.Item>
            <Descriptions.Item label="责任方">{({ user: '用户', logistics: '物流公司', platform: '平台', site: '网点' })[lossDetail.responsible_party] || lossDetail.responsible_party}</Descriptions.Item>
            <Descriptions.Item label="责任比例">{lossDetail.user_ratio}%</Descriptions.Item>
            <Descriptions.Item label="赔偿金额">¥{formatCents(lossDetail.compensation_cents || 0)}</Descriptions.Item>
            <Descriptions.Item label="用户承担">¥{formatCents(lossDetail.user_burden_cents || 0)}</Descriptions.Item>
            <Descriptions.Item label="找回/结算" span={2}>
              {lossDetail.restored_at
                ? `已恢复上架${lossDetail.restored_damaged ? '（有损坏）' : '（无损坏）'}`
                : lossDetail.settled_at ? '已结算（丢失中）' : '未结算（纯库存）'}
            </Descriptions.Item>
            <Descriptions.Item label="描述" span={2}>{lossDetail.description || '-'}</Descriptions.Item>
            <Descriptions.Item label="登记人/时间" span={2}>
              {lossDetail.created_by ? lossDetail.created_by.slice(0, 8) : '-'} · {lossDetail.created_at ? new Date(lossDetail.created_at).toLocaleString('zh-CN') : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="照片" span={2}>
              {(() => {
                let ps = []
                try { ps = typeof lossDetail.photos === 'string' ? JSON.parse(lossDetail.photos || '[]') : (lossDetail.photos || []) } catch { ps = [] }
                return ps.length ? <Image.PreviewGroup>{ps.map((p, i) => <Image key={i} src={p} width={72} height={72} style={{ objectFit: 'cover', borderRadius: 6 }} />)}</Image.PreviewGroup> : '-'
              })()}
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </div>
  )
}
