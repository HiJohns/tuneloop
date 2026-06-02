import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Card, Form, Input, Button, message, Alert } from 'antd'
import { KeyOutlined } from '@ant-design/icons'
import api from '../services/api'

export default function ChangePassword() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const isFirstLogin = searchParams.get('first_login') === '1'
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (values) => {
    if (values.new_password !== values.confirm_password) {
      message.error('两次输入的密码不一致')
      return
    }

    setLoading(true)
    try {
      const resp = await api.post('/user/change-password', { new_password: values.new_password })
      if (resp.code === 20000) {
        message.success('密码修改成功')
        if (isFirstLogin) {
          navigate('/')
        } else {
          navigate(-1)
        }
      } else {
        message.error(resp.message || '修改失败')
      }
    } catch (err) {
      message.error(err.response?.data?.message || '修改失败，请稍后重试')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      padding: isFirstLogin ? 0 : 24,
      maxWidth: 480,
      margin: '0 auto',
      display: 'flex',
      alignItems: 'center',
      minHeight: isFirstLogin ? '100vh' : 'auto',
      background: isFirstLogin ? '#f0f2f5' : 'transparent',
    }}>
      <Card style={{ width: '100%' }}>
        {isFirstLogin && (
          <Alert
            message="首次登录需修改密码"
            description="系统要求您在首次登录时设置新密码，完成后即可正常使用系统功能。"
            type="warning"
            showIcon
            style={{ marginBottom: 24 }}
          />
        )}
        <h2 style={{ marginBottom: 24, textAlign: 'center' }}>
          <KeyOutlined /> 修改密码
        </h2>
        <Form layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="new_password"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '密码长度不能少于 8 位' },
              { pattern: /[A-Z]/, message: '必须包含大写字母' },
              { pattern: /[a-z]/, message: '必须包含小写字母' },
              { pattern: /[0-9]/, message: '必须包含数字' },
            ]}
          >
            <Input.Password placeholder="8位以上 + 大写 + 小写 + 数字" />
          </Form.Item>

          <Form.Item
            name="confirm_password"
            label="确认新密码"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('new_password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password placeholder="再次输入新密码" />
          </Form.Item>

          <div style={{ fontSize: 12, color: '#999', marginBottom: 16 }}>
            密码要求：8 位以上，必须包含大写字母、小写字母和数字。
          </div>

          <Button type="primary" htmlType="submit" block loading={loading}>
            {isFirstLogin ? '确认修改' : '修改密码'}
          </Button>

          {!isFirstLogin && (
            <Button style={{ marginTop: 12 }} block onClick={() => navigate(-1)}>
              取消
            </Button>
          )}
        </Form>
      </Card>
    </div>
  )
}
