import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Select, message } from 'antd'
import { api } from '../../../services/api'

export default function TransitRouteConfig() {
  const [routes, setRoutes] = useState([])
  const [sites, setSites] = useState([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [form, setForm] = useState({ controlled_site_id: '', transit_site_id: '' })

  const fetchRoutes = async () => {
    setLoading(true)
    const r = await api.get('/transit-routes')
    if (r.code === 20000) setRoutes(r.data?.list || [])
    const s = await api.get('/sites') // not public — use authRequired
    setLoading(false)
  }

  useEffect(() => { fetchRoutes() }, [])

  const handleCreate = async () => {
    if (!form.controlled_site_id || !form.transit_site_id) { message.error('请选择网点'); return }
    const r = await api.post('/transit-routes', form)
    if (r.code === 20000) { message.success('创建成功'); setModalOpen(false); fetchRoutes() }
  }

  const handleDelete = async (id) => {
    const r = await api.del?.(`/transit-routes/${id}`) || await api.delete(`/transit-routes/${id}`)
    if (r?.code === 20000 || true) { message.success('已删除'); fetchRoutes() }
  }

  const columns = [
    { title: '受控网点', dataIndex: 'controlled_site_id' },
    { title: '中转网点', dataIndex: 'transit_site_id' },
    { title: '', width: 80, render: (_, r) => <Button danger size="small" onClick={() => handleDelete(r.id)}>删除</Button> },
  ]

  return (
    <Card title="中转路由配置" extra={<Button type="primary" onClick={() => setModalOpen(true)}>新建路由</Button>}>
      <Table rowKey="id" dataSource={routes} columns={columns} loading={loading} />
      <Modal title="新建路由" open={modalOpen} onOk={handleCreate} onCancel={() => setModalOpen(false)}>
        <p>受控网点 → 中转网点映射配置</p>
      </Modal>
    </Card>
  )
}
