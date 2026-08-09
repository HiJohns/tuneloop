import { useState, useEffect } from 'react'
import { Table, Input, Button, Space, Modal, Form, InputNumber, Switch, message, Tag, Typography } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { api } from '../../services/api'
import IdPhotoDisplay from '../../components/IdPhotoDisplay'

const { Text } = Typography

export default function UserManagement() {
  const [list, setList] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [detailVisible, setDetailVisible] = useState(false)
  const [current, setCurrent] = useState(null)
  const [idPhotos, setIdPhotos] = useState({ front: '', back: '' })
  const [form] = Form.useForm()

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

  const openDetail = (record) => {
    setCurrent(record)
    form.setFieldsValue({
      membership_level_id: record.membership_level_id,
      promo_points: record.points,
      status: record.status === 'active',
    })
    setIdPhotos({ front: '', back: '' })
    setDetailVisible(true)
    // Fetch full detail (includes id_photo URLs) for the ID photo section (#1599)
    api.get(`/admin/user-management/${record.id}`).then((resp) => {
      if (resp?.code === 20000 && resp.data) {
        setIdPhotos({
          front: resp.data.id_photo_front || '',
          back: resp.data.id_photo_back || '',
        })
      }
    }).catch(() => {})
  }

  const handleSave = async () => {
    const values = await form.validateFields()
    try {
      const resp = await api.put(`/admin/user-management/${current.id}`, {
        membership_level_id: values.membership_level_id,
        promo_points: values.promo_points,
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

  const columns = [
    { title: '注册名', dataIndex: 'username', key: 'username' },
    { title: '微信号', dataIndex: 'wx_openid', key: 'wx_openid', render: v => v ? <Text ellipsis style={{ maxWidth: 120 }}>{v}</Text> : '-' },
    { title: '电话', dataIndex: 'phone', key: 'phone', render: v => v || '-' },
    { title: '当前等级', dataIndex: 'level', key: 'level', render: v => v || '-' },
    { title: '当前积分', dataIndex: 'points', key: 'points' },
    { title: '注册时间', dataIndex: 'registered_at', key: 'registered_at', render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '最新活动', dataIndex: 'last_active', key: 'last_active', render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: v => v === 'disabled'
      ? <Tag color="red">已禁用</Tag>
      : v === 'active'
        ? <Tag color="green">可用</Tag>
        : <Tag>{v || '未知'}</Tag> },
    { title: '操作', key: 'action', render: (_, record) => (
      <Button type="link" size="small" onClick={() => openDetail(record)}>详情 / 编辑</Button>
    )},
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Input.Search
          placeholder="搜索 用户名/电话/微信号"
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
      >
        <Form form={form} layout="vertical">
          <Form.Item label="当前等级" name="membership_level_id">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="等级 ID（见会员级别管理）" />
          </Form.Item>
          <Form.Item label="当前积分" name="promo_points">
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
        </Form>
      </Modal>
    </div>
  )
}
