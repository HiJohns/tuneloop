import { useState, useEffect } from 'react'
import { formatCents } from '../../../utils/money'
import { Card, Table, Tag, Spin, Select, Tabs, Modal, Descriptions, Image, Rate, Empty } from 'antd'
import { api } from '../../../services/api'
import { formatBeijingDate, formatBeijingDateTimeShort } from '../../../utils/date'

const statusLabels = {
  pending_assessment: '待估价', transit_processing: '中转处理中',
  pending_ship: '待发送', shipping: '发送中', transit_in: '转入中',
  pending_payment: '待付款', repairing: '维修中', return_pending: '待发回',
  transit_out: '转出中', returned: '已发回',
  closed: '已关闭', appealing: '申诉中',
}
const statusColors = {
  pending_assessment: 'purple', transit_processing: 'gold',
  closed: 'default', appealing: 'red',
  pending_ship: 'orange', shipping: 'blue', transit_in: 'cyan',
  pending_payment: 'cyan', repairing: 'geekblue', return_pending: 'lime',
  transit_out: 'blue', returned: 'green',
}

// #1952 阶段4：维修服务（type='service'）状态（RS-API 契约）
// RS-API-4：时间线类型 → 展示文案（与后端 record_type 对应）
const svcTimelineLabels = {
  created: '创建维修单', technician_selected: '选择维修师', quoted: '师傅报价',
  quote_accepted: '接受报价', paid: '支付成功', shipped: '乐器寄出',
  adjust_requested: '师傅发起加价', adjust_accepted: '同意加价',
  adjust_paid: '补差价到账', adjust_declined: '拒绝加价', leg_fee: '分段物流费登记',
  repair_completed: '完成修理', settled: '发回结算', reviewed: '提交评价',
  shortfall_paid: '补缴到账',
}

const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算',
}
const svcStatusColors = {
  pending_quote: 'orange', pending_payment: 'cyan', paid: 'blue',
  shipping: 'geekblue', repairing: 'purple', adjust_pending: 'gold',
  done_repair: 'lime', closed: 'green',
}
const yuan = (cents) => (cents == null ? '-' : `¥${formatCents(cents)}`)

export default function MerchantRepairList() {
  return (
    <Card title="维修管理">
      <Tabs
        defaultActiveKey="warranty"
        items={[
          { key: 'warranty', label: '乐器报修', children: <WarrantyList /> },
          { key: 'service', label: '维修服务', children: <ServiceList /> },
        ]}
      />
    </Card>
  )
}

function WarrantyList() {
  const [requests, setRequests] = useState([])
  const [loading, setLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState('')

  useEffect(() => {
    const params = statusFilter ? `?status=${statusFilter}` : ''
    api.get(`/merchant/repair-requests${params}`).then(r => {
      if (r.code === 20000) setRequests(r.data?.list || [])
    }).finally(() => setLoading(false))
  }, [statusFilter])

  const columns = [
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: v => v ? formatBeijingDate(v) : '-' },
    { title: '乐器', dataIndex: 'user_instrument_id', key: 'instrument', render: (_, r) => r.sn || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: s => <Tag color={statusColors[s]}>{statusLabels[s] || s}</Tag> },
    { title: '网点', dataIndex: 'site_id', key: 'site', render: (_, r) => r.site_name || '-' },
    { title: '报价', dataIndex: 'quote_amount', key: 'quote', render: v => v ? `¥${formatCents(v)}` : '-' },
  ]

  return (
    <>
      <Select value={statusFilter} onChange={setStatusFilter} allowClear placeholder="全部状态" style={{ width: 140, marginBottom: 12 }}>
        {Object.entries(statusLabels).map(([k, v]) => <Select.Option key={k} value={k}>{v}</Select.Option>)}
      </Select>
      {loading ? <Spin /> : <Table rowKey="id" dataSource={requests} columns={columns} />}
    </>
  )
}

// #1952：维修服务（type='service'）列表 + 详情（报价/加价/分段物流/结算/评价）
function ServiceList() {
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState('')
  const [detail, setDetail] = useState(null) // {repair, logistics_fees, review?, site?}
  const [detailLoading, setDetailLoading] = useState(false)

  useEffect(() => {
    setLoading(true)
    // RS-API-2：scope=site（员工上下文 JWT oid，商户/平台管理员回退 tid）
    const params = statusFilter ? `?scope=site&status=${statusFilter}` : '?scope=site'
    api.get(`/repair-services${params}`).then(r => {
      if (r.code === 20000) setList(r.data?.list || [])
    }).finally(() => setLoading(false))
  }, [statusFilter])

  const openDetail = (id) => {
    setDetailLoading(true)
    // RS-API-3：详情含 site（寄件地址）/ logistics_fees（分段物流）/ review（评价）
    api.get(`/user/repair-services/${id}`).then(r => {
      if (r.code === 20000) setDetail(r.data || {})
    }).finally(() => setDetailLoading(false))
  }

  const columns = [
    { title: '维修编码', dataIndex: 'repair_code', key: 'repair_code', render: v => v || '-' },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true, render: v => v || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: s => <Tag color={svcStatusColors[s]}>{svcStatusLabels[s] || s}</Tag> },
    { title: '修理费', dataIndex: 'quote_repair_cents', key: 'quote_repair', render: v => yuan(v) },
    {
      title: '加价后', dataIndex: 'adjusted_quote_cents', key: 'adjusted',
      render: v => (v == null ? '-' : yuan(v)),
    },
    { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', render: v => v ? formatBeijingDateTimeShort(v) : '-' },
    { title: '操作', key: 'action', render: (_, r) => <a onClick={() => openDetail(r.id)}>详情</a> },
  ]

  // 详情：评价展示（评分/留言/照片预览）—— #1952 验收项
  const review = detail?.review
  const fees = detail?.logistics_fees || []
  const rr = detail?.repair || {}

  return (
    <>
      <Select value={statusFilter} onChange={setStatusFilter} allowClear placeholder="全部状态" style={{ width: 140, marginBottom: 12 }}>
        {Object.entries(svcStatusLabels).map(([k, v]) => <Select.Option key={k} value={k}>{v}</Select.Option>)}
      </Select>
      {loading ? <Spin /> : <Table rowKey="id" dataSource={list} columns={columns} />}
      <Modal
        title={`维修服务详情${rr.repair_code ? ` · ${rr.repair_code}` : ''}`}
        open={!!detail}
        onCancel={() => setDetail(null)}
        footer={null}
        width={680}
      >
        {detailLoading ? <Spin /> : (
          <>
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="状态" span={2}>
                <Tag color={svcStatusColors[rr.status]}>{svcStatusLabels[rr.status] || rr.status}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="描述" span={2}>{rr.description || '-'}</Descriptions.Item>
              <Descriptions.Item label="报价修理费">{yuan(rr.quote_repair_cents)}</Descriptions.Item>
              <Descriptions.Item label="物流费预估">{yuan(rr.quote_logistics_cents)}</Descriptions.Item>
              <Descriptions.Item label="加价后修理费">{rr.adjusted_quote_cents == null ? '-' : yuan(rr.adjusted_quote_cents)}</Descriptions.Item>
              <Descriptions.Item label="到此为止修理费">{rr.incurred_repair_cents == null ? '-' : yuan(rr.incurred_repair_cents)}</Descriptions.Item>
              <Descriptions.Item label="寄出物流" span={2}>
                {rr.tracking_number ? `${rr.tracking_company || ''} ${rr.tracking_number}` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="发回物流" span={2}>
                {rr.return_tracking_number ? `${rr.return_company || ''} ${rr.return_tracking_number}` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="寄件网点" span={2}>
                {detail?.site ? `${detail.site.name}${detail.site.address ? `（${detail.site.address}）` : ''}` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="创建时间">{rr.created_at ? formatBeijingDateTimeShort(rr.created_at) : '-'}</Descriptions.Item>
              <Descriptions.Item label="结算时间">{rr.closed_at ? formatBeijingDateTimeShort(rr.closed_at) : '-'}</Descriptions.Item>
            </Descriptions>

            <Descriptions size="small" column={1} bordered style={{ marginTop: 12 }}
              items={[
                {
                  key: 'legs', label: '分段物流费（实填）',
                  children: fees.length === 0 ? '-' : fees.map(f => (
                    <div key={f.id}>第 {f.leg} 段：{yuan(f.amount_cents)}（{f.filled_by ? f.filled_by.slice(0, 8) : '-'}）</div>
                  )),
                },
                {
                  key: 'timeline', label: '状态时间线',
                  children: (detail.timeline || []).length === 0 ? '-' : (detail.timeline || []).map(t => (
                    <div key={t.id}>
                      [{t.created_at ? formatBeijingDateTimeShort(t.created_at) : '-'}]{' '}
                      {svcTimelineLabels[t.record_type] || t.record_type}
                      {t.comment ? ` — ${t.comment}` : ''}
                    </div>
                  )),
                },
                {
                  key: 'payments', label: '支付汇总',
                  children: detail.payments
                    ? `已付 ${yuan(detail.payments.made_cents)}` +
                      (detail.payments.refund_cents > 0 ? ` ｜ 已退 ${yuan(detail.payments.refund_cents)}` : '') +
                      (detail.payments.pending_shortfall_cents > 0 ? ` ｜ 待补缴 ${yuan(detail.payments.pending_shortfall_cents)}` : '')
                    : '-',
                },
              ]}
            />

            <div style={{ marginTop: 12 }}>
              <div style={{ fontWeight: 'bold', marginBottom: 6 }}>用户评价</div>
              {review && review.id ? (
                <div>
                  <Rate disabled value={review.rating} />
                  {review.message ? <div style={{ marginTop: 6 }}>{review.message}</div> : null}
                  {(() => {
                    let photos = []
                    try { photos = typeof review.photos === 'string' ? JSON.parse(review.photos || '[]') : (review.photos || []) } catch { photos = [] }
                    return photos.length ? (
                      <Image.PreviewGroup style={{ marginTop: 8 }}>
                        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 8 }}>
                          {photos.map((p, i) => <Image key={i} src={p} width={80} height={80} style={{ objectFit: 'cover', borderRadius: 8 }} />)}
                        </div>
                      </Image.PreviewGroup>
                    ) : null
                  })()}
                </div>
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无评价" />}
            </div>
          </>
        )}
      </Modal>
    </>
  )
}
