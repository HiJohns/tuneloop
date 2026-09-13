import { useState, useEffect } from 'react'
import { Card, InputNumber, Switch, Button, message, Spin, Input } from 'antd'
import { api } from '../../../services/api'

// #1898: 警告通知配置（按级别配置通知邮箱 + 重复间隔）。
// 收件邮箱可为任意邮箱（不限于本站用户）。
const LEVELS = [
  { key: 'low', label: '低' },
  { key: 'medium', label: '中' },
  { key: 'high', label: '高' },
]

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export default function WarningSettings() {
  const [settings, setSettings] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.get('/warning-settings').then(res => {
      if (res.code === 20000) setSettings(res.data || {})
      else message.error(res.message || '加载警告配置失败')
    }).catch(err => message.error(err.message || '加载警告配置失败')).finally(() => setLoading(false))
  }, [])

  const update = (level, patch) => setSettings(prev => ({ ...prev, [level]: { ...(prev?.[level] || {}), ...patch } }))

  const normalized = (level) => {
    const cfg = settings?.[level] || {}
    return {
      enabled: !!cfg.enabled,
      emails: (cfg.emails || []).map(e => (e || '').trim()).filter(Boolean),
      cooldown_minutes: cfg.cooldown_minutes || 0,
    }
  }

  const handleSave = async () => {
    for (const lv of LEVELS) {
      const cfg = normalized(lv.key)
      const invalid = cfg.emails.find(e => !EMAIL_RE.test(e))
      if (invalid) { message.error(`[${lv.label}级] 邮箱格式不正确：${invalid}`); return }
      if (cfg.cooldown_minutes < 0 || cfg.cooldown_minutes > 1440) {
        message.error(`[${lv.label}级] 重复间隔需在 0~1440 分钟`); return
      }
    }
    setSaving(true)
    try {
      const payload = {}
      for (const lv of LEVELS) payload[lv.key] = normalized(lv.key)
      const res = await api.put('/warning-settings', payload)
      if (res.code === 20000) message.success('警告配置已保存')
      else message.error(res.message || '保存失败')
    } catch (err) { message.error(err.message || '保存失败') }
    setSaving(false)
  }

  return (
    <Card title="警告配置" extra={<Button type="primary" loading={saving} onClick={handleSave}>保存</Button>}>
      {loading ? <Spin /> : (
        <div className="space-y-6">
          <p className="text-gray-500">
            警告出现时，按级别向配置的邮箱发送邮件通知。收件邮箱可为外部邮箱（不限于本站用户）。
            重复间隔用于未处理警告的再次提醒：0 表示每条警告只通知一次。
            注意：SMTP 未配置时保存仍生效，发送会失败并在「创建警告」响应中返回错误提示。
          </p>
          {LEVELS.map(lv => {
            const cfg = normalized(lv.key)
            return (
              <Card
                key={lv.key}
                size="small"
                title={`${lv.label}级警告`}
                extra={<Switch checked={cfg.enabled} onChange={v => update(lv.key, { enabled: v })} checkedChildren="启用" unCheckedChildren="停用" />}
              >
                <div className="mb-3">
                  <label className="block text-sm font-medium mb-1">通知邮箱（每行一个）</label>
                  <Input.TextArea
                    rows={3}
                    value={cfg.emails.join('\n')}
                    onChange={e => update(lv.key, { emails: e.target.value.split('\n') })}
                    placeholder={'每行一个邮箱，例如：\nops@example.com\nit@corp.cn'}
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">重复间隔（分钟，0 = 仅一次）</label>
                  <InputNumber
                    min={0}
                    max={1440}
                    value={cfg.cooldown_minutes}
                    onChange={v => update(lv.key, { cooldown_minutes: v || 0 })}
                    style={{ width: 160 }}
                  />
                </div>
              </Card>
            )
          })}
        </div>
      )}
    </Card>
  )
}
