import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Card, Button, Form, Input, Select, AutoComplete, Alert, Space, message } from 'antd'
import { staffApi, sitesApi } from '../services/api'

function convertSitesToOptions(sites, parentPath = '') {
  let options = []
  sites.forEach(site => {
    const path = parentPath ? `${parentPath} / ${site.name}` : site.name
    options.push({ label: path, value: site.id, key: site.id })
    if (site.children && site.children.length > 0) {
      options = options.concat(convertSitesToOptions(site.children, path))
    }
  })
  return options
}

// #2429：职位 = 自由文本（ui.md「职位列取 position，缺失回退 role」），
// 不做封闭枚举；给出常用项建议 + 允许自由输入。
const POSITION_OPTIONS = [
  { value: '客服' }, { value: '运营' }, { value: '教务' },
  { value: '调律' }, { value: '维修' }, { value: '前台' },
]

export default function StaffEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const [userForm] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [fetching, setFetching] = useState(true)
  // 先用列表带来的 state 立即渲染，随后按 id 拉取权威数据（刷新/直达也能加载）
  const [editingUser, setEditingUser] = useState(location.state?.user || null)
  const [siteOptions, setSiteOptions] = useState([])
  const [loadError, setLoadError] = useState('')

  const isTechnician = useMemo(() => {
    if (!editingUser) return false
    return !!editingUser.is_technician || editingUser.role === 'repair_technician'
  }, [editingUser])

  // 直属员工：有 merchant_members 且无 site_members（后端 is_direct_staff）
  const isDirectStaff = useMemo(() => {
    if (!editingUser) return false
    if (typeof editingUser.is_direct_staff === 'boolean') return editingUser.is_direct_staff
    return !editingUser.site_id
  }, [editingUser])

  useEffect(() => {
    let cancelled = false
    setFetching(true)
    staffApi.getUser(id)
      .then((res) => {
        if (cancelled) return
        if (res.code === 20000 && res.data) {
          setEditingUser(res.data)
          setLoadError('')
        } else {
          setLoadError(res.message || '无法获取用户数据')
        }
      })
      .catch((err) => {
        if (!cancelled) setLoadError(err?.message || '无法获取用户数据')
      })
      .finally(() => { if (!cancelled) setFetching(false) })

    sitesApi.getTree().then(res => {
      if (res.code === 20000) setSiteOptions(convertSitesToOptions(res.data?.list || []))
    }).catch(() => {})

    return () => { cancelled = true }
  }, [id])

  useEffect(() => {
    if (!editingUser) return
    userForm.setFieldsValue({
      name: editingUser.name,
      email: editingUser.email,
      phone: editingUser.phone,
      position: editingUser.position,
      site_id: editingUser.site_id || undefined,
      role: editingUser.role,
    })
  }, [editingUser, userForm])

  const handleSubmit = async (values) => {
    setLoading(true)
    try {
      const emailChanged = editingUser.email && values.email && values.email !== editingUser.email
      const payload = { ...values }
      // #2429：站点未变化时不回传 site_id —— 避免后端误判为「换网点」并触发 IAM 重绑
      if (!editingUser.site_id || values.site_id === editingUser.site_id) {
        delete payload.site_id
      }
      const result = await staffApi.updateUser(id, payload)
      if (result.code === 20000) {
        if (emailChanged) {
          try {
            await staffApi.updateIAMUser(editingUser.iam_sub || id, {
              name: values.name,
              email: values.email,
              phone: values.phone,
            })
            message.success('用户更新成功，邮箱变更需确认后生效')
          } catch {
            message.warning('用户更新成功，但邮箱变更请求发送失败')
          }
        } else {
          message.success('更新用户成功')
        }
        if (result.data?.role_errors?.length > 0) {
          message.warning('已保存，但部分角色权限同步失败，可在成员详情中重新分配')
        }
        navigate('/staff')
      } else {
        message.error(result.message || '更新用户失败')
      }
    } catch (error) {
      message.error('更新用户失败: ' + error.message)
    }
    setLoading(false)
  }

  if (loadError) {
    return (
      <div className="p-6">
        <Card title="编辑用户">
          <Alert type="error" showIcon message={loadError} description="请返回列表重试；若持续失败请联系管理员。" />
          <Space style={{ marginTop: 16 }}>
            <Button type="primary" onClick={() => navigate('/staff')}>返回列表</Button>
          </Space>
        </Card>
      </div>
    )
  }

  if (!editingUser) {
    return (
      <div className="p-6">
        <Card title="编辑用户" loading={fetching}>
          <div style={{ height: 120 }} />
        </Card>
      </div>
    )
  }

  return (
    <div className="p-6">
      <Card title="编辑用户" extra={<Button onClick={() => navigate('/staff')}>返回列表</Button>}>
        <Form form={userForm} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="email" label="邮箱" rules={[{ type: 'email', message: '请输入有效的邮箱地址' }]}>
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          {editingUser?.email && (
            <Alert message="修改邮箱后，系统将发送确认邮件到新邮箱地址，需确认后方可生效。" type="info" showIcon style={{ marginBottom: 16 }} />
          )}
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input placeholder="请输入手机号" />
          </Form.Item>

          {/* #2429：维修师傅无「职位」字段（ui.md L2565：师傅职位即「维修师傅」） */}
          {!isTechnician && (
            <Form.Item name="position" label="职位" extra="可自由填写；常用项仅供参考（如 客服 / 运营 / 调律）">
              <AutoComplete
                options={POSITION_OPTIONS}
                placeholder="请输入或选择职位"
                filterOption={(input, option) => (option?.value || '').includes(input)}
              />
            </Form.Item>
          )}

          {/* #2429：直属员工不显示「归属网点」必填（否则为保存被迫选网点 → IAM 误绑） */}
          {isDirectStaff ? (
            <Form.Item label="归属">
              <Input
                disabled
                value={editingUser.merchant_role ? `直属商户（${editingUser.merchant_role}）` : '直属商户'}
              />
            </Form.Item>
          ) : (
            <Form.Item name="site_id" label="归属网点" rules={[{ required: true, message: '请选择归属网点' }]}>
              <Select placeholder="请选择归属网点" style={{ width: '100%' }} dropdownStyle={{ maxHeight: 300, overflow: 'auto' }}>
                {siteOptions.map(option => (
                  <Select.Option key={option.key} value={option.value}>{option.label}</Select.Option>
                ))}
              </Select>
            </Form.Item>
          )}

          <Form.Item name="role" label="角色" extra="仅支持：管理员 / 成员 / 商户直属员工 / 维修师傅">
            <Select allowClear>
              <Select.Option value="site_admin">管理员</Select.Option>
              <Select.Option value="site_member">成员</Select.Option>
              <Select.Option value="merchant_staff">商户直属员工</Select.Option>
              <Select.Option value="repair_technician">维修师傅</Select.Option>
            </Select>
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>保存</Button>
            <Button onClick={() => navigate('/staff')}>取消</Button>
          </Space>
        </Form>
      </Card>
    </div>
  )
}
