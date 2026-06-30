import { useState, useEffect } from 'react'
import { Card, InputNumber, Switch, Button, message, Spin, Form, Tooltip } from 'antd'
import { adminApi } from '../../services/api'

export default function RepairConfigPage() {
  const [inspectionFee, setInspectionFee] = useState(0)
  const [shippingFee, setShippingFee] = useState(0)
  const [giftPointsEnabled, setGiftPointsEnabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    adminApi.get('/config/repair').then(res => {
      if (res.code === 20000) {
        setInspectionFee(Number(res.data?.repair_inspection_fee || 0))
        setShippingFee(Number(res.data?.repair_shipping_fee || 0))
        setGiftPointsEnabled(res.data?.repair_gift_points_enabled === 'true')
      }
    }).catch(() => {}).finally(() => setLoading(false))
  }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      await Promise.all([
        adminApi.put('/config/repair/single', { key: 'repair_inspection_fee', value: String(inspectionFee) }),
        adminApi.put('/config/repair/single', { key: 'repair_shipping_fee', value: String(shippingFee) }),
        adminApi.put('/config/repair/single', { key: 'repair_gift_points_enabled', value: String(giftPointsEnabled) }),
      ])
      message.success('保存成功')
    } catch {
      message.error('保存失败')
    }
    setSaving(false)
  }

  if (loading) return <Spin style={{ display: 'flex', justifyContent: 'center', marginTop: 100 }} />

  return (
    <Card title="报修设置" extra={<Button type="primary" loading={saving} onClick={handleSave}>保存配置</Button>}>
      <Form layout="vertical" style={{ maxWidth: 400 }}>
        <Form.Item label="检查费（元）" tooltip="用户拒绝报价时收取的检查费用">
          <InputNumber min={0} precision={2} value={inspectionFee} onChange={setInspectionFee} style={{ width: '100%' }} addonAfter="元" />
        </Form.Item>
        <Form.Item label="报修物流费默认值（元）" tooltip="商户级默认，网点可覆盖">
          <InputNumber min={0} precision={2} value={shippingFee} onChange={setShippingFee} style={{ width: '100%' }} addonAfter="元" />
        </Form.Item>
        <Form.Item label="报修允许使用赠点">
          <Switch checked={giftPointsEnabled} onChange={setGiftPointsEnabled} />
        </Form.Item>
      </Form>
    </Card>
  )
}
