// 中转中心（#1931/#1935/#1936）— 平台级中转网点管理 + 成员 + 路由配置
import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, message, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'
import SiteMemberManagement from '../../../components/SiteMemberManagement'

const { Text } = Typography

// #1938: 中转成员角色仅两类，静态注入（不复用 /admin/roles——其权限门与中转中心不同）
const TRANSIT_ROLES = [
  { code: 'site_admin', name: '中转网点管理员' },
  { code: 'site_member', name: '中转网点员工' },
]

export default function TransitCenter() {
  const [sites, setSites] = useState([])
  const [routes, setRoutes] = useState([])
  const [controlledSites, setControlledSites] = useState([])
  const [loading, setLoading] = useState(false)

  const [siteModalOpen, setSiteModalOpen] = useState(false)
  const [editingSite, setEditingSite] = useState(null)
  const [siteForm] = Form.useForm()

  const [memberModalOpen, setMemberModalOpen] = useState(false)
  const [memberSite, setMemberSite] = useState(null)

  const [routeModalOpen, setRouteModalOpen] = useState(false)
  const [routeForm] = Form.useForm()

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      // #1936 audit Bug1: GET /api/sites 不存在 → 改用平台级受控网点列表端点
      const [s, r, cs] = await Promise.all([
        api.get('/admin/transit-sites'),
        api.get('/transit-routes'),
        api.get('/admin/controlled-sites'),
      ])
      if (s.code === 20000) setSites(s.data?.list || [])
      if (r.code === 20000) setRoutes(r.data?.list || [])
      // 后端已按 controlled 商户 + 非中转类型过滤，前端无需再筛
      if (cs.code === 20000) setControlledSites(cs.data?.list || [])
    } catch (e) {
      message.error(e.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])

  const openCreate = () => {
    setEditingSite(null)
    siteForm.resetFields()
    setSiteModalOpen(true)
  }

  const openEdit = (site) => {
    setEditingSite(site)
    siteForm.setFieldsValue(site)
    setSiteModalOpen(true)
  }

  const handleSiteSubmit = async () => {
    const values = await siteForm.validateFields()
    if (editingSite) {
      await api.put(`/admin/transit-sites/${editingSite.id}`, values)
      message.success('已更新')
    } else {
      const r = await api.post('/admin/transit-sites', values)
      if (r.code !== 20000) { message.error(r.message || '创建失败'); return }
      message.success('中转网点已创建')
    }
    setSiteModalOpen(false)
    fetchAll()
  }

  const handleDeactivate = async (site) => {
    const r = await api.delete(`/admin/transit-sites/${site.id}`)
    if (r.code === 20000) { message.success('已停用'); fetchAll() }
    else message.error(r.message || '停用失败（需先解除路由引用）')
  }

  const openMembers = (site) => {
    setMemberSite(site)
    setMemberModalOpen(true)
  }

  const handleRouteCreate = async () => {
    const values = await routeForm.validateFields()
    const r = await api.post('/transit-routes', values)
    if (r.code === 20000) {
      message.success('路由已创建')
      setRouteModalOpen(false)
      routeForm.resetFields()
      fetchAll()
    } else {
      message.error(r.message || '创建失败')
    }
  }

  const handleRouteDelete = async (routeId) => {
    const r = await api.delete(`/transit-routes/${routeId}`)
    if (r.code === 20000) { message.success('路由已删除'); fetchAll() }
    else message.error(r.message || '删除失败')
  }

  return (
    <div style={{ padding: 24 }}>
      <Card title="中转网点（平台直辖）" extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>创建中转网点</Button>
      } style={{ marginBottom: 16 }}>
        <Table
          rowKey="id"
          loading={loading}
          dataSource={sites}
          columns={[
            { title: '名称', dataIndex: 'name', key: 'name' },
            { title: '类型', dataIndex: 'type', key: 'type', render: v => <Tag color="purple">{v === 'transit' ? '中转网点' : (v || '-')}</Tag> },
            { title: '地址', dataIndex: 'address', key: 'address' },
            { title: '电话', dataIndex: 'phone', key: 'phone' },
            { title: '路由引用', dataIndex: 'route_count', key: 'route_count', render: v => <Tag color={v > 0 ? 'blue' : 'default'}>{v} 条</Tag> },
            { title: '成员', dataIndex: 'member_count', key: 'member_count' },
            { title: '状态', dataIndex: 'status', key: 'status', render: v => <Tag color={v === 'active' ? 'green' : 'default'}>{v === 'active' ? '启用' : '停用'}</Tag> },
            { title: '操作', key: 'ops', render: (_, s) => (
              <Space>
                <Button size="small" onClick={() => openEdit(s)}>编辑</Button>
                <Button size="small" onClick={() => openMembers(s)}>成员</Button>
                {s.status === 'active' && <Button size="small" danger onClick={() => handleDeactivate(s)}>停用</Button>}
              </Space>
            )},
          ]}
          pagination={false}
        />
      </Card>

      <Card title="中转路由（受控网点 ↔ 中转网点）" extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setRouteModalOpen(true)}>新增路由</Button>
      }>
        <Table
          rowKey="id"
          loading={loading}
          dataSource={routes}
          columns={[
            { title: '受控网点', dataIndex: 'controlled_site_id', key: 'cs', render: v => <Text code>{String(v).slice(0, 8)}</Text> },
            { title: '中转网点', dataIndex: 'transit_site_id', key: 'ts', render: v => {
              const s = sites.find(x => x.id === v)
              return s ? s.name : <Text code>{String(v).slice(0, 8)}</Text>
            }},
            { title: '默认', dataIndex: 'is_default', key: 'is_default', render: v => v ? <Tag color="green">默认</Tag> : '-' },
            { title: '优先级', dataIndex: 'priority', key: 'priority' },
            { title: '操作', key: 'ops', render: (_, r) => <Button size="small" danger onClick={() => handleRouteDelete(r.id)}>删除</Button> },
          ]}
          pagination={false}
        />
      </Card>

      {/* 创建/编辑中转网点 */}
      <Modal
        title={editingSite ? '编辑中转网点' : '创建中转网点'}
        open={siteModalOpen}
        onOk={handleSiteSubmit}
        onCancel={() => setSiteModalOpen(false)}
        destroyOnClose
      >
        <Form form={siteForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如 北京中转站" />
          </Form.Item>
          <Form.Item name="address" label="地址（必填，物流面单用）" rules={[{ required: true, message: '请输入地址' }]}>
            <Input placeholder="详细地址" />
          </Form.Item>
          <Form.Item name="phone" label="电话（必填）" rules={[{ required: true, message: '请输入电话' }]}>
            <Input placeholder="联系电话" />
          </Form.Item>
          <Form.Item name="contact_name" label="联系人（可选）">
            <Input placeholder="联系人姓名" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 成员管理（#1938：复用网点成员管理组件 SiteMemberManagement） */}
      <Modal
        title={`成员管理 — ${memberSite?.name || ''}`}
        open={memberModalOpen}
        footer={null}
        onCancel={() => setMemberModalOpen(false)}
        width={720}
        destroyOnClose
      >
        {memberSite && (
          <SiteMemberManagement
            siteId={memberSite.id}
            membersBase="/admin/transit-sites"
            roles={TRANSIT_ROLES}
            onRefresh={fetchAll}
          />
        )}
      </Modal>

      {/* 新增路由 */}
      <Modal
        title="新增中转路由（受控网点 → 中转网点）"
        open={routeModalOpen}
        onOk={handleRouteCreate}
        onCancel={() => setRouteModalOpen(false)}
        destroyOnClose
      >
        <Form form={routeForm} layout="vertical">
          <Form.Item name="controlled_site_id" label="受控网点" rules={[{ required: true, message: '请选择受控网点' }]}>
            <Select placeholder="选择受控网点" options={controlledSites.map(s => ({ value: s.id, label: s.name }))} />
          </Form.Item>
          <Form.Item name="transit_site_id" label="中转网点" rules={[{ required: true, message: '请选择中转网点' }]}>
            <Select placeholder="选择中转网点" options={sites.map(s => ({ value: s.id, label: s.name }))} />
          </Form.Item>
          <Form.Item name="is_default" label="设为默认" initialValue={false}>
            <Select options={[{ value: true, label: '是' }, { value: false, label: '否' }]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}