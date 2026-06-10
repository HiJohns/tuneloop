import { useState, useEffect } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Card, Button, Form, Input, Radio, Checkbox, Space, message, Descriptions } from 'antd'
import { staffApi } from '../services/api'

export default function StaffResetPassword() {
  const { id } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [user, setUser] = useState(location.state?.user || null)

  useEffect(() => {
    if (!user) {
      message.warning('无法获取用户数据，请返回列表重试')
      navigate('/staff')
    }
  }, [])

  const handleSubmit = async () => {
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

  if (!user) return null

  return (
    <div className="p-6">
      <Card title="重置密码" extra={<Button onClick={() => navigate('/staff')}>返回列表</Button>}>
        <Descriptions column={1} style={{ marginBottom: 24 }}>
          <Descriptions.Item label="用户">{user.name}</Descriptions.Item>
          <Descriptions.Item label="邮箱">{user.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="手机号">{user.phone || '-'}</Descriptions.Item>
        </Descriptions>

        <Form form={form} layout="vertical">
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
          <Button type="primary" onClick={handleSubmit} loading={loading}>确认重置</Button>
          <Button onClick={() => navigate('/staff')}>取消</Button>
        </Space>
      </Card>
    </div>
  )
}
