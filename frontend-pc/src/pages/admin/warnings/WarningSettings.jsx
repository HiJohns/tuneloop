import { useState, useEffect } from 'react'
import { Card, InputNumber, Switch, Button, message, Spin, Input } from 'antd'
import { api } from '../../../services/api'

// #1908: 警告通知配置（合并为单一配置，不再分级）。
// 收件邮箱可为任意邮箱（不限于本站用户）。
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export default function WarningSettings() {
  const [cfg, setCfg] = useState({ enabled: false, emails: [], cooldown_minutes: 0 })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.get('/warning-settings').then(res => {
      if (res.code === 20000) {
        setCfg({
          enabled: !!res.data?.enabled,
          emails: res.data?.emails || [],
          cooldown_minutes: res.data?.cooldown_minutes || 0,
        })
      } else {
        message.error(res.message || '加载警告配置失败')
      }
    }).catch(err => message.error(err.message || '加载警告配置失败')).finally(() => setLoading(false))
  }, [])

  const handleSave = async () => {
    const emails = (cfg.emails || []).map(e => (e || '').trim()).filter(Boolean)
    const invalid = emails.find(e => !EMAIL_RE.test(e))
    if (invalid) { message.error(`邮箱格式不正确：${invalid}`); return }
    if (cfg.cooldown_minutes < 0 || cfg.cooldown_minutes > 1440) {
      message.error('重复间隔需在 0~1440 分钟'); return
    }
    setSaving(true)
    try {
      const res = await api.put('/warning-settings', {
        enabled: !!cfg.enabled,
        emails,
        cooldown_minutes: cfg.cooldown_minutes || 0,
      })
      if (res.code === 20000) message.success('警告配置已保存')
      else message.error(res.message || '保存失败')
    } catch (err) { message.error(err.message || '保存失败') }
    setSaving(false)
  }

  return (
    <Card title="警告配置" extra={<Button type="primary" loading={saving} onClick={handleSave}>保存</Button>}>
      {loading ? <Spin /> : (
        <div className="space-y-4" style={{ maxWidth: 640 }}>
          <p className="text-gray-500">
            警告出现时，向下方邮箱发送邮件通知。收件邮箱可为外部邮箱（不限于本站用户）。
            重复间隔用于未处理警告的再次提醒：0 表示每条警告只通知一次。
            注意：SMTP 未配置时保存仍生效，发送会失败并在「创建警告」响应中返回错误提示。
          </p>
          <div className="flex items-center gap-2">
            <Switch
              checked={cfg.enabled}
              onChange={v => setCfg(p => ({ ...p, enabled: v }))}
              checkedChildren="启用"
              unCheckedChildren="停用"
            />
            <span>{cfg.enabled ? '启用邮件通知' : '停用邮件通知'}</span>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">通知邮箱（每行一个）</label>
            <Input.TextArea
              rows={4}
              value={(cfg.emails || []).join('\n')}
              onChange={e => setCfg(p => ({ ...p, emails: e.target.value.split('\n') }))}
              placeholder={'每行一个邮箱，例如：\nops@example.com\nit@corp.cn'}
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">重复间隔（分钟，0 = 仅一次）</label>
            <InputNumber
              min={0}
              max={1440}
              value={cfg.cooldown_minutes}
              onChange={v => setCfg(p => ({ ...p, cooldown_minutes: v || 0 }))}
              style={{ width: 160 }}
            />
          </div>
        </div>
      )}
    </Card>
  )
}
