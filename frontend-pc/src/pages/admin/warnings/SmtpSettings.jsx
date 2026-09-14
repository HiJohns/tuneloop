import { useState, useEffect } from 'react'
import { Card, Form, Input, InputNumber, Switch, Button, message, Spin, Alert, Space } from 'antd'
import { api } from '../../../services/api'

// #1910: 邮件服务配置（系统管理员）。界面配置（阿里云邮件推送）优先，
// 未启用或发送失败自动回落到 .env 的个人邮箱；密码加密存储、不回显。
export default function SmtpSettings() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [meta, setMeta] = useState({ source: 'none', password_set: false })
  const [testing, setTesting] = useState(false)
  const [testTo, setTestTo] = useState('')

  const applyData = (d) => {
    setMeta({ source: d?.source || 'none', password_set: !!d?.password_set })
    form.setFieldsValue({
      host: d?.host || '',
      port: d?.port || 587,
      username: d?.username || '',
      from: d?.from || '',
      use_tls: d?.use_tls !== false,
      enabled: !!d?.enabled,
      password: '',
    })
  }

  const fetchConfig = async () => {
    try {
      const res = await api.get('/admin/smtp-config')
      if (res.code === 20000) applyData(res.data)
      else message.error(res.message || '加载邮件配置失败')
    } catch (err) { message.error(err.message || '加载邮件配置失败') }
    setLoading(false)
  }

  useEffect(() => { fetchConfig() }, [])

  const handleSave = async () => {
    let values
    try { values = await form.validateFields() } catch { return }
    setSaving(true)
    try {
      const res = await api.put('/admin/smtp-config', values)
      if (res.code === 20000) {
        message.success('邮件配置已保存')
        await fetchConfig()
      } else {
        message.error(res.message || '保存失败')
      }
    } catch (err) { message.error(err.message || '保存失败') }
    setSaving(false)
  }

  const handleTest = async () => {
    if (!testTo.trim()) { message.error('请输入测试收件邮箱'); return }
    setTesting(true)
    try {
      const res = await api.post('/admin/smtp-config/test', { to: testTo.trim() })
      if (res.code === 20000) {
        message.success(`测试邮件已发送（通道：${res.data?.channel === 'directmail' ? '界面配置' : '个人邮箱回落'}）`)
      } else {
        message.error(res.message || '测试发送失败')
      }
    } catch (err) { message.error(err.message || '测试发送失败') }
    setTesting(false)
  }

  const sourceText = meta.source === 'db'
    ? '当前使用界面配置发送（发送失败会自动回落到 .env 的个人邮箱）'
    : meta.source === 'env'
      ? '当前未启用界面配置，使用 .env 个人邮箱（建议迁移到阿里云邮件推送）'
      : '未检测到可用的 SMTP 配置，请填写下方界面配置'

  return (
    <Card title="邮件服务配置" extra={<Button type="primary" loading={saving} onClick={handleSave}>保存</Button>}>
      {loading ? <Spin /> : (
        <div style={{ maxWidth: 640 }}>
          <Alert type={meta.source === 'none' ? 'warning' : 'info'} showIcon style={{ marginBottom: 16 }} message={sourceText} />
          <Form form={form} layout="vertical">
            <Form.Item name="enabled" label="启用界面配置" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="停用" />
            </Form.Item>
            <Form.Item name="host" label="SMTP 主机">
              <Input placeholder="如 smtpdm.aliyun.com" />
            </Form.Item>
            <Form.Item name="port" label="端口">
              <InputNumber min={1} max={65535} style={{ width: 160 }} />
            </Form.Item>
            <Form.Item name="username" label="账号">
              <Input placeholder="阿里云邮件推送发信地址" />
            </Form.Item>
            <Form.Item
              name="password"
              label="密码 / 授权码"
              extra={meta.password_set ? '已设置；留空表示不修改' : '加密存储，不会回显'}
            >
              <Input.Password placeholder={meta.password_set ? '••••••（留空不修改）' : '输入密码或授权码'} />
            </Form.Item>
            <Form.Item name="from" label="发件人">
              <Input placeholder="noreply@your-domain.com" />
            </Form.Item>
            <Form.Item name="use_tls" label="TLS" valuePropName="checked">
              <Switch checkedChildren="开" unCheckedChildren="关" />
            </Form.Item>
          </Form>
          <Card size="small" title="测试发送">
            <Space>
              <Input placeholder="收件邮箱" value={testTo} onChange={e => setTestTo(e.target.value)} style={{ width: 280 }} />
              <Button loading={testing} onClick={handleTest}>发送测试邮件</Button>
            </Space>
          </Card>
        </div>
      )}
    </Card>
  )
}
