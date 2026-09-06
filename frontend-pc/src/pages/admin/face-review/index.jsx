// 实名核身人工审核队列（#1793 T5）
// 权限：平台员工/系统管理员（SysPermUserUpdate = bit 18）
// 列表：pending 批次 + 用户证件照三张 + 自拍图/视频 → 通过/驳回（驳回必填原因）
// 字段边界：仅展示审核所需（姓名 + 证件照 + 自拍），不展示身份证号明文
import { useState, useCallback, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Table, Card, Button, Space, Tag, Image, Modal, Input, message, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { faceReviewApi } from '../../../services/api'

const { Text } = Typography

export default function FaceReviewPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const filterUserId = searchParams.get('user_id') || ''
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const [reviewing, setReviewing] = useState(null) // 当前驳回的 batch
  const [rejectReason, setRejectReason] = useState('')
  // #1807/#1822: 通过弹窗两态——未采录时员工录入 5 项实名信息（根据证件照核对）；
  // 已采录（id_info_collected=true）时仅展示只读摘要做证/人核验，确认即通过。
  const [approving, setApproving] = useState(null) // 当前通过的 batch record
  const [realName, setRealName] = useState('')
  const [idCardNo, setIdCardNo] = useState('')
  const [idCardExpire, setIdCardExpire] = useState('')
  const [idCardAuthority, setIdCardAuthority] = useState('')
  const [idCardAddress, setIdCardAddress] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const fetchQueue = useCallback(async () => {
    setLoading(true)
    try {
      const params = filterUserId ? { user_id: filterUserId } : {}
      const resp = await faceReviewApi.queue(params)
      if (resp.code === 20000) {
        setList(resp.data?.list || [])
      } else {
        message.error(resp.message || '加载审核队列失败')
      }
    } catch (error) {
      message.error(error.message || '加载审核队列失败')
    } finally {
      setLoading(false)
    }
  }, [filterUserId])

  useEffect(() => { fetchQueue() }, [fetchQueue])

  // #1822: 已采录用户 approve 不带身份证字段（只读摘要核验，防覆盖已存实名信息）；
  // 未采录用户仍需员工录入完整 5 项（按证件照抄录，#1807）。
  const handleApprove = async () => {
    const collected = !!(approving && approving.id_info_collected)
    if (!collected) {
      if (!realName.trim() || !idCardNo.trim() || !idCardExpire.trim() || !idCardAuthority.trim() || !idCardAddress.trim()) {
        message.warning('请填写完整实名信息（真实姓名、身份证号、有效期、签发机关、住址，根据证件照核对）')
        return
      }
      if (!/^\d{17}[\dX]$/.test(idCardNo.trim())) {
        message.warning('身份证号格式不正确（18 位，末位可为 X）')
        return
      }
    }
    setSubmitting(true)
    try {
      const payload = collected
        ? { action: 'approve' }
        : {
            action: 'approve',
            real_name: realName.trim(),
            id_card_no: idCardNo.trim(),
            id_card_expire: idCardExpire.trim(),
            id_card_authority: idCardAuthority.trim(),
            id_card_address: idCardAddress.trim(),
          }
      const resp = await faceReviewApi.review(approving.batch_id, payload)
      if (resp.code === 20000) {
        message.success('已通过')
        setApproving(null)
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
            {videos.map((url, i) => (
              <video key={i} src={url} controls style={{ width: 96, height: 64, borderRadius: 4, background: '#f4f4f5' }} />
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
      render: (v) => v ? new Date(v).toLocaleString('zh-CN') : '-',
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
            onClick={() => { setApproving(record); setRealName(''); setIdCardNo(''); setIdCardExpire(''); setIdCardAuthority(''); setIdCardAddress('') }}
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

  return (
    <Card
      title="实名核身审核队列"
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
      <Table
        rowKey="batch_id"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: '暂无待审核批次' }}
      />
      <Modal
        title="通过核身申请"
        open={!!approving}
        onOk={handleApprove}
        onCancel={() => setApproving(null)}
        okText="确认通过"
        cancelText="取消"
        okButtonProps={{ loading: submitting }}
      >
        {approving && approving.id_info_collected ? (
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
    </Card>
  )
}
