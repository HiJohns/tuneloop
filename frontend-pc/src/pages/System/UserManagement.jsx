import { useState, useEffect } from 'react'
import { Table, Input, Button, Space, Modal, Form, InputNumber, Switch, message, Tag, Typography, Collapse, Descriptions, Image, Divider, Alert } from 'antd'
import { DownloadOutlined, IdcardOutlined, ScanOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { api, faceReviewApi } from '../../services/api'
import IdPhotoDisplay from '../../components/IdPhotoDisplay'

const { Text } = Typography

const VERIFY_STATUS = {
  none: { label: '未上传证件照', color: 'default' },
  uploaded: { label: '已上传证件照', color: 'blue' },
  pending_review: { label: '审核中', color: 'gold' },
  verified: { label: '已核身', color: 'green' },
  rejected: { label: '审核驳回', color: 'red' },
}

function idCardStatus(user) {
  if (!user) return 'none'
  if (user.face_verified) return 'verified'
  if (user.real_name) return 'collected'
  if (user.id_photo_front || user.id_photo_back || user.id_photo_other) return 'provided'
  return 'none'
}

export default function UserManagement() {
  const [list, setList] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [detailVisible, setDetailVisible] = useState(false)
  const [current, setCurrent] = useState(null)
  const [idPhotos, setIdPhotos] = useState({ front: '', back: '', other: '' })
  const [detail, setDetail] = useState(null)
  const [idCardForm] = Form.useForm()
  const [form] = Form.useForm()
  // Module 2: face batches + review state
  const [batches, setBatches] = useState([])
  const [approving, setApproving] = useState(null)
  const [reviewing, setReviewing] = useState(null)
  const [rejectReason, setRejectReason] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [showIdPhotosCompare, setShowIdPhotosCompare] = useState(false)

  const fetchList = async (p = page, ps = pageSize, s = search) => {
    setLoading(true)
    try {
      const resp = await api.get('/admin/user-management', { params: { page: p, pageSize: ps, search: s } })
      if (resp?.code === 20000) {
        setList(resp.data.list || [])
        setTotal(resp.data.total || 0)
      }
    } catch (err) {
      message.error('加载失败: ' + (err.message || ''))
    }
    setLoading(false)
  }

  useEffect(() => { fetchList(1) }, [])

  const handleExport = () => {
    const params = new URLSearchParams()
    if (search) params.set('search', search)
    window.open(`/api/admin/user-management/export?${params.toString()}`)
  }

  const loadDetail = (userId) => {
    setDetail(null)
    setBatches([])
    api.get(`/admin/user-management/${userId}`).then((resp) => {
      if (resp?.code === 20000 && resp.data) {
        setDetail(resp.data)
        setIdPhotos({
          front: resp.data.id_photo_front || '',
          back: resp.data.id_photo_back || '',
          other: resp.data.id_photo_other || '',
          otherType: resp.data.id_photo_other_type || '',
        })
        idCardForm.setFieldsValue({
          real_name: resp.data.real_name || '',
          id_card_no: resp.data.id_card_no || '',
          id_card_expire: resp.data.id_card_expire || '',
          id_card_authority: resp.data.id_card_authority || '',
          id_card_address: resp.data.id_card_address || '',
        })
      }
    }).catch(() => {})
    // Module 2: face capture batch history (#1810)
    faceReviewApi.userBatches(userId).then((resp) => {
      if (resp?.code === 20000) setBatches(resp.data?.list || [])
    }).catch(() => {})
  }

  const openDetail = (record) => {
    setCurrent(record)
    form.setFieldsValue({
      membership_level_id: record.membership_level_id,
      promo_points: record.points != null ? record.points / 100 : undefined,
      status: record.status === 'active',
    })
    setIdPhotos({ front: '', back: '', other: '' })
    setDetailVisible(true)
    loadDetail(record.id)
  }

  const handleSave = async () => {
    const values = await form.validateFields()
    try {
      const resp = await api.put(`/admin/user-management/${current.id}`, {
        membership_level_id: values.membership_level_id,
        promo_points: values.promo_points != null ? Math.round(values.promo_points * 100) : undefined,
        status: values.status ? 'active' : 'disabled',
      })
      if (resp?.code === 20000) {
        message.success('保存成功')
        setDetailVisible(false)
        fetchList()
      } else {
        message.error(resp?.message || '保存失败')
      }
    } catch (err) {
      message.error('保存失败: ' + (err.message || ''))
    }
  }

  // ---- Module 1: ID card info ----
  const handleSaveIdCard = async () => {
    const values = await idCardForm.validateFields()
    try {
      const resp = await api.put(`/admin/user-management/${current.id}/id-card`, {
        real_name: values.real_name?.trim() || '',
        id_card_no: values.id_card_no?.trim() || '',
        id_card_expire: values.id_card_expire?.trim() || '',
        id_card_authority: values.id_card_authority?.trim() || '',
        id_card_address: values.id_card_address?.trim() || '',
      })
      if (resp?.code === 20000) {
        message.success('身份证信息已保存')
        loadDetail(current.id)
      } else {
        message.error(resp?.message || '保存失败')
      }
    } catch (err) {
      message.error('保存失败: ' + (err.message || ''))
    }
  }

  const handleRejectIdPhotos = () => {
    Modal.confirm({
      title: '拒绝采用身份证照片？',
      content: '将清除该用户的全部身份证照片与实名信息，并作废待审核的人脸采集批次。该操作不可撤销。',
      okText: '拒绝采用',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          const resp = await api.post(`/admin/user-management/${current.id}/id-photo/reject`)
          if (resp?.code === 20000) {
            message.success('已拒绝采用，通知已发送')
            loadDetail(current.id)
          } else {
            message.error(resp?.message || '操作失败')
          }
        } catch (err) {
          message.error('操作失败: ' + (err.message || ''))
        }
      },
    })
  }

  // ---- Module 2: face review ----
  const handleApprove = async (batchId) => {
    const values = approving
    if (!values.real_name?.trim() || !values.id_card_no?.trim() || !values.id_card_expire?.trim() || !values.id_card_authority?.trim() || !values.id_card_address?.trim()) {
      message.warning('请填写完整实名信息（根据证件照核对）')
      return
    }
    if (!/^\d{17}[\dX]$/.test(values.id_card_no.trim())) {
      message.warning('身份证号格式不正确（18 位，末位可为 X）')
      return
    }
    setSubmitting(true)
    try {
      const resp = await faceReviewApi.review(batchId, {
        action: 'approve',
        real_name: values.real_name.trim(),
        id_card_no: values.id_card_no.trim(),
        id_card_expire: values.id_card_expire.trim(),
        id_card_authority: values.id_card_authority.trim(),
        id_card_address: values.id_card_address.trim(),
      })
      if (resp.code === 20000) {
        message.success('已通过')
        setApproving(null)
        loadDetail(current.id)
      } else {
        message.error(resp.message || '操作失败')
      }
    } catch (error) {
      message.error(error.message || '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleReject = async (batchId) => {
    if (!rejectReason.trim()) {
      message.warning('请填写驳回原因')
      return
    }
    setSubmitting(true)
    try {
      const resp = await faceReviewApi.review(batchId, { action: 'reject', reason: rejectReason.trim() })
      if (resp.code === 20000) {
        message.success('已驳回')
        setReviewing(null)
        setRejectReason('')
        loadDetail(current.id)
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
      title: '昵称', dataIndex: 'nickname', key: 'nickname',
      render: (v, record) => (
        <Button type="link" size="small" style={{ padding: 0 }} onClick={() => openDetail(record)}>
          {v || record.username || record.phone || '-'}
        </Button>
      ),
    },
    { title: '微信号', dataIndex: 'wx_openid', key: 'wx_openid', render: v => v ? <Text ellipsis style={{ maxWidth: 120 }}>{v}</Text> : '-' },
    { title: '电话', dataIndex: 'phone', key: 'phone', render: v => v || '-' },
    { title: '当前等级', dataIndex: 'level', key: 'level', render: v => v || '-' },
    { title: '当前积分', dataIndex: 'points', key: 'points', render: v => v != null ? v / 100 : '-' },
    { title: '注册时间', dataIndex: 'registered_at', key: 'registered_at', render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '最新活动', dataIndex: 'last_active', key: 'last_active', render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: v => v === 'disabled'
      ? <Tag color="red">已禁用</Tag>
      : v === 'active'
        ? <Tag color="green">可用</Tag>
        : <Tag>{v || '未知'}</Tag> },
  ]

  const idCardStatusTag = idCardStatus(detail)
  const verifyStatus = detail?.id_verify_status || 'none'
  const latestBatch = batches.length > 0 ? batches[0] : null

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Input.Search
          placeholder="搜索 昵称/电话/微信号"
          allowClear
          style={{ width: 280 }}
          onSearch={(v) => { setSearch(v); fetchList(1, pageSize, v) }}
        />
        <Button icon={<DownloadOutlined />} onClick={handleExport}>导出 CSV</Button>
      </Space>

      <Table
        rowKey="id"
        loading={loading}
        dataSource={list}
        columns={columns}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); fetchList(p, ps, search) },
        }}
      />

      <Modal
        title={current ? `用户详情: ${current.username}` : '用户详情'}
        open={detailVisible}
        onOk={handleSave}
        onCancel={() => setDetailVisible(false)}
        destroyOnClose
        width={720}
      >
        <Form form={form} layout="vertical">
          <Form.Item label="当前等级" name="membership_level_id">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="等级 ID（见会员级别管理）" />
          </Form.Item>
          <Form.Item label="当前积分（点，1点=1元）" name="promo_points">
            <InputNumber min={0} precision={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="禁用/可用" name="status" valuePropName="checked">
            <Switch checkedChildren="可用" unCheckedChildren="禁用" />
          </Form.Item>
          {current && (
            <Form.Item label="身份证照片">
              <div style={{ display: 'flex', gap: 24 }}>
                <IdPhotoDisplay
                  side="front"
                  initialUrl={idPhotos.front}
                  uploadEndpoint={`/admin/user-management/${current.id}/id-photo`}
                  deleteEndpoint={`/admin/user-management/${current.id}/id-photo`}
                />
                <IdPhotoDisplay
                  side="back"
                  initialUrl={idPhotos.back}
                  uploadEndpoint={`/admin/user-management/${current.id}/id-photo`}
                  deleteEndpoint={`/admin/user-management/${current.id}/id-photo`}
                />
              </div>
            </Form.Item>
          )}
          {current && (
            <Form.Item label={`其他证件照片${idPhotos.otherType ? `（${idPhotos.otherType}）` : ''}`}>
              <div style={{ display: 'flex', gap: 24 }}>
                <IdPhotoDisplay
                  side="other"
                  label={idPhotos.otherType || '其他证件'}
                  initialUrl={idPhotos.other}
                  uploadEndpoint=""
                  deleteEndpoint=""
                  readOnly
                />
              </div>
            </Form.Item>
          )}

          {/* #1810: 实名核身区块 */}
          <Divider plain style={{ margin: '16px 0' }}>实名核身</Divider>
          <Collapse
            defaultActiveKey={[]}
            items={[
              {
                key: 'id-card',
                label: (
                  <Space>
                    <IdcardOutlined />
                    <Text strong>身份证信息</Text>
                    {idCardStatusTag === 'verified' && <Tag color="green">已核身</Tag>}
                    {idCardStatusTag === 'collected' && <Tag color="blue">已采集</Tag>}
                    {idCardStatusTag === 'provided' && <Tag color="gold">待采集</Tag>}
                    {idCardStatusTag === 'none' && <Tag>未提供</Tag>}
                  </Space>
                ),
                children: idCardStatusTag === 'none' ? (
                  <Alert type="info" showIcon message="用户未上传证件照，无法采集身份证信息" />
                ) : (
                  <div>
                    <Form form={idCardForm} layout="vertical" disabled={idCardStatusTag === 'verified'}>
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0 16px' }}>
                        <Form.Item label="真实姓名" name="real_name">
                          <Input placeholder="按证件照填写" />
                        </Form.Item>
                        <Form.Item label="身份证号" name="id_card_no">
                          <Input placeholder="18 位，末位可为 X" maxLength={18} />
                        </Form.Item>
                        <Form.Item label="有效期" name="id_card_expire">
                          <Input placeholder="YYYY-MM-DD 或「长期」" />
                        </Form.Item>
                        <Form.Item label="签发机关" name="id_card_authority">
                          <Input placeholder="按证件照填写" />
                        </Form.Item>
                        <Form.Item label="住址" name="id_card_address" style={{ gridColumn: '1 / -1' }}>
                          <Input placeholder="按证件照填写" />
                        </Form.Item>
                      </div>
                      <Space>
                        <Button type="primary" onClick={handleSaveIdCard}>保存</Button>
                      </Space>
                    </Form>
                    {/* Bug 3 fix: 拒绝采用按钮移出 Form disabled 包裹，verified 状态下仍可点击 */}
                    <Button danger onClick={handleRejectIdPhotos} style={{ marginTop: 8 }}>拒绝采用</Button>
                  </div>
                ),
              },
              {
                key: 'face',
                label: (
                  <Space>
                    <ScanOutlined />
                    <Text strong>人脸信息采集</Text>
                    {verifyStatus === 'verified' && <Tag color="green">{VERIFY_STATUS[verifyStatus].label}</Tag>}
                    {verifyStatus === 'pending_review' && <Tag color="gold">{VERIFY_STATUS[verifyStatus].label}</Tag>}
                    {verifyStatus === 'rejected' && <Tag color="red">{VERIFY_STATUS[verifyStatus].label}</Tag>}
                    {verifyStatus === 'uploaded' && <Tag color="blue">{VERIFY_STATUS[verifyStatus].label}</Tag>}
                    {verifyStatus === 'none' && <Tag>{VERIFY_STATUS[verifyStatus].label}</Tag>}
                    {detail?.face_verify_method === 'tencent' && <Text type="secondary" style={{ fontSize: 12 }}>自动识别</Text>}
                    {detail?.face_verify_method === 'manual' && <Text type="secondary" style={{ fontSize: 12 }}>人工审核</Text>}
                  </Space>
                ),
                children: (
                  <div>
                    {batches.length === 0 && (
                      <Alert type="info" showIcon message="用户未提交人脸采集素材" />
                    )}
                    {batches.map((b) => (
                      <div key={b.batch_id} style={{ marginBottom: 12, padding: 12, background: '#fafafa', borderRadius: 6 }}>
                        <Space style={{ marginBottom: 8 }}>
                          <Tag color={b.status === 'approved' ? 'green' : b.status === 'pending' ? 'gold' : 'red'}>
                            {b.status === 'approved' ? '已通过' : b.status === 'pending' ? '待审核' : '已驳回'}
                          </Tag>
                          <Text type="secondary" style={{ fontSize: 12 }}>提交于 {b.submitted_at}</Text>
                          {b.reviewed_at && <Text type="secondary" style={{ fontSize: 12 }}>审核于 {b.reviewed_at}</Text>}
                        </Space>
                        {b.reject_reason && <Alert type="error" showIcon message={`驳回原因：${b.reject_reason}`} style={{ marginBottom: 8 }} />}
                        <Space direction="vertical" size={8}>
                          <Space size={8} wrap>
                            {(b.selfie_urls || []).map((url, i) => (
                              url.endsWith('.mp4') || url.endsWith('.mov') || url.endsWith('.webm')
                                ? <video key={i} src={url} controls style={{ width: 96, height: 64, borderRadius: 4, background: '#f4f4f5' }} />
                                : <Image key={i} src={url} width={64} height={64} style={{ objectFit: 'cover', borderRadius: 4 }} />
                            ))}
                            {(b.selfie_urls || []).length === 0 && <Text type="secondary">无自拍素材</Text>}
                          </Space>
                          {b.status === 'pending' && detail?.face_verify_method !== 'tencent' && (
                            <Space>
                              <Button type="primary" size="small" icon={<CheckCircleOutlined />}
                                onClick={() => setApproving({ batch_id: b.batch_id })}>确认通过</Button>
                              <Button danger size="small" icon={<CloseCircleOutlined />}
                                onClick={() => setReviewing(b.batch_id)}>驳回</Button>
                              <Button size="small" onClick={() => setShowIdPhotosCompare(true)}>唤出证件照对比</Button>
                            </Space>
                          )}
                          {b.status === 'approved' && detail?.face_verified && (
                            <Text type="success"><CheckCircleOutlined /> 已核身{detail?.face_verified_at ? `（${new Date(detail.face_verified_at).toLocaleString()}）` : ''}</Text>
                          )}
                        </Space>
                      </div>
                    ))}
                  </div>
                ),
              },
            ]}
          />
        </Form>
      </Modal>

      {/* Approve modal: fill identity info per #1807 */}
      <Modal
        title="确认通过核身"
        open={!!approving}
        onOk={() => handleApprove(approving?.batch_id)}
        confirmLoading={submitting}
        onCancel={() => setApproving(null)}
        okText="确认通过"
        cancelText="取消"
      >
        <Alert type="warning" showIcon message="请根据身份证照核对填写以下实名信息（不可留空）" style={{ marginBottom: 12 }} />
        <Form layout="vertical">
          <Form.Item label="真实姓名" required>
            <Input value={approving?.real_name || ''} onChange={(e) => setApproving({ ...approving, real_name: e.target.value })} />
          </Form.Item>
          <Form.Item label="身份证号" required>
            <Input maxLength={18} value={approving?.id_card_no || ''} onChange={(e) => setApproving({ ...approving, id_card_no: e.target.value })} />
          </Form.Item>
          <Form.Item label="有效期" required>
            <Input placeholder="YYYY-MM-DD 或「长期」" value={approving?.id_card_expire || ''} onChange={(e) => setApproving({ ...approving, id_card_expire: e.target.value })} />
          </Form.Item>
          <Form.Item label="签发机关" required>
            <Input value={approving?.id_card_authority || ''} onChange={(e) => setApproving({ ...approving, id_card_authority: e.target.value })} />
          </Form.Item>
          <Form.Item label="住址" required>
            <Input value={approving?.id_card_address || ''} onChange={(e) => setApproving({ ...approving, id_card_address: e.target.value })} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Reject modal */}
      <Modal
        title="驳回人脸采集"
        open={!!reviewing}
        onOk={() => handleReject(reviewing)}
        confirmLoading={submitting}
        onCancel={() => setReviewing(null)}
        okText="驳回"
        okButtonProps={{ danger: true }}
        cancelText="取消"
      >
        <Input.TextArea
          rows={3}
          placeholder="请填写驳回原因（必填，将通知用户）"
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
        />
      </Modal>

      {/* ID photo compare modal */}
      <Modal
        title="身份证照对比"
        open={showIdPhotosCompare}
        onCancel={() => setShowIdPhotosCompare(false)}
        footer={null}
        width={560}
      >
        <div style={{ display: 'flex', gap: 16, justifyContent: 'center' }}>
          {idPhotos.front && <Image src={idPhotos.front} width={160} alt="正面" />}
          {idPhotos.back && <Image src={idPhotos.back} width={160} alt="背面" />}
          {idPhotos.other && <Image src={idPhotos.other} width={160} alt="其他证件" />}
          {!idPhotos.front && !idPhotos.back && !idPhotos.other && <Text type="secondary">该用户无证件照</Text>}
        </div>
      </Modal>
    </div>
  )
}
