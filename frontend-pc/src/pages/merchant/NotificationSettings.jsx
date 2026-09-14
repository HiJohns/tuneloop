import { useState, useEffect } from 'react'
import { Card, Switch, Input, Button, message, Spin, Alert } from 'antd'
import { api } from '../../services/api'

// #1909: 商户通知设置（商户管理员）。商户用自己的企业微信群机器人；
// 未配置或推送失败时，警告仍会以站内通知送达。
export default function NotificationSettings() {
  const [enabled, setEnabled] = useState(false)
  const [urlSet, setUrlSet] = useState(false)
  const [url, setUrl] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  const fetchConfig = async () => {
    try {
      const res = await api.get('/merchant/notification-webhook')
      if (res.code === 20000) {
        setEnabled(!!res.data?.enabled)
        setUrlSet(!!res.data?.url_set)
        setUrl('')
      } else {
        message.error(res.message || '加载通知设置失败')
      }
    } catch (err) { message.error(err.message || '加载通知设置失败') }
    setLoading(false)
  }

  useEffect(() => { fetchConfig() }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      const res = await api.put('/merchant/notification-webhook', { enabled, url: url.trim() })
      if (res.code === 20000) {
        message.success('通知设置已保存')
        await fetchConfig()
      } else {
        message.error(res.message || '保存失败')
      }
    } catch (err) { message.error(err.message || '保存失败') }
    setSaving(false)
  }

  const handleTest = async () => {
    setTesting(true)
    try {
      const res = await api.post('/merchant/notification-webhook/test', {})
      if (res.code === 20000) message.success('测试消息已发送到企业微信群')
      else message.error(res.message || '测试发送失败')
    } catch (err) { message.error(err.message || '测试发送失败') }
    setTesting(false)
  }

  return (
    <Card title="通知设置" extra={<Button type="primary" loading={saving} onClick={handleSave}>保存</Button>}>
      {loading ? <Spin /> : (
        <div style={{ maxWidth: 640 }}>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="企业微信群机器人推送（可选）"
            description="在本商户的企业微信群里添加“群机器人”，把 Webhook 地址填到下面即可把本商户的告警推送到群。未配置或推送失败时，告警仍会出现在站内通知中，不会丢失。"
          />
          <div className="flex items-center gap-2 mb-3">
            <Switch
              checked={enabled}
              onChange={setEnabled}
              checkedChildren="启用"
              unCheckedChildren="停用"
            />
            <span>{enabled ? '启用企业微信推送' : '停用企业微信推送'}</span>
            <Button size="small" loading={testing} onClick={handleTest} disabled={!urlSet && !url.trim()}>
              发送测试
            </Button>
          </div>
          <label className="block text-sm font-medium mb-1">企业微信群机器人 Webhook URL</label>
          <Input
            value={url}
            onChange={e => setUrl(e.target.value)}
            placeholder={urlSet ? '已配置（留空表示不修改）' : 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…'}
          />
          <p className="text-xs text-gray-400 mt-1">地址加密存储，不会回显；每个商户只推送本商户的事件。</p>
        </div>
      )}
    </Card>
  )
}
