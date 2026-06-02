import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Button, Descriptions, Tag, message, Modal, Spin } from 'antd'
import { UserOutlined, SafetyOutlined, BankOutlined, KeyOutlined } from '@ant-design/icons'
import api from '../services/api'

export default function UserProfile() {
  const navigate = useNavigate()
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(false)
  const [resetting, setResetting] = useState(false)

  useEffect(() => {
    fetchProfile()
  }, [])

  const fetchProfile = async () => {
    setLoading(true)
    try {
      const resp = await api.get('/users/me')
      if (resp.code === 20000) {
        setUser(resp.data)
      }
    } catch (error) {
      message.error('获取用户信息失败')
    } finally {
      setLoading(false)
    }
  }

  const handleResetPassword = () => {
    const email = user?.email || user?.id
    const maskedEmail = email && email.includes('@')
      ? email[0] + '***@' + email.split('@')[1]
      : '注册邮箱'

    Modal.confirm({
      title: '确认重置密码',
      content: `系统将向您的邮箱 ${maskedEmail} 发送密码重置邮件，邮件中的链接 24 小时内有效。`,
      okText: '确认发送',
      cancelText: '取消',
      onOk: async () => {
        setResetting(true)
        try {
          const resp = await api.post('/user/reset-password')
          if (resp.code === 20000) {
            message.success(resp.message || `密码重置邮件已发送至 ${resp.data?.email_masked || maskedEmail}，请查收`)
          } else if (resp.code === 42900) {
            message.warning(resp.message || '操作过于频繁，请 30 分钟后再试')
          } else if (resp.code === 40001) {
            message.error(resp.message || '您的账户未绑定邮箱，请联系管理员')
          } else {
            message.error(resp.message || '操作失败')
          }
        } catch (error) {
          const resp = error.response?.data
          if (resp?.code === 42900) {
            message.warning(resp.message || '操作过于频繁，请 30 分钟后再试')
          } else {
            message.error(resp?.message || '邮件发送失败，请稍后重试')
          }
        } finally {
          setResetting(false)
        }
      },
    })
  }

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div style={{ padding: 24, maxWidth: 720, margin: '0 auto' }}>
      <h2 style={{ marginBottom: 24 }}>个人中心</h2>

      <Card
        title={<span><UserOutlined /> 基本信息</span>}
        style={{ marginBottom: 16 }}
      >
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="用户名">{user?.username || '-'}</Descriptions.Item>
          <Descriptions.Item label="姓名">{user?.name || '-'}</Descriptions.Item>
          <Descriptions.Item label="邮箱">{user?.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="角色">
            <Tag color="blue">{user?.business_role || user?.role || '-'}</Tag>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title={<span><SafetyOutlined /> 账户安全</span>}
        style={{ marginBottom: 16 }}
      >
        <Button
          type="primary"
          icon={<KeyOutlined />}
          onClick={handleResetPassword}
          loading={resetting}
        >
          通过邮件重置密码
        </Button>
        <div style={{ marginTop: 8, fontSize: 12, color: '#999' }}>
          系统将向您的注册邮箱发送密码重置邮件，点击邮件中的链接设置新密码。
        </div>
      </Card>

      <Card
        title={<span><BankOutlined /> 关联信息</span>}
      >
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="关联网点">{user?.site_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="网点 ID">{user?.site_id || '-'}</Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  )
}
