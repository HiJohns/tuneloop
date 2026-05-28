import { useState, useEffect, useCallback } from 'react'
import { Tabs, Table, Card, Modal, Form, Select, Checkbox, Tag, Button, Space, message, Spin, Popconfirm, Tooltip, Alert } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons'
import { adminApi } from '../../../services/api'

const PERM_GROUPS = [
  {
    title: '乐器',
    key: 'instrument',
    codes: [
      { code: 'instrument:create', name: '创建乐器' },
      { code: 'instrument:read', name: '查看乐器' },
      { code: 'instrument:update', name: '编辑乐器' },
      { code: 'instrument:delete', name: '删除乐器' },
      { code: 'instrument:price', name: '乐器定价' },
      { code: 'instrument:maintain', name: '维修管理' },
    ],
  },
  {
    title: '订单',
    key: 'order',
    codes: [
      { code: 'order:create', name: '创建订单' },
      { code: 'order:read', name: '查看订单' },
      { code: 'order:update', name: '编辑订单' },
      { code: 'order:cancel', name: '取消订单' },
    ],
  },
]

const ROLE_COLORS = {
  owner: 'red',
  merchant_admin: 'red',
  admin: 'blue',
  site_admin: 'blue',
  staff: 'green',
  site_member: 'green',
  worker: 'orange',
}

function getCusPermSummary(codes) {
  if (!codes || codes.length === 0) return '无'
  const groups = {}
  for (const c of codes) {
    const prefix = c.split(':')[0]
    groups[prefix] = (groups[prefix] || 0) + 1
  }
  return Object.entries(groups).map(([k, v]) => `${k === 'instrument' ? '乐器' : '订单'}:${v}`).join(' ')
}

export default function PermissionManage() {
  const [activeTab, setActiveTab] = useState('members')

  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold text-gray-900 mb-6">权限管理</h1>
      <Tabs activeKey={activeTab} onChange={setActiveTab}>
        <Tabs.TabPane tab="成员权限" key="members">
          <MemberPermissions />
        </Tabs.TabPane>
        <Tabs.TabPane tab="角色管理" key="roles">
          <RoleManagement />
        </Tabs.TabPane>
      </Tabs>
    </div>
  )
}

function MemberPermissions() {
  const [members, setMembers] = useState([])
  const [loading, setLoading] = useState(true)
  const [editModal, setEditModal] = useState(null)
  const [saving, setSaving] = useState(false)

  const fetchMembers = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await adminApi.listUsers()
      if (resp.code === 20000) setMembers(resp.data || [])
    } catch { message.error('加载成员列表失败') }
    setLoading(false)
  }, [])

  useEffect(() => { fetchMembers() }, [fetchMembers])

  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '所属网点', dataIndex: 'site_name', key: 'site_name' },
    {
      title: '角色',
      dataIndex: 'role_code',
      key: 'role_code',
      render: (code) => <Tag color={ROLE_COLORS[code] || 'default'}>{code}</Tag>,
    },
    {
      title: '权限摘要',
      key: 'summary',
      render: (_, r) => <span className="text-gray-500">{getCusPermSummary(r.cus_perm_codes)}</span>,
    },
    {
      title: '操作',
      key: 'action',
      render: (_, r) => (
        <Button type="link" icon={<EditOutlined />} onClick={() => setEditModal(r)}>编辑权限</Button>
      ),
    },
  ]

  return (
    <>
      <Card>
        <Table
          columns={columns}
          dataSource={members}
          rowKey="user_id"
          loading={loading}
          pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>
      {editModal && (
        <MemberPermissionEditModal
          member={editModal}
          visible={!!editModal}
          onClose={() => setEditModal(null)}
          onSave={async (roleCode, cusPermCodes) => {
            setSaving(true)
            try {
              if (roleCode) {
                const roleResp = await adminApi.setUserRole(editModal.user_id, roleCode)
                if (roleResp.code !== 20000) { message.error(roleResp.message || '更新失败'); setSaving(false); return }
              }
              const permResp = await adminApi.setUserPermissions(editModal.user_id, cusPermCodes)
              if (permResp.code !== 20000) { message.error(permResp.message || '更新失败'); setSaving(false); return }
              message.success('权限已更新，该用户下次登录后生效')
              setEditModal(null)
              fetchMembers()
            } catch { message.error('更新失败') }
            setSaving(false)
          }}
          saving={saving}
        />
      )}
    </>
  )
}

function MemberPermissionEditModal({ member, visible, onClose, onSave, saving }) {
  const [roleCode, setRoleCode] = useState(member.role_code || 'site_member')
  const [cusPermCodes, setCusPermCodes] = useState(member.cus_perm_codes || [])
  const [roles, setRoles] = useState([])

  useEffect(() => {
    if (!visible) return
    setRoleCode(member.role_code || 'site_member')
    setCusPermCodes(member.cus_perm_codes || [])
    adminApi.listRoles().then(resp => {
      if (resp.code === 20000) setRoles(resp.data || [])
    })
  }, [visible, member])

  const toggleCode = (code) => {
    setCusPermCodes(prev =>
      prev.includes(code) ? prev.filter(c => c !== code) : [...prev, code]
    )
  }

  return (
    <Modal
      title={`编辑权限 — ${member.name}`}
      open={visible}
      onCancel={onClose}
      onOk={() => onSave(roleCode, cusPermCodes)}
      confirmLoading={saving}
      width={520}
    >
      <Form layout="vertical">
        <Form.Item label="角色">
          <Select value={roleCode} onChange={setRoleCode} style={{ width: '100%' }}>
            {roles.map(r => (
              <Select.Option key={r.code} value={r.code}>{r.name}</Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item label="个人权限（独立于角色，叠加生效）">
          {PERM_GROUPS.map(group => (
            <div key={group.key} className="mb-4">
              <div className="font-medium mb-2 text-gray-700">{group.title}</div>
              <div className="flex flex-wrap gap-3">
                {group.codes.map(p => (
                  <Checkbox
                    key={p.code}
                    checked={cusPermCodes.includes(p.code)}
                    onChange={() => toggleCode(p.code)}
                  >
                    {p.name}
                  </Checkbox>
                ))}
              </div>
            </div>
          ))}
        </Form.Item>
      </Form>
    </Modal>
  )
}

function RoleManagement() {
  const [roles, setRoles] = useState([])
  const [loading, setLoading] = useState(true)
  const [editModal, setEditModal] = useState(null)
  const [creating, setCreating] = useState(false)
  const [saving, setSaving] = useState(false)

  const fetchRoles = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await adminApi.listRoles()
      if (resp.code === 20000) setRoles(resp.data || [])
    } catch { message.error('加载角色列表失败') }
    setLoading(false)
  }, [])

  useEffect(() => { fetchRoles() }, [fetchRoles])

  const handleDelete = async (id) => {
    try {
      const resp = await adminApi.deleteRole(id)
      if (resp.code === 20000) {
        message.success('角色已删除')
        fetchRoles()
      }
    } catch { message.error('删除失败') }
  }

  const columns = [
    { title: '角色名称', dataIndex: 'name', key: 'name' },
    { title: '代码', dataIndex: 'code', key: 'code' },
    {
      title: '权限数', dataIndex: 'permission_count', key: 'permission_count',
      render: (count) => <Tag color="green">{count}</Tag>,
    },
    {
      title: '权限详情',
      key: 'details',
      render: (_, r) => <span className="text-gray-500">{getCusPermSummary(r.cus_perm_codes)}</span>,
    },
    {
      title: '操作',
      key: 'action',
      render: (_, r) => (
        <Space>
          <Button type="link" icon={<EditOutlined />} onClick={() => setEditModal(r)}>编辑</Button>
          {!r.is_system && (
            <Popconfirm title="确定删除此角色？" onConfirm={() => handleDelete(r.id)}>
              <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Card extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>新建角色</Button>}>
        <Table
          columns={columns}
          dataSource={roles}
          rowKey={r => r.id || r.code}
          loading={loading}
          pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>
      <RoleFormModal
        visible={creating || !!editModal}
        editing={editModal}
        onClose={() => { setEditModal(null); setCreating(false) }}
        onSave={async (data) => {
          setSaving(true)
          try {
            let resp
            if (editModal) {
              resp = await adminApi.updateRole(editModal.id, data)
            } else {
              resp = await adminApi.createRole(data)
            }
            if (resp.code !== 20000) { message.error(resp.message || '保存失败'); setSaving(false); return }
            message.success(editModal ? '角色已更新' : '角色已创建')
            setEditModal(null); setCreating(false)
            fetchRoles()
          } catch { message.error('保存失败') }
          setSaving(false)
        }}
        saving={saving}
      />
    </>
  )
}

function RoleFormModal({ visible, editing, onClose, onSave, saving }) {
  const [name, setName] = useState('')
  const [code, setCode] = useState('')
  const [cusPermCodes, setCusPermCodes] = useState([])

  useEffect(() => {
    if (!visible) return
    if (editing) {
      setName(editing.name)
      setCode(editing.code)
      setCusPermCodes(editing.cus_perm_codes || [])
    } else {
      setName('')
      setCode('')
      setCusPermCodes([])
    }
  }, [visible, editing])

  const toggleCode = (code) => {
    setCusPermCodes(prev =>
      prev.includes(code) ? prev.filter(c => c !== code) : [...prev, code]
    )
  }

  return (
    <Modal
      title={editing ? '编辑角色' : '新建角色'}
      open={visible}
      onCancel={onClose}
      onOk={() => onSave({ name, code, cus_perm_codes: cusPermCodes })}
      confirmLoading={saving}
      width={520}
    >
      <Form layout="vertical">
        <Form.Item label="角色名称" required>
          <input
            className="ant-input"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="输入角色名称"
            disabled={!!editing}
          />
        </Form.Item>
        {!editing && (
          <Form.Item label="角色代码" required>
            <input
              className="ant-input"
              value={code}
              onChange={e => setCode(e.target.value)}
              placeholder="输入角色代码（英文）"
            />
          </Form.Item>
        )}
        <Form.Item label="权限配置">
          {PERM_GROUPS.map(group => (
            <div key={group.key} className="mb-4">
              <div className="font-medium mb-2 text-gray-700">{group.title}</div>
              <div className="flex flex-wrap gap-3">
                {group.codes.map(p => (
                  <Checkbox
                    key={p.code}
                    checked={cusPermCodes.includes(p.code)}
                    onChange={() => toggleCode(p.code)}
                  >
                    {p.name}
                  </Checkbox>
                ))}
              </div>
            </div>
          ))}
        </Form.Item>
      </Form>
    </Modal>
  )
}
