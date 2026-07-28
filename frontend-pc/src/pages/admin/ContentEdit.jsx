import { useState, useEffect } from 'react'
import { Card, Tabs, Input, Button, message } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import { api } from '../../services/api'

const KEYS = [
  { key: 'rental_notice', title: '租赁须知' },
  { key: 'contact_us', title: '联系我们' },
]

export default function ContentEdit() {
  const [loading, setLoading] = useState({})
  const [values, setValues] = useState({})

  useEffect(() => {
    KEYS.forEach(k => load(k.key))
  }, [])

  const load = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.get(`/settings/${key}`)
      if (res.code === 20000) {
        setValues(prev => ({ ...prev, [key]: res.data?.value || '' }))
      }
    } catch { message.error(`加载${key}失败`) }
    setLoading(prev => ({ ...prev, [key]: false }))
  }

  const save = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.put(`/admin/content/${key}`, { value: values[key] || '' })
      if (res.code === 20000) {
        message.success(`${KEYS.find(k => k.key === key)?.title}已保存`)
      } else { message.error(res.message || '保存失败') }
    } catch { message.error('保存失败') }
    setLoading(prev => ({ ...prev, [key]: false }))
  }

  const items = KEYS.map(k => ({
    key: k.key,
    label: k.title,
    children: (
      <div>
        <Input.TextArea
          rows={12}
          value={values[k.key]}
          onChange={e => setValues(prev => ({ ...prev, [k.key]: e.target.value }))}
          placeholder={`请输入${k.title}内容`}
          style={{ marginBottom: 12 }}
        />
        <Button type="primary" icon={<SaveOutlined />} onClick={() => save(k.key)} loading={loading[k.key]}>
          保存
        </Button>
      </div>
    ),
  }))

  return (
    <div className="p-6">
      <Card title="内容编辑">
        <Tabs items={items} />
      </Card>
    </div>
  )
}
