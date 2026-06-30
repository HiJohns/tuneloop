import { useEffect, useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Card, Button, Form, Input, Select, Alert, Space, message } from 'antd'
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

export default function StaffEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const [userForm] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [editingUser, setEditingUser] = useState(location.state?.user || null)
  const [siteOptions, setSiteOptions] = useState([])

  useEffect(() => {
    if (!editingUser) {
      message.warning('无法获取用户数据，请返回列表重试')
      navigate('/staff')
      return
    }
    userForm.setFieldsValue({
      name: editingUser.name,
      email: editingUser.email,
      phone: editingUser.phone,
      site_id: editingUser.site_id,
      role: editingUser.role,
    })
    sitesApi.getTree().then(res => {
      if (res.code === 20000) {
        setSiteOptions(convertSitesToOptions(res.data?.list || []))
      }
    }).catch(() => {})
  }, [id])

  const handleSubmit = async (values) => {
    setLoading(true)
    try {
      const emailChanged = editingUser.email && values.email && values.email !== editingUser.email
      const result = await staffApi.updateUser(id, values)
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
        navigate('/staff')
      }
    } catch (error) {
      message.error('更新用户失败: ' + error.message)
    }
    setLoading(false)
  }

  if (!editingUser) return null

  return (
    <div className="p-6">
      <Card title="编辑用户" extra={<Button onClick={() => navigate('/staff')}>返回列表</Button>}>
        <Form form={userForm} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="email" label="邮箱" rules={[{ type: 'email', message: '请输入有效的邮箱地址' }, { required: true, message: '请输入邮箱' }]}>
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          {editingUser?.email && (
            <Alert message="修改邮箱后，系统将发送确认邮件到新邮箱地址，需确认后方可生效。" type="info" showIcon style={{ marginBottom: 16 }} />
          )}
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input placeholder="请输入手机号" />
          </Form.Item>
          <Form.Item name="site_id" label="归属网点" rules={[{ required: true, message: '请选择归属网点' }]}>
            <Select placeholder="请选择归属网点" style={{ width: '100%' }} dropdownStyle={{ maxHeight: 300, overflow: 'auto' }}>
              {siteOptions.map(option => (
                <Select.Option key={option.key} value={option.value}>{option.label}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="role" label="角色">
            <Select>
                    <Select.Option value="site_admin">管理员</Select.Option>
                    <Select.Option value="site_member">成员</Select.Option>
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
