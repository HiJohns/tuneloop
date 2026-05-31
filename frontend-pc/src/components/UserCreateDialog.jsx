import { useEffect, useState } from 'react'
import { Form, Input, Select, Button, Space, Alert } from 'antd'
import { staffApi } from '../services/api'

const { Option } = Select

export default function UserCreateDialog({ form, onSubmit, onCancel, siteOptions }) {
  const [fieldErrors, setFieldErrors] = useState({})

  useEffect(() => {
    staffApi.getMe().then(res => {
      if (res.code === 20000 && res.data && res.data.site_id) {
        const role = (res.data.role || '').toLowerCase()
        const businessRole = (res.data.business_role || '').toLowerCase()
        if (businessRole === 'site_admin' || businessRole === 'site_member' ||
            role === 'site_admin' || role === 'site_member') {
          form.setFieldsValue({ site_id: res.data.site_id })
        }
      }
    }).catch(() => {})
  }, [form])

  const checkFieldUnique = async (field, value) => {
    if (!value) return
    try {
      const response = await staffApi.checkUserExists(
        field === 'phone' ? value : '',
        field === 'email' ? value : ''
      )
      if (response.code === 20000 && response.data?.exists) {
        setFieldErrors(prev => ({ ...prev, [field]: { conflict: true, users: response.data.users || [] } }))
      } else {
        setFieldErrors(prev => {
          const next = { ...prev }
          delete next[field]
          return next
        })
      }
    } catch (error) {
      console.error('Uniqueness check failed:', error)
    }
  }

  return (
    <Form
      form={form}
      layout="vertical"
      onFinish={onSubmit}
    >
      <Form.Item
        name="name"
        label="姓名"
        rules={[{ required: true, message: '请输入姓名' }]}
      >
        <Input placeholder="请输入姓名" />
      </Form.Item>

      <Form.Item
        name="username"
        label="用户名"
        rules={[
          { pattern: /^[a-zA-Z0-9_]+$/, message: '用户名只能包含英文、数字和下划线' },
          { required: true, message: '请输入用户名' }
        ]}
      >
        <Input placeholder="请输入用户名（英文数字下划线）" />
      </Form.Item>

      <Form.Item
        name="email"
        label="邮箱"
        rules={[
          { type: 'email', message: '请输入有效的邮箱地址' },
          { required: true, message: '请输入邮箱' }
        ]}
      >
        <Input
          placeholder="请输入邮箱"
          onBlur={(e) => checkFieldUnique('email', e.target.value)}
        />
      </Form.Item>
      {fieldErrors.email?.conflict && (
        <Alert
          type="warning"
          message={`邮箱已被 ${fieldErrors.email.users.map(u => u.name).join('、')} 使用`}
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Form.Item
        name="phone"
        label="手机号"
        rules={[{ required: true, message: '请输入手机号' }]}
      >
        <Input
          placeholder="请输入手机号"
          onBlur={(e) => checkFieldUnique('phone', e.target.value)}
        />
      </Form.Item>
      {fieldErrors.phone?.conflict && (
        <Alert
          type="warning"
          message={`手机号已被 ${fieldErrors.phone.users.map(u => u.name).join('、')} 使用`}
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Form.Item
        name="site_id"
        label="归属网点"
        rules={[{ required: true, message: '请选择归属网点' }]}
      >
        <Select 
          placeholder="请选择归属网点"
          style={{ width: '100%' }}
          dropdownStyle={{ maxHeight: 300, overflow: 'auto' }}
        >
          {siteOptions.map(option => (
            <Option key={option.key} value={option.value}>
              {option.label}
            </Option>
          ))}
        </Select>
      </Form.Item>

      <Form.Item
        name="user_type"
        label="用户类型"
        initialValue="员工"
        rules={[{ required: true, message: '请选择用户类型' }]}
      >
        <Select placeholder="请选择用户类型">
          <Option value="员工">员工</Option>
          <Option value="维修技师">维修技师</Option>
        </Select>
      </Form.Item>

      <Form.Item>
        <Space>
          <Button type="primary" htmlType="submit">
            创建
          </Button>
          <Button onClick={onCancel}>
            取消
          </Button>
        </Space>
      </Form.Item>
    </Form>
  )
}