import { useEffect, useState, useRef, useCallback } from 'react'
import { Form, Input, Select, Button, Space, Alert, Switch, Radio, Modal } from 'antd'
import { staffApi } from '../services/api'

const { Option } = Select

export default function UserCreateDialog({ form, onSubmit, onCancel, siteOptions }) {
  const [fieldErrors, setFieldErrors] = useState({})
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [initialPassword, setInitialPassword] = useState('')
  const debounceRef = useRef({})

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

  const doCheckField = useCallback(async (field, value) => {
    if (!value || value.length < 2) return
    try {
      const response = await staffApi.checkUserExists(
        field === 'phone' ? value : '',
        field === 'email' ? value : '',
        field === 'username' ? value : ''
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
  }, [])

  const handleFieldChange = useCallback((field, value) => {
    if (debounceRef.current[field]) clearTimeout(debounceRef.current[field])
    debounceRef.current[field] = setTimeout(() => doCheckField(field, value), 500)
  }, [doCheckField])

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
        <Input
          placeholder="请输入用户名（英文数字下划线）"
          onChange={(e) => handleFieldChange('username', e.target.value)}
        />
      </Form.Item>
      {fieldErrors.username?.conflict && (
        <Alert
          type="warning"
          message={`用户名已被 ${fieldErrors.username.users.map(u => u.name).join('、')} 使用`}
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Form.Item
        name="email"
        label="邮箱（选填）"
        rules={[
          { type: 'email', message: '请输入有效的邮箱地址' }
        ]}
        extra="配置邮箱后可支持密码重置"
      >
        <Input
          placeholder="请输入邮箱"
          onChange={(e) => handleFieldChange('email', e.target.value)}
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
          onChange={(e) => handleFieldChange('phone', e.target.value)}
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

      <div style={{ marginBottom: 16 }}>
        <div style={{ marginBottom: 8, fontWeight: 500 }}>密码设置</div>
        <Form.Item name="auto_generate" initialValue={true}>
          <Radio.Group onChange={e => setAutoGenerate(e.target.value)}>
            <Radio value={true}>自动生成密码</Radio>
            <Radio value={false}>手动设置密码</Radio>
          </Radio.Group>
        </Form.Item>
        {!autoGenerate && (
          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '密码长度不能少于 8 位' },
              { pattern: /[A-Z]/, message: '必须包含大写字母' },
              { pattern: /[a-z]/, message: '必须包含小写字母' },
              { pattern: /[0-9]/, message: '必须包含数字' },
            ]}
          >
            <Input.Password placeholder="8位以上 + 大写 + 小写 + 数字" />
          </Form.Item>
        )}
        <Form.Item name="force_password_change" valuePropName="checked" initialValue={true}>
          <Switch /> 首次登录强制修改密码
        </Form.Item>
      </div>

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
        name="role"
        label="角色"
        initialValue="site_member"
        rules={[{ required: true, message: '请选择角色' }]}
      >
        <Select placeholder="请选择角色">
                    <Option value="site_admin">网点管理员</Option>
                    <Option value="site_member">网点员工</Option>
                    <Option value="repair_technician">维修师傅</Option>
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
