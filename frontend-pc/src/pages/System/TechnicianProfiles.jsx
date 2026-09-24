import { useState, useEffect } from 'react'
import { Card, Table, Tag, Button, Space, Modal, Form, Input, InputNumber, Select, Upload, Image, message, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, UploadOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import 'react-quill/dist/quill.snow.css'
import { technicianApi, staffApi } from '../../services/api'
import { formatBeijingDateTimeShort } from '../../utils/date'
// #1974 T4 师傅档案维护页（直属商户）：照片/介绍/专长年限 + 新增/编辑/停用
// #2049：照片改走媒体管线端点 `/technician-profiles/:id/photo`（原图+缩略图）；
//        简介 bio 为富文本（react-quill，前端渲染 RichContent/富文本）。
// 注意：照片上传必须走 api.uploadFile / technicianApi.uploadPhoto（FormData），不可用 api.post（#1967 教训）

function parseExperience(v) {
  if (!v) return []
  if (Array.isArray(v)) return v
  try { return JSON.parse(v) } catch { return [] }
}

export default function TechnicianProfiles() {
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(true)
  const [users, setUsers] = useState([])
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增
  const [submitting, setSubmitting] = useState(false)
  const [photoUrl, setPhotoUrl] = useState('')
  const [pendingPhoto, setPendingPhoto] = useState(null) // #2049 待上传文件（保存时走媒体管线）
  const [form] = Form.useForm()

  const fetchList = async () => {
    setLoading(true)
    try {
      const resp = await technicianApi.list()
      if (resp.code === 20000) setList(resp.data?.list || [])
      else message.error(resp.message || '加载失败')
    } finally { setLoading(false) }
  }

  useEffect(() => { fetchList() }, [])

  const openCreate = async () => {
    setEditing(null)
    setPhotoUrl('')
    setPendingPhoto(null)
    form.resetFields()
    form.setFieldsValue({ experience: [{ craft: '', years: undefined }] })
    setModalOpen(true)
    // 可选：拉取员工列表（取姓名用于选择关联师傅）
    try {
      const resp = await staffApi.list({ page: 1, pageSize: 200 })
      if (resp.code === 20000) setUsers(resp.data?.list || [])
    } catch { /* 非阻塞：可手填 user_id */ }
  }

  const openEdit = (row) => {
    setEditing(row)
    setPhotoUrl(row.photo || '')
    setPendingPhoto(null)
    form.setFieldsValue({
      user_id: row.user_id,
      bio: row.bio,
      experience: parseExperience(row.experience).length ? parseExperience(row.experience) : [{ craft: '', years: undefined }],
    })
    setModalOpen(true)
  }

  // #2049：暂存所选文件（本地预览），保存时经 `/technician-profiles/:id/photo` 走媒体管线
  const uploadPhoto = async (file) => {
    setPendingPhoto(file)
    setPhotoUrl(URL.createObjectURL(file))
    return false
  }

  const submit = async () => {
    const v = await form.validateFields()
    const experience = (v.experience || [])
      .filter(e => e && e.craft)
      .map(e => ({ craft: e.craft, years: Number(e.years) || 0 }))
    setSubmitting(true)
    // #2049：先保存档案（bio/experience），再经媒体管线上传照片（需 profile id）
    const savePhoto = async (id) => {
      if (!pendingPhoto || !id) return
      const fd = new FormData()
      fd.append('file', pendingPhoto)
      const up = await technicianApi.uploadPhoto(id, fd)
      if (up.code !== 20000) message.error(up.message || '照片上传失败')
    }
    try {
      let resp
      if (editing) {
        resp = await technicianApi.update(editing.id, { bio: v.bio || '', experience })
        if (resp.code === 20000) await savePhoto(editing.id)
      } else {
        resp = await technicianApi.create({ user_id: v.user_id, bio: v.bio || '', experience })
        if (resp.code === 20000) await savePhoto(resp.data?.id)
      }
      if (resp.code === 20000) {
        message.success(editing ? '已更新' : '已新增')
        setModalOpen(false)
        setPendingPhoto(null)
        fetchList()
      } else message.error(resp.message || '保存失败')
    } catch (e) { message.error(e.message || '保存失败') }
    setSubmitting(false)
  }

  const toggleStatus = async (row) => {
    const next = row.status === 'active' ? 'inactive' : 'active'
    const resp = await technicianApi.setStatus(row.id, next)
    if (resp.code === 20000) { message.success(next === 'active' ? '已启用' : '已停用'); fetchList() }
    else message.error(resp.message || '操作失败')
  }

  const columns = [
    {
      title: '照片', dataIndex: 'photo_thumb', width: 80,
      render: (v, r) => (v || r.photo) ? <Image src={v || r.photo} width={48} height={48} style={{ objectFit: 'cover', borderRadius: 6 }} /> : '-',
    },
    { title: '姓名', dataIndex: 'name', width: 120, render: (v, r) => v || r.user_id?.slice(0, 8) || '-' },
    {
      title: '经验', dataIndex: 'experience', render: v => {
        const exp = parseExperience(v)
        return exp.length ? exp.map(e => `${e.craft} ${e.years} 年`).join(' · ') : '-'
      },
    },
    { title: '介绍', dataIndex: 'bio', ellipsis: true, render: v => v || '-' },
    {
      title: '状态', dataIndex: 'status', width: 90,
      render: v => <Tag color={v === 'active' ? 'green' : 'default'}>{v === 'active' ? '启用' : '停用'}</Tag>,
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 170, render: v => v ? formatBeijingDateTimeShort(v) : '-' },
    {
      title: '操作', width: 160,
      render: (_, r) => (
        <Space>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>编辑</Button>
          <Popconfirm title={r.status === 'active' ? '停用后顾客列表不再出现，确认？' : '确认启用？'} onConfirm={() => toggleStatus(r)}>
            <Button type="link" size="small" danger={r.status === 'active'}>{r.status === 'active' ? '停用' : '启用'}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title="师傅档案"
      extra={<Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增师傅</Button>}
    >
      <Table rowKey="id" loading={loading} dataSource={list} columns={columns} />

      <Modal
        title={editing ? '编辑师傅档案' : '新增师傅档案'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={submit}
        confirmLoading={submitting}
        width={640}
        okText="保存"
      >
        <Form form={form} layout="vertical">
          {!editing && (
            <Form.Item name="user_id" label="关联师傅（用户）" rules={[{ required: true, message: '请选择或填写师傅 user_id' }]}>
              {users.length > 0 ? (
                <Select
                  showSearch
                  placeholder="选择员工"
                  optionFilterProp="label"
                  options={users.map(u => ({ value: u.id, label: `${u.name || u.username || u.id.slice(0, 8)}（${u.id.slice(0, 8)}）` }))}
                />
              ) : (
                <Input placeholder="师傅 user_id（UUID）" />
              )}
            </Form.Item>
          )}
          <Form.Item label="个人照片">
            <Space>
              {photoUrl ? <Image src={photoUrl} width={72} height={72} style={{ objectFit: 'cover', borderRadius: 6 }} /> : null}
              <Upload beforeUpload={uploadPhoto} showUploadList={false} accept="image/*">
                <Button icon={<UploadOutlined />}>上传照片</Button>
              </Upload>
            </Space>
          </Form.Item>
          <Form.Item name="bio" label="详细介绍">
            <ReactQuill theme="snow" style={{ background: '#fff' }} placeholder="如：钢琴维修 12 年 · 小提琴维修 8 年，擅长音色调整" />
          </Form.Item>
          <Form.Item label="专长与年限">
            <Form.List name="experience">
              {(fields, { add, remove }) => (
                <>
                  {fields.map(({ key, name, ...rest }) => (
                    <Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
                      <Form.Item {...rest} name={[name, 'craft']} rules={[{ required: true, message: '填写专长' }]} style={{ marginBottom: 0 }}>
                        <Input placeholder="专长（如 钢琴）" style={{ width: 200 }} />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, 'years']} style={{ marginBottom: 0 }}>
                        <InputNumber placeholder="年限" min={0} max={80} style={{ width: 100 }} />
                      </Form.Item>
                      <Button type="link" danger onClick={() => remove(name)}>删除</Button>
                    </Space>
                  ))}
                  <Button type="dashed" onClick={() => add({ craft: '', years: undefined })} icon={<PlusOutlined />}>添加专长</Button>
                </>
              )}
            </Form.List>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
