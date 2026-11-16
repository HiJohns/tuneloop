import { useState, useEffect } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Card, Button, Form, Input, Radio, Checkbox, Space, message, Modal, Descriptions, Alert } from 'antd'
import { staffApi, api } from '../services/api'

export default function StaffResetPassword() {
  const { id } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [user, setUser] = useState(location.state?.user || null)
  const [loaded, setLoaded] = useState(!!location.state?.user)

  // #2389 Rev.3：无 state 时尝试取用户 email；取不到 → 保守禁用（不跳转、不发重置请求）。
  useEffect(() => {
    if (user) return
    let cancelled = false
    api.get(`/admin/user-management/${id}`)
      .then(resp => {
        if (!cancelled && resp?.code === 20000 && resp.data) setUser(resp.data)
      })
      .catch(() => {})
      .finally(() => { if (!cancelled) setLoaded(true) })
    return () => { cancelled = true }
  }, [id, user])

  // #2389 Rev.3：无邮箱/未知用户一律禁止走邮件重置（直接访问 URL 也兜底）。
  const noEmail = !user || !String(user.email || '').trim()

  const handleSubmit = async () => {
    if (noEmail) {
      Modal.warning({
        title: '该账号未配置邮箱',
        content: '无法通过邮件重置密码。请使用「直接设置密码」功能（对接中）；在此之前可由管理员在成员详情中重置。',
        okText: '知道了',
      })
      return
    }
    setLoading(true)
    try {
      const values = form.getFieldsValue()
      const result = await staffApi.resetPassword([id], window.location.origin)
      if (result.code === 20000) {
        const { sent, skipped } = result.data
        if (!autoGenerate) {
          message.success(`密码已重置。重设密码邮件已发送 ${sent} 封${skipped > 0 ? `，${skipped} 个用户被跳过` : ''}`)
        } else {
          message.success(`密码已重置。重设密码邮件已发送 ${sent} 封${skipped > 0 ? `，${skipped} 个用户被跳过` : ''}`)
        }
        navigate('/staff')
      } else {
        message.error(result.message || '重置密码失败')
      }
    } catch (err) {
      message.error('重置密码失败: ' + err.message)
    }
    setLoading(false)
  }

  if (!user && !loaded) return null

  return (
    <div className="p-6">
      <Card title="重置密码" extra={<Button onClick={() => navigate('/staff')}>返回列表</Button>}>
        <Descriptions column={1} style={{ marginBottom: 24 }}>
          <Descriptions.Item label="用户">{user?.name || '-'}</Descriptions.Item>
          <Descriptions.Item label="邮箱">{user?.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="手机号">{user?.phone || '-'}</Descriptions.Item>
        </Descriptions>

        {noEmail && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="该账号未配置邮箱"
            description="无法通过邮件重置密码。请使用「直接设置密码」功能（对接中）；在此之前可由管理员在成员详情中重置。"
          />
        )}

        <Form form={form} layout="vertical" disabled={noEmail}>
          <Form.Item name="auto_generate" label="密码设置" initialValue={true}>
            <Radio.Group onChange={e => setAutoGenerate(e.target.value)}>
              <Radio value={true}>自动生成</Radio>
              <Radio value={false}>手动设置</Radio>
            </Radio.Group>
          </Form.Item>
          {!autoGenerate && (
            <Form.Item name="password" label="密码" rules={[{ required: true, message: '请设置密码' }]}>
              <Input.Password placeholder="8位+大写+小写+数字" />
            </Form.Item>
          )}
          <Form.Item name="force_password_change" valuePropName="checked" initialValue={true}>
            <Checkbox>首次登录时强制修改密码</Checkbox>
          </Form.Item>
        </Form>

        <Space>
          <Button type="primary" onClick={handleSubmit} loading={loading} disabled={noEmail}>确认重置</Button>
          <Button onClick={() => navigate('/staff')}>取消</Button>
        </Space>
      </Card>
    </div>
  )
}
