// 实名核身人工审核队列（#1793 T5）
// 权限：平台员工/系统管理员（SysPermUserUpdate = bit 18）
// 列表：pending 批次 + 用户证件照三张 + 自拍图/视频 → 通过/驳回（驳回必填原因）
// 字段边界：仅展示审核所需（姓名 + 证件照 + 自拍），不展示身份证号明文
import { useState, useCallback, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Table, Card, Button, Space, Tag, Image, Modal, Input, Select, Tabs, message, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { faceReviewApi } from '../../../services/api'
import { formatBeijingDateTimeShort } from '../../../utils/date'

const { Text } = Typography

// #1924: 审核员指定的第二证件类型
const SECOND_DOC_OPTIONS = [
  { value: 'student', label: '学生证' },
  { value: 'teacher', label: '教职工证' },
  { value: 'work', label: '工作证' },
  { value: 'other', label: '其他' },
]

export default function FaceReviewPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const filterUserId = searchParams.get('user_id') || ''
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [reviewing, setReviewing] = useState(null) // 当前驳回的 batch
  const [rejectReason, setRejectReason] = useState('')
  // #1807/#1822: 通过弹窗两态——未采录时员工录入 5 项实名信息（根据证件照核对）；
  // 已采录（id_info_collected=true）时仅展示只读摘要做证/人核验，确认即通过。
  const [approving, setApproving] = useState(null) // 当前通过的 batch record
  const [secondDocType, setSecondDocType] = useState('') // #1924: 第二证件类型
  const [previewVideo, setPreviewVideo] = useState(null) // #1921: 大窗播放的自拍视频 URL
  const [realName, setRealName] = useState('')
  const [idCardNo, setIdCardNo] = useState('')
  const [idCardExpire, setIdCardExpire] = useState('')
  const [idCardAuthority, setIdCardAuthority] = useState('')
  const [idCardAddress, setIdCardAddress] = useState('')
  const [submitting, setSubmitting] = useState(false)
  // #2242: 审核历史（队列 | 历史）
  const [activeTab, setActiveTab] = useState('queue')
  const [historyList, setHistoryList] = useState([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyPage, setHistoryPage] = useState(1)
  const [historyPageSize, setHistoryPageSize] = useState(20)
  const [historyTotal, setHistoryTotal] = useState(0)
  const [historyName, setHistoryName] = useState('')
  const [historyAction, setHistoryAction] = useState('')
  const [historyDetail, setHistoryDetail] = useState(null)

  const fetchQueue = useCallback(async () => {
    setLoading(true)
    try {
      const params = { page, page_size: pageSize, ...(filterUserId ? { user_id: filterUserId } : {}) }
      const resp = await faceReviewApi.queue(params)
      if (resp.code === 20000) {
        setList(resp.data?.list || [])
        setTotal(resp.data?.total || 0)
      } else {
        message.error(resp.message || '加载审核队列失败')
      }
    } catch (error) {
      message.error(error.message || '加载审核队列失败')
    } finally {
      setLoading(false)
    }
  }, [filterUserId, page, pageSize])

  useEffect(() => { fetchQueue() }, [fetchQueue])

  // #2242: 审核历史（服务端分页 + 实名姓名搜索）
  const fetchHistory = useCallback(async () => {
    setHistoryLoading(true)
    try {
      const params = { page: historyPage, page_size: historyPageSize }
      if (historyName) params.real_name = historyName
      if (historyAction) params.action = historyAction
      if (filterUserId) params.user_id = filterUserId
      const resp = await faceReviewApi.history(params)
      if (resp.code === 20000) {
        setHistoryList(resp.data?.list || [])
        setHistoryTotal(resp.data?.total || 0)
      } else {
        message.error(resp.message || '加载审核历史失败')
      }
    } catch (error) {
      message.error(error.message || '加载审核历史失败')
    } finally {
      setHistoryLoading(false)
    }
  }, [historyPage, historyPageSize, historyName, historyAction, filterUserId])

  useEffect(() => {
    if (activeTab === 'history') fetchHistory()
  }, [activeTab, fetchHistory])

  // #1822: 已采录用户 approve 不带身份证字段（只读摘要核验，防覆盖已存实名信息）；
  // 未采录用户仍需员工录入完整 5 项（按证件照抄录，#1807）。
  const handleApprove = async () => {
    const collected = !!(approving && approving.id_info_collected)
    const isSecondDoc = approving?.kind === 'second_doc'
    const needsSecondType = !!(approving?.has_second_doc)
    // #1924: 第二证件复审只指定类型；注册审核未采录时才需录入 5 项实名信息。
    if (!collected && !isSecondDoc) {
      if (!realName.trim() || !idCardNo.trim() || !idCardExpire.trim() || !idCardAuthority.trim() || !idCardAddress.trim()) {
        message.warning('请填写完整实名信息（真实姓名、身份证号、有效期、签发机关、住址，根据证件照核对）')
        return
      }
      if (!/^\d{17}[\dX]$/.test(idCardNo.trim())) {
        message.warning('身份证号格式不正确（18 位，末位可为 X）')
        return
      }
    }
    if (needsSecondType && !secondDocType) {
      message.warning('请指定第二证件类型（学生证/教职工证/工作证/其他）')
      return
    }
    // #2104/#2057 裁定3：学生证作为第二证件必须已有介绍信，否则引导驳回补传
    if (needsSecondType && secondDocType === 'student' && !approving?.intro_letter_url) {
      message.warning('学生证作为第二证件需介绍信，请驳回并要求补传')
      return
    }
    setSubmitting(true)
    try {
      const payload = { action: 'approve' }
      if (!collected && !isSecondDoc) {
        payload.real_name = realName.trim()
        payload.id_card_no = idCardNo.trim()
        payload.id_card_expire = idCardExpire.trim()
        payload.id_card_authority = idCardAuthority.trim()
        payload.id_card_address = idCardAddress.trim()
      }
      if (needsSecondType) {
        payload.second_doc_type = secondDocType
      }
      const resp = await faceReviewApi.review(approving.batch_id, payload)
      if (resp.code === 20000) {
        message.success('已通过')
        setApproving(null)
        setSecondDocType('')
        setRealName('')
        setIdCardNo('')
        setIdCardExpire('')
        setIdCardAuthority('')
        setIdCardAddress('')
        fetchQueue()
      } else {
        message.error(resp.message || '操作失败')
      }
    } catch (error) {
      message.error(error.message || '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleReject = async () => {
    if (!rejectReason.trim()) {
      message.warning('请填写驳回原因')
      return
    }
    setSubmitting(true)
    try {
      const resp = await faceReviewApi.review(reviewing, { action: 'reject', reason: rejectReason.trim() })
      if (resp.code === 20000) {
        message.success('已驳回')
        setReviewing(null)
        setRejectReason('')
        fetchQueue()
      } else {
        message.error(resp.message || '操作失败')
      }
    } catch (error) {
      message.error(error.message || '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns = [
    {
      title: '用户',
      dataIndex: 'user_name',
      key: 'user_name',
      render: (name, record) => (
        <Space direction="vertical" size={0}>
          <Space size={6}>
            <Text strong>{name || '-'}</Text>
            {record.id_info_collected && <Tag color="green" style={{ fontSize: 12 }}>实名已采录</Tag>}
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }}>{record.user_id}</Text>
        </Space>
      ),
    },
    {
      title: '证件照片',
      key: 'id_photos',
      render: (_, record) => (
        <Space>
          {(record.id_photos || []).map((url, i) => (
            <Image key={i} src={url} width={48} height={48} style={{ objectFit: 'cover', borderRadius: 4 }} />
          ))}
          {(record.id_photos || []).length === 0 && <Text type="secondary">无</Text>}
          {record.has_second_doc && (
            <Tag color={record.id_photo_other_verified ? 'green' : 'orange'}>
              第二证件{record.id_photo_other_verified ? '已认证' : '未认证'}
            </Tag>
          )}
          {record.kind === 'second_doc' && <Tag color="blue">第二证件复审</Tag>}
        </Space>
      ),
    },
    {
      title: '自拍照',
      key: 'selfie_photos',
      render: (_, record) => {
        const photos = (record.selfie_urls || []).filter((u) => !/\.(mp4|mov|webm)(\?|$)/i.test(u))
        return (
          <Space>
            {photos.map((url, i) => (
              <Image key={i} src={url} width={48} height={48} style={{ objectFit: 'cover', borderRadius: 4 }} />
            ))}
            {photos.length === 0 && <Text type="secondary">无</Text>}
          </Space>
        )
      },
    },
    {
      title: '自拍视频',
      key: 'selfie_videos',
      render: (_, record) => {
        const videos = (record.selfie_urls || []).filter((u) => /\.(mp4|mov|webm)(\?|$)/i.test(u))
        return (
          <Space direction="vertical" size={4}>
            {/* #1921: 96×64 缩略，点击弹出大窗播放（证件照/自拍照为 antd Image，自带点击放大预览） */}
            {videos.map((url, i) => (
              <video
                key={i}
                src={url}
                muted
                preload="metadata"
                onClick={() => setPreviewVideo(url)}
                style={{ width: 96, height: 64, borderRadius: 4, background: '#f4f4f5', cursor: 'pointer', objectFit: 'cover' }}
              />
            ))}
            {videos.length === 0 && <Text type="secondary">无</Text>}
          </Space>
        )
      },
    },
    {
      title: '提交时间',
      dataIndex: 'submitted_at',
      key: 'submitted_at',
      render: (v) => v ? formatBeijingDateTimeShort(v) : '-',
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space>
          <Button
            size="small"
            type="primary"
            icon={<CheckCircleOutlined />}
            onClick={() => { setApproving(record); setSecondDocType(record.id_photo_other_type || ''); setRealName(''); setIdCardNo(''); setIdCardExpire(''); setIdCardAuthority(''); setIdCardAddress('') }}
          >
            通过
          </Button>
          <Button
            size="small"
            danger
            icon={<CloseCircleOutlined />}
            onClick={() => { setReviewing(record.batch_id); setRejectReason('') }}
          >
            驳回
          </Button>
        </Space>
      ),
    },
  ]

  // #2242: 历史列表列
  const historyColumns = [
    {
      title: '审核时间',
      dataIndex: 'reviewed_at',
      key: 'reviewed_at',
      width: 170,
      render: (v) => (v ? formatBeijingDateTimeShort(v) : '-'),
    },
    {
      title: '实名姓名 / 用户',
      key: 'real_name',
      render: (_, r) => (
        <Space direction="vertical" size={0}>
          <Text strong>{r.real_name || '-'}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>{r.user_name || r.user_id}</Text>
        </Space>
      ),
    },
    {
      title: '批次',
      dataIndex: 'kind',
      key: 'kind',
      width: 120,
      render: (k) => (k === 'second_doc' ? <Tag color="blue">第二证件复审</Tag> : <Tag>注册核身</Tag>),
    },
    {
      title: '结果',
      dataIndex: 'result',
      key: 'result',
      width: 90,
      render: (v) => (v === 'approve' ? <Tag color="green">通过</Tag> : <Tag color="red">驳回</Tag>),
    },
    {
      title: '驳回原因',
      dataIndex: 'reject_reason',
      key: 'reject_reason',
      render: (v) => v || '-',
    },
    {
      title: '审核人',
      dataIndex: 'reviewed_by',
      key: 'reviewed_by',
      width: 140,
      render: (v) => v || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_, r) => <Button type="link" size="small" onClick={() => setHistoryDetail(r)}>查看详情</Button>,
    },
  ]

  return (
    <Card
      title="实名核身审核"
      extra={
        <Space>
          {filterUserId && (
            <Tag closable onClose={() => { searchParams.delete('user_id'); setSearchParams(searchParams) }}>
              按用户筛选：{filterUserId.slice(0, 8)}…
            </Tag>
          )}
          <Button size="small" onClick={fetchQueue}>刷新</Button>
        </Space>
      }
    >
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'queue',
            label: total ? `待审核（${total}）` : '待审核',
            children: (
              <Table
                rowKey="batch_id"
                columns={columns}
                dataSource={list}
                loading={loading}
                pagination={{
                  current: page, pageSize, total, showSizeChanger: true,
                  showTotal: (t) => `共 ${t} 条`,
                  onChange: (p, ps) => { setPage(p); setPageSize(ps) },
                }}
                locale={{ emptyText: '暂无待审核批次' }}
              />
            ),
          },
          {
            key: 'history',
            label: '审核历史',
            children: (
              <>
                <Space style={{ marginBottom: 12 }} wrap>
                  <Input.Search
                    allowClear
                    placeholder="按实名姓名搜索"
                    style={{ width: 220 }}
                    onSearch={(v) => { setHistoryPage(1); setHistoryName((v || '').trim()) }}
                  />
                  <Select
                    allowClear
                    placeholder="结果"
                    style={{ width: 120 }}
                    value={historyAction || undefined}
                    onChange={(v) => { setHistoryPage(1); setHistoryAction(v || '') }}
                    options={[{ value: 'approve', label: '通过' }, { value: 'reject', label: '驳回' }]}
                  />
                  <Button size="small" onClick={fetchHistory}>刷新</Button>
                </Space>
                <Table
                  rowKey="batch_id"
                  size="small"
                  columns={historyColumns}
                  dataSource={historyList}
                  loading={historyLoading}
                  pagination={{
                    current: historyPage, pageSize: historyPageSize, total: historyTotal,
                    showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
                    onChange: (p, ps) => { setHistoryPage(p); setHistoryPageSize(ps) },
                  }}
                  locale={{ emptyText: '暂无审核历史' }}
                />
              </>
            ),
          },
        ]}
      />
      <Modal
        title="通过核身申请"
        open={!!approving}
        onOk={handleApprove}
        onCancel={() => { setApproving(null); setSecondDocType('') }}
        okText="确认通过"
        cancelText="取消"
        okButtonProps={{
          loading: submitting,
          // #2104：学生证缺介绍信时禁用「通过」（须走驳回补传），避免无效重试
          disabled: secondDocType === 'student' && approving?.has_second_doc && !approving?.intro_letter_url,
        }}
      >
        {approving && (approving.id_info_collected || approving.kind === 'second_doc') ? (
          // #1822: 已采录态——用户详情已维护实名信息，本弹窗只做证/人核验，
          // 展示只读摘要（不含身份证号明文），确认即 approve（不带 id 字段防覆盖）。
          <div>
            <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
              该用户实名信息已在用户详情中采录，请与证件照、人脸自拍核对一致后确认通过（无需重复填写）。
            </Text>
            <div style={{ border: '1px solid #f0f0f0', background: '#fafafa', borderRadius: 6, padding: '12px 16px' }}>
              <Space direction="vertical" size={6} style={{ width: '100%' }}>
                <Space><Text type="secondary" style={{ width: 90, display: 'inline-block' }}>真实姓名</Text><Text strong>{approving.real_name || '-'}</Text></Space>
                <Space><Text type="secondary" style={{ width: 90, display: 'inline-block' }}>身份证号</Text><Text strong>{approving.id_card_no_masked || '-'}</Text></Space>
                <Space><Text type="secondary" style={{ width: 90, display: 'inline-block' }}>有效期</Text><Text>{approving.id_card_expire || '-'}</Text></Space>
                <Space><Text type="secondary" style={{ width: 90, display: 'inline-block' }}>签发机关</Text><Text>{approving.id_card_authority || '-'}</Text></Space>
                <Space align="start"><Text type="secondary" style={{ width: 90, display: 'inline-block' }}>住址</Text><Text style={{ flex: 1 }}>{approving.id_card_address || '-'}</Text></Space>
              </Space>
            </div>
          </div>
        ) : (
          // #1807: 未采录态——实名信息由员工按证件照录入（顾客不自行输入）。
          <div>
            <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>请根据证件照核对填写完整实名信息（实名信息由员工填写，顾客不自行输入）。</Text>
            <Input
              value={realName}
              onChange={(e) => setRealName(e.target.value)}
              placeholder="真实姓名（必填）"
              style={{ marginBottom: 12 }}
            />
            <Input
              value={idCardNo}
              onChange={(e) => setIdCardNo(e.target.value)}
              placeholder="身份证号（必填，18 位）"
              maxLength={18}
              style={{ marginBottom: 12 }}
            />
            <Input
              value={idCardExpire}
              onChange={(e) => setIdCardExpire(e.target.value)}
              placeholder="身份证有效期（必填，如 2035-12-31 或 长期）"
              maxLength={20}
              style={{ marginBottom: 12 }}
            />
            <Input
              value={idCardAuthority}
              onChange={(e) => setIdCardAuthority(e.target.value)}
              placeholder="签发机关（必填，按证件照抄录）"
              maxLength={100}
              style={{ marginBottom: 12 }}
            />
            <Input
              value={idCardAddress}
              onChange={(e) => setIdCardAddress(e.target.value)}
              placeholder="证件住址（必填，按证件照抄录）"
              maxLength={200}
            />
          </div>
        )}
        {/* #1924: 用户提交了第二证件 → 审核人必须指定类型 */}
        {approving?.has_second_doc && (
          <div style={{ marginTop: 16 }}>
            <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
              第二证件类型（用户自报{approving?.id_photo_other_type ? `：${approving.id_photo_other_type}` : '：未选择'}；审核人可修正）
              {approving.id_photo_other_verified ? '（当前已认证，可修改类型）' : '（当前未认证）'}
            </Text>
            <Select
              value={secondDocType || undefined}
              onChange={setSecondDocType}
              options={SECOND_DOC_OPTIONS}
              placeholder="选择第二证件类型"
              style={{ width: 220 }}
            />
            {/* #2057 裁定3: 学生证介绍信（用户已传时展示，供审核核对） */}
            {approving?.intro_letter_url && (
              <div style={{ marginTop: 12 }}>
                <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>介绍信：</Text>
                <Image src={approving.intro_letter_url} width={80} height={80}
                  style={{ objectFit: 'cover', borderRadius: 4 }}
                  preview={{ src: approving.intro_letter_url }} />
              </div>
            )}
            {/* #2104: 学生证缺介绍信 → 前置引导（禁用通过，请走驳回补传） */}
            {secondDocType === 'student' && !approving?.intro_letter_url && (
              <div style={{ marginTop: 12 }}>
                <Text type="danger">学生证作为第二证件需介绍信，请驳回并要求补传</Text>
              </div>
            )}
          </div>
        )}
      </Modal>
      <Modal
        title="驳回核身申请"
        open={!!reviewing}
        onOk={handleReject}
        onCancel={() => setReviewing(null)}
        okText="确认驳回"
        cancelText="取消"
        okButtonProps={{ danger: true, loading: submitting }}
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>驳回原因将展示给顾客，请说明具体问题（如照片不清晰、与证件不符）。</Text>
        <Input.TextArea
          rows={3}
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
          placeholder="请输入驳回原因（必填）"
          maxLength={200}
          showCount
          style={{ marginBottom: 24 }}
        />
      </Modal>

      {/* #1921: 自拍视频大窗播放 */}
      <Modal
        open={!!previewVideo}
        title="自拍视频预览"
        footer={null}
        width={720}
        onCancel={() => setPreviewVideo(null)}
        destroyOnClose
      >
        {previewVideo && (
          <video
            src={previewVideo}
            controls
            autoPlay
            style={{ width: '100%', maxHeight: '70vh', borderRadius: 8, background: '#000' }}
          />
        )}
      </Modal>
      {/* #2242: 历史详情（审批快照） */}
      <Modal
        title="审核详情"
        open={!!historyDetail}
        onCancel={() => setHistoryDetail(null)}
        footer={null}
        width={560}
      >
        {historyDetail && (
          <Space direction="vertical" size={6} style={{ width: '100%' }}>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>审核结果</Text>
              {historyDetail.result === 'approve' ? <Tag color="green">通过</Tag> : <Tag color="red">驳回</Tag>}</Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>实名姓名</Text><Text strong>{historyDetail.real_name || '-'}</Text></Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>身份证号</Text><Text strong>{historyDetail.id_card_no_masked || '-'}</Text></Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>有效期</Text><Text>{historyDetail.id_card_expire || '-'}</Text></Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>签发机关</Text><Text>{historyDetail.id_card_authority || '-'}</Text></Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>住址</Text><Text>{historyDetail.id_card_address || '-'}</Text></Space>
            {historyDetail.second_doc_type && (
              <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>第二证件</Text><Text>{historyDetail.second_doc_type}</Text></Space>
            )}
            {historyDetail.reject_reason && (
              <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>驳回原因</Text><Text>{historyDetail.reject_reason}</Text></Space>
            )}
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>审核时间</Text><Text>{historyDetail.reviewed_at ? formatBeijingDateTimeShort(historyDetail.reviewed_at) : '-'}</Text></Space>
            <Space><Text type="secondary" style={{ width: 110, display: 'inline-block' }}>审核人</Text><Text>{historyDetail.reviewed_by || '-'}</Text></Space>
          </Space>
        )}
      </Modal>
    </Card>
  )
}
