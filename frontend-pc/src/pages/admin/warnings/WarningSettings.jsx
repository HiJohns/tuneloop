import { useState, useEffect } from 'react'
import { Card, InputNumber, Switch, Button, message, Spin, Form } from 'antd'
import { api } from '../../../services/api'

export default function WarningSettings() {
  const [settings, setSettings] = useState({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    Promise.all([
      api.get('/config/repair/single?key=warning_level_low_actions'),
      api.get('/config/repair/single?key=warning_level_medium_actions'),
      api.get('/config/repair/single?key=warning_level_high_actions'),
    ]).then(() => setLoading(false)).catch(() => setLoading(false))
  }, [])

  return (
    <Card title="警告配置" extra={<Button type="primary" loading={saving}>保存</Button>}>
      {loading ? <Spin /> : (
        <p className="text-gray-500">警告级别配置：各等级的默认通知对象、通知方式和重复频率在此设置。配置存储在 system_settings 表。</p>
      )}
    </Card>
  )
}
