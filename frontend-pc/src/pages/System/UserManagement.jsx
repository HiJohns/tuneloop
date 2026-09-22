import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Table, Input, Button, Space, Modal, Form, InputNumber, Switch, message, Tag, Typography, Collapse, Image, Divider, Alert } from 'antd'
import { DownloadOutlined, IdcardOutlined, ScanOutlined, CheckCircleOutlined } from '@ant-design/icons'
import { api, faceReviewApi } from '../../services/api'
import IdPhotoDisplay from '../../components/IdPhotoDisplay'
import { formatBeijingDate, formatBeijingDateTimeShort } from '../../utils/date'

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
  const navigate = useNavigate()
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
  const [grantVisible, setGrantVisible] = useState(false)
  const [grantForm] = Form.useForm()
  const [granting, setGranting] = useState(false)
  // Module 2: face batches (read-only, review via face-review queue #1813)
  const [batches, setBatches] = useState([])

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
    // eslint-disable-next-line no-undef
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

  const handleGrant = async () => {
    const values = await grantForm.validateFields()
    setGranting(true)
    try {
      const resp = await api.post(`/admin/user-management/${current.id}/points-grant`, {
        amount: values.amount,
        reason: values.reason?.trim(),
      })
      if (resp?.code === 20000) {
        message.success('加赠成功')
        setGrantVisible(false)
        grantForm.resetFields()
        fetchList()
        loadDetail(current.id)
      } else {
        message.error(resp?.message || '加赠失败')
      }
    } catch (err) {
      message.error('加赠失败: ' + (err.message || ''))
    }
    setGranting(false)
  }

  // ---- Module 1: ID card info ----
  const handleMarkDeleted = async () => {
    if (!current) return
    Modal.confirm({
      title: '标记删除该用户？',
      content: '将解除其全部组织关联并标记为不可用（账户记录保留，可重新加回）。此操作会使其立即无法登录。',
      okText: '确认标记删除',
      okButtonProps: { danger: true },
      onOk: async () => {
        const resp = await api.post(`/admin/user-management/${current.id}/mark-deleted`)
        if (resp.code === 20000) {
          message.success('已标记删除（账户保留，可重新加回）')
          setDetailVisible(false)
          fetchList()
        } else {
          message.error(resp.message || '标记删除失败')
        }
      },
    })
  }

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


  const columns = [
    {
      title: '昵称', dataIndex: 'nickname', key: 'nickname',
      render: (v, record) => (
        <Button type="link" size="small" style={{ padding: 0 }} onClick={() => openDetail(record)}>
          {v || record.username || record.phone || '-'}
        </Button>
      ),
    },
    { title: '电话', dataIndex: 'phone', key: 'phone', render: v => v || '-' },
    { title: '当前等级', dataIndex: 'level', key: 'level', render: v => v || '-' },
    { title: '当前乐币', dataIndex: 'points', key: 'points', render: v => v != null ? v / 100 : '-' },
    { title: '注册时间', dataIndex: 'registered_at', key: 'registered_at', render: v => v ? formatBeijingDateTimeShort(v) : '-' },
    { title: '最新活动', dataIndex: 'last_active', key: 'last_active', render: v => v ? formatBeijingDateTimeShort(v) : '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: v => v === 'disabled'
      ? <Tag color="red">已禁用</Tag>
      : v === 'active'
        ? <Tag color="green">可用</Tag>
        : <Tag>{v || '未知'}</Tag> },
  ]

  const idCardStatusTag = idCardStatus(detail)
  const verifyStatus = detail?.id_verify_status || 'none'

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
          <Form.Item label="当前乐币（点，1点=1元）">
            <Space>
              <Text strong>{current?.points != null ? current.points / 100 : '-'}</Text>
              <Button size="small" onClick={() => { grantForm.resetFields(); setGrantVisible(true) }}>加赠乐币</Button>
            </Space>
          </Form.Item>
          <Form.Item label="禁用/可用" name="status" valuePropName="checked">
            <Switch checkedChildren="可用" unCheckedChildren="禁用" />
          </Form.Item>
          {/* #2028 Step 4 / D4：标记删除——解除全部关联（记录保留，可重新加回） */}
          <Form.Item label="标记删除">
            <Space>
              <Button danger onClick={handleMarkDeleted} disabled={!current}>标记删除（解除全部关联）</Button>
              <Typography.Text type="secondary">账户保留，可重新加回；不等于彻底删除</Typography.Text>
            </Space>
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
                          <Text type="secondary" style={{ fontSize: 12 }}>提交于 {formatBeijingDateTimeShort(b.submitted_at)}</Text>
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
                            <Button type="link" size="small" onClick={() => navigate(`/face-review?user_id=${current?.id}`)}>
                              去实名审核队列处理 ›
                            </Button>
                          )}
                          {b.status === 'approved' && detail?.face_verified && (
                             <Text type="success"><CheckCircleOutlined /> 已核身{detail?.face_verified_at ? `（${formatBeijingDateTimeShort(detail.face_verified_at)}）` : ''}</Text>
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

      {/* #1982: 管理员加赠乐币（批次入口，替代原直接编辑） */}
      <Modal
        title={`加赠乐币${current ? ` · ${current.username || current.nickname || ''}` : ''}`}
        open={grantVisible}
        onOk={handleGrant}
        confirmLoading={granting}
        okText={granting ? '处理中...' : '加赠乐币'}
        cancelText="取消"
        onCancel={() => setGrantVisible(false)}
        destroyOnClose
      >
        <Form form={grantForm} layout="vertical">
          <Form.Item label="加赠数量（点，1点=1元）" name="amount" rules={[{ required: true, message: '请输入加赠数量' }]}>
            <InputNumber min={0.01} precision={2} style={{ width: '100%' }} placeholder="如 10" />
          </Form.Item>
          <Form.Item label="加赠原因" name="reason" rules={[{ required: true, message: '请输入加赠原因' }]}>
            <Input.TextArea rows={3} maxLength={200} placeholder="用于台账留痕，如：客服补偿" />
          </Form.Item>
          <Alert type="info" showIcon message="加赠将新增一条乐币批次，有效期按全局规则（默认获取后 2 年，次月首日到期）。" />
        </Form>
      </Modal>
    </div>
  )
}
