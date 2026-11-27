import { useState, useEffect } from 'react'
import { formatCents, yuanToCents } from '../utils/money'
import { Table, Tag, Button, Space, Modal, Form, Input, InputNumber, Select, Radio, Upload, Descriptions, Image, message, Popconfirm, Alert } from 'antd'
import { EditOutlined, WarningOutlined, RollbackOutlined, FileTextOutlined, DeleteOutlined } from '@ant-design/icons'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { inventoryApi, lossApi, api, pricingApi, instrumentsApi, fetchBlob } from '../services/api'
import FormConflictAlert from '../components/FormConflictAlert'
import { conflictText, isBusinessCorrectableConflict } from '../utils/formConflict'
import PermissionGate from '../components/PermissionGate'
import { usePermission } from '../hooks/usePermission'
import {
  BATCH_PRICE_OPERATOR_OPTIONS,
  BATCH_PRICE_TYPE_OPTIONS,
  batchPricingFeedback,
  buildBatchPricingPayload,
} from '../utils/batchPricing'
import {
  INSTRUMENT_IMAGE_PLACEHOLDER,
  formatDailyRate,
  formatDepositFull,
  formatDepositReduced,
  formatOverdueDailyFee,
  instrumentImageSrc,
} from '../utils/instrumentDisplay'
import {
  applyFilterPatch,
  buildStockExportParams,
  buildStockFilterQuery,
  buildStockListParams,
  readStockFilters,
  stockStatusOptions,
} from '../utils/instrumentStockFilters'

export default function InstrumentStock() {
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1) // #2140：服务端分页
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [selectedRowKeys, setSelectedRowKeys] = useState([]) // #2043 批量操作
  const [priceModalVisible, setPriceModalVisible] = useState(false) // #2043 批量设价
  // #2488（沿用 #2480 D8）：库存操作里的业务可纠正冲突（40005 状态不允许 / 40900 冲突）
  // 用表单内持久提示，不当作瞬时 toast —— 用户改一个输入/重开表单即可重试。
  const [priceConflict, setPriceConflict] = useState('')
  const [lostConflict, setLostConflict] = useState('')
  const [restoreConflict, setRestoreConflict] = useState('')
  const [priceForm] = Form.useForm()

  // #2577: URL 即筛选唯一来源（D1）；page/pageSize 仍为本地 state（D6）。
  const filters = readStockFilters(searchParams)
  const filterKey = searchParams.toString()
  // 识别码为 `Input.Search`（回车/点搜索/清空触发，D3）——输入过程不取数。
  const [snInput, setSnInput] = useState(filters.sn)
  const [filterOptions, setFilterOptions] = useState({ categories: [], statuses: [] })

  // #2043: 管理员级可见性代理（删除权限=管理级）
  const { hasCusPerm } = usePermission()
  const isManagerLike = hasCusPerm('instrument:delete')

  const loadData = async () => {
    setLoading(true)
    try {
      // #2140/#2577：真分页 + 服务端筛选；参数与可见性口径统一由
      // utils/instrumentStockFilters 生成（sn / category_id / stock_status /
      // exclude_status），页面不再各自拼装。
      const params = buildStockListParams({ filters, page, pageSize, isManagerLike })
      const response = await inventoryApi.list(params)
      setAssets(response?.data?.list || [])
      setTotal(response?.data?.total || 0)
    } catch (error) {
      console.error('Failed to load inventory:', error)
    } finally {
      setLoading(false)
    }
  }

  // #2577: 只有 URL 变化才触发取数（控件回调只写 URL，不重复请求）。
  useEffect(() => {
    loadData()
  }, [filterKey, page, pageSize, isManagerLike])

  // URL 外部变化（深链 / 清空 / 浏览器前进后退）回填识别码输入框。
  useEffect(() => {
    setSnInput(filters.sn)
  }, [filters.sn])

  // #2577: 分类/状态选项与乐器列表同源（filter-options）。
  useEffect(() => {
    instrumentsApi.getFilterOptions().then((res) => {
      if (res?.code === 20000) setFilterOptions(res.data || {})
    }).catch((error) => {
      // 下拉不可用时列表仍可用（不阻塞主流程，见 Input Truth 损坏输入约定）。
      console.error('Failed to load filter options:', error)
    })
  }, [])

  // 筛选变化的唯一入口：合并 patch → 回第 1 页 → 清空行选择 → 写回 URL。
  const patchFilters = (patch) => {
    const next = applyFilterPatch({ filters, page, selectedRowKeys }, patch)
    setPage(next.page)
    setSelectedRowKeys(next.selectedRowKeys)
    setSearchParams(buildStockFilterQuery(next.filters), { replace: true })
  }

  const clearFilters = () => patchFilters({ sn: '', categoryId: '', status: '' })

  const statusOptions = stockStatusOptions({
    statuses: filterOptions.statuses,
    isManagerLike,
    current: filters.status,
  })
  
  const columns = [
    {
      title: '图片',
      dataIndex: 'thumbnail',
      key: 'thumbnail',
      width: 80,
      render: (_, record) => (
        <img
          src={instrumentImageSrc(record)}
          alt=""
          style={{ width: 50, height: 50, objectFit: 'cover', borderRadius: 4 }}
          onError={(e) => { e.currentTarget.src = INSTRUMENT_IMAGE_PLACEHOLDER }}
        />
      )
    },
    {
      title: '识别码',
      dataIndex: 'sn',
      key: 'sn',
      width: 160,
      // #2537 D6: 识别码即详情入口（不再有独立「详情」按钮）。
      render: (sn, record) => (
        <Button
          type="link"
          size="small"
          style={{ padding: 0, fontFamily: 'monospace', fontSize: 12 }}
          onClick={(e) => {
            e.stopPropagation()
            navigate(`/site/stock/${record.id}`)
          }}
        >
          {sn || record.id?.slice(0, 8) || '-'}
        </Button>
      )
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
      width: 120,
    },
    {
      title: '所属网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 150,
    },
    // #2043: 价格字段 = 敏感字段，仅管理员/商户管理员可见
    // #2528: 「估值」实为标准日租金（分），改名 + 分→元换算；新增押金列（与详情同源）
    ...(isManagerLike ? [{
      title: '标准日租金',
      dataIndex: 'resolved_daily_rate',
      key: 'resolved_daily_rate',
      width: 120,
      align: 'right',
      render: (_, record) => formatDailyRate(record)
    }, {
      // #2537: 逾期租金列（列表新字段，与 pricing-v2 同链）。
      title: '逾期租金',
      dataIndex: 'overdue_daily_fee',
      key: 'overdue_daily_fee',
      width: 120,
      align: 'right',
      render: (_, record) => formatOverdueDailyFee(record)
    }, {
      title: '全额押金',
      dataIndex: 'deposit',
      key: 'deposit',
      width: 120,
      align: 'right',
      render: (_, record) => formatDepositFull(record)
    }, {
      title: '差额押金',
      dataIndex: 'deposit_reduced',
      key: 'deposit_reduced',
      width: 120,
      align: 'right',
      render: (_, record) => formatDepositReduced(record)
    }] : []),
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space>
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

  // #2043/#2514: 批量设价（服务端 RequireRole(ADMIN/OWNER/site_admin) 兜底）
  // 路径与 payload 统一由 utils/batchPricing 生成，避免再次打到不存在的
  // /instruments/batch/price 或发送后端不认识的 {ids, price_type, ...}。
  const handleBatchPrice = async () => {
    setPriceConflict('') // #2488 D8：提交开始即清空陈旧冲突提示
    try {
      const values = await priceForm.validateFields()
      const built = buildBatchPricingPayload(selectedRowKeys, values)
      if (built.error) {
        message.error(built.error)
        return
      }
      const res = await pricingApi.batchSetPricing(built.payload)
      if (res.code !== 20000) {
        // #2488 CODE_FIRST：先判 code，再按 rubric 决定常驻提示还是瞬时 toast。
        if (isBusinessCorrectableConflict(res)) {
          setPriceConflict(conflictText(res, '批量修改价格失败，请调整后重试'))
          return
        }
        message.error(res.message || '批量修改价格失败')
        return
      }
      const feedback = batchPricingFeedback(res, selectedRowKeys.length)
      message[feedback.level](feedback.text)
      setPriceConflict('')
      setPriceModalVisible(false)
      priceForm.resetFields()
      setSelectedRowKeys([])
      loadData()
    } catch (e) {
      if (e?.errorFields) return
      // #2488（D2/Rev.5 Bug D）：传输层 ApiError 也走同一 rubric 判定。
      if (isBusinessCorrectableConflict(e)) {
        setPriceConflict(conflictText(e, '批量修改价格失败，请调整后重试'))
        return
      }
      message.error(e?.message || '批量修改价格失败')
    }
  }

  // #2578: 导出 Excel 跟随当前筛选（识别码/分类/状态/可见性），与列表同一套参数口径。
  // D4：统一走 fetchBlob（带 Bearer + 空参剔除），产物为 xlsx，命名与乐器列表一致。
  const handleExport = async () => {
    try {
      const blob = await fetchBlob('/instruments/export', {
        params: buildStockExportParams({ filters, isManagerLike }),
      })
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `instruments_export_${new Date().toISOString().split('T')[0]}.xlsx`
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
    setLostConflict('') // #2488 D8：打开表单即清空陈旧冲突提示
    setLostTarget(record)
    lossForm.setFieldsValue({ responsible_party: 'user', user_ratio: 100 })
    setRestoreFiles([])
  }
  const openRestoreModal = (record) => {
    setRestoreConflict('')
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
        setLostConflict('')
        setLostTarget(null)
        loadData()
      } else if (isBusinessCorrectableConflict(resp)) {
        setLostConflict(conflictText(resp, '登记失败，请调整后重试'))
      } else message.error(resp.message || '登记失败')
    } catch (e) {
      if (isBusinessCorrectableConflict(e)) {
        setLostConflict(conflictText(e, '登记失败，请调整后重试'))
      } else message.error(e.message || '登记失败')
    }
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
        setRestoreConflict('')
        const d = resp.data || {}
        message.success(d.reversal === 'refunded'
          ? `恢复成功，冲正退款 ¥${formatCents(d.refund_cents || 0)}`
          : d.reversal === 'held_pending_assessment' ? '恢复成功（有损坏，赔偿暂扣待定损）' : '恢复成功')
        setRestoreTarget(null)
        loadData()
      } else if (isBusinessCorrectableConflict(resp)) {
        setRestoreConflict(conflictText(resp, '恢复失败，请调整后重试'))
      } else message.error(resp.message || '恢复失败')
    } catch (e) {
      if (isBusinessCorrectableConflict(e)) {
        setRestoreConflict(conflictText(e, '恢复失败，请调整后重试'))
      } else message.error(e.message || '恢复失败')
    }
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

  // #2577 D7: 首屏 loading 改为 `Table loading`——整页 early return 会在每次筛选时
  // 卸载工具栏，导致输入框失焦、控件闪烁。
  return (
    <div className="p-6">
      {/* #2043: 批量设价模态 */}
      <Modal
        title="批量修改价格"
        open={priceModalVisible}
        onOk={handleBatchPrice}
        onCancel={() => { setPriceConflict(''); setPriceModalVisible(false) }}
        width={500}
      >
        <FormConflictAlert text={priceConflict} testId="stock-price-conflict" />
        <Form form={priceForm} layout="vertical">
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="当前仅支持按日租金批量设置为固定金额"
          />
          <Form.Item name="price_type" label="价格类型" rules={[{ required: true, message: '请选择价格类型' }]}>
            <Select placeholder="请选择价格类型">
              {BATCH_PRICE_TYPE_OPTIONS.map((option) => (
                <Select.Option key={option.value} value={option.value}>{option.label}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="operator" label="操作" rules={[{ required: true, message: '请选择操作' }]}>
            <Select placeholder="请选择操作">
              {BATCH_PRICE_OPERATOR_OPTIONS.map((option) => (
                <Select.Option key={option.value} value={option.value}>{option.label}</Select.Option>
              ))}
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
        {/* #2043/#2578: 导出 Excel（跟随当前筛选；需 instrument:read） */}
        <PermissionGate code="instrument:read">
          {view === 'stock' && (
            <Button onClick={handleExport}>导出 Excel</Button>
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
            <Button onClick={() => { setPriceConflict(''); priceForm.resetFields(); setPriceModalVisible(true) }}>批量设价（{selectedRowKeys.length}）</Button>
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
          按状态查看乐器（待租 / 在租 / 维修中 / 已丢失）；乐器档案的新增与编辑请用「乐器列表」。
        </div>
      )}
      {view === 'stock' && (
        <Space style={{ marginBottom: 12, flexWrap: 'wrap' }} wrap>
          <Input.Search
            placeholder="搜索识别码..."
            value={snInput}
            allowClear
            style={{ width: 250 }}
            onChange={(e) => setSnInput(e.target.value)}
            onSearch={(value) => patchFilters({ sn: (value || '').trim() })}
          />
          <Select
            placeholder="选择分类"
            style={{ width: 180 }}
            allowClear
            value={filters.categoryId || undefined}
            onChange={(value) => patchFilters({ categoryId: value || '' })}
            options={(filterOptions.categories || []).map((cat) => ({
              value: cat.category_id,
              label: cat.category_name,
            }))}
          />
          <Select
            placeholder="选择状态"
            style={{ width: 140 }}
            allowClear
            value={filters.status || undefined}
            onChange={(value) => patchFilters({ status: value || '' })}
            options={statusOptions}
          />
          <Button onClick={clearFilters}>清空筛选</Button>
        </Space>
      )}
      {view === 'stock' ? (
        <Table
          columns={columns}
          dataSource={assets || []}
          rowKey="id"
          loading={loading}
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
        onCancel={() => { setLostConflict(''); setLostTarget(null) }} onOk={submitLost} confirmLoading={submitting} okText="提交登记">
        <FormConflictAlert text={lostConflict} testId="stock-lost-conflict" />
        <Form form={lossForm} layout="vertical" onValuesChange={() => { if (lostConflict) setLostConflict('') }}>
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
        onCancel={() => { setRestoreConflict(''); setRestoreTarget(null) }} onOk={submitRestore} confirmLoading={submitting} okText="确认恢复">
        <FormConflictAlert text={restoreConflict} testId="stock-restore-conflict" />
        <Form form={restoreForm} layout="vertical" onValuesChange={() => { if (restoreConflict) setRestoreConflict('') }}>
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
