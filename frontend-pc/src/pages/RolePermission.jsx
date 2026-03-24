import { useState, useEffect } from 'react'
import { Table, Button, Modal, Form, Input, Checkbox, Tag, Space, message, Card, Empty, Spin } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, LockOutlined } from '@ant-design/icons'
import { permissionApi } from '../services/api'

const PERMISSION_CATEGORIES = {
  dashboard: { label: 'Dashboard', icon: '📊' },
  assets: { label: 'Assets', icon: '🎸' },
  leases: { label: 'Leases', icon: '📋' },
  maintenance: { label: 'Maintenance', icon: '🔧' },
  finance: { label: 'Finance', icon: '💰' },
  users: { label: 'Users', icon: '👥' },
  settings: { label: 'Settings', icon: '⚙️' },
}

export default function RolePermission() {
  const [roles, setRoles] = useState([])
  const [permissions, setPermissions] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [editModalOpen, setEditModalOpen] = useState(false)
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [selectedRole, setSelectedRole] = useState(null)
  const [selectedPermissions, setSelectedPermissions] = useState([])
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      setLoading(true)
      setError(null)
      const [rolesRes, permsRes] = await Promise.all([
        permissionApi.getRoles(),
        permissionApi.getPermissions(),
      ])
      
      if (rolesRes.code === 20000) {
        setRoles(rolesRes.data || [])
      }
      if (permsRes.code === 20000) {
        setPermissions(permsRes.data || [])
      }
    } catch (err) {
      setError(err.message)
      message.error('加载失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleEditPermissions = (role) => {
    setSelectedRole(role)
    setSelectedPermissions(role.permissions || [])
    setEditModalOpen(true)
  }

  const handleSavePermissions = async () => {
    if (!selectedRole) return
    
    try {
      setSaving(true)
      await permissionApi.updateRolePermissions(selectedRole.id, selectedPermissions)
      message.success('权限更新成功')
      setEditModalOpen(false)
      loadData()
    } catch (err) {
      message.error('保存失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateRole = async (values) => {
    try {
      setSaving(true)
      await permissionApi.createRole(values)
      message.success('角色创建成功')
      setCreateModalOpen(false)
      form.resetFields()
      loadData()
    } catch (err) {
      message.error('创建失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDeleteRole = async (role) => {
    if (role.is_system) {
      message.warning('系统角色不可删除')
      return
    }

    try {
      await permissionApi.deleteRole(role.id)
      message.success('角色删除成功')
      loadData()
    } catch (err) {
      message.error('删除失败: ' + err.message)
    }
  }

  const columns = [
    {
      title: '角色名称',
      dataIndex: 'name',
      key: 'name',
      render: (name, record) => (
        <Space>
          <span>{name}</span>
          {record.is_system && <Tag color="blue" icon={<LockOutlined />}>系统</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '权限数量',
      dataIndex: 'permission_count',
      key: 'permission_count',
      width: 100,
      render: (count) => <Tag color="green">{count}</Tag>,
    },
    {
      title: '权限列表',
      dataIndex: 'permissions',
      key: 'permissions',
      render: (perms) => (
        <div style={{ maxWidth: 400 }}>
          {perms && perms.length > 0 ? (
            perms.slice(0, 5).map((p) => (
              <Tag key={p} style={{ marginBottom: 2 }}>{p}</Tag>
            ))
          ) : (
            <span className="text-gray-400">无权限</span>
          )}
          {perms && perms.length > 5 && (
            <Tag>+{perms.length - 5} more</Tag>
          )}
        </div>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_, record) => (
        <Space>
          <Button 
            type="link" 
            icon={<EditOutlined />} 
            onClick={() => handleEditPermissions(record)}
          >
            编辑
          </Button>
          {!record.is_system && (
            <Button 
              type="link" 
              danger 
              icon={<DeleteOutlined />}
              onClick={() => handleDeleteRole(record)}
            >
              删除
            </Button>
          )}
        </Space>
      ),
    },
  ]

  const groupedPermissions = permissions.reduce((acc, perm) => {
    if (!acc[perm.category]) {
      acc[perm.category] = []
    }
    acc[perm.category].push(perm)
    return acc
  }, {})

  const permissionOptions = Object.entries(groupedPermissions).map(([category, perms]) => ({
    label: (
      <span>
        {PERMISSION_CATEGORIES[category]?.icon || '📁'} {PERMISSION_CATEGORIES[category]?.label || category}
      </span>
    ),
    options: perms.map((p) => ({
      label: p.description || p.name,
      value: p.name,
    })),
  }))

  if (loading) {
    return (
      <div className="p-6">
        <div className="text-center py-16">
          <Spin size="large" />
          <div className="mt-4 text-gray-500">数据正在同步中...</div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <Empty description="数据加载失败" />
        <Button type="primary" onClick={loadData} className="mt-4">
          重试
        </Button>
      </div>
    )
  }

  return (
    <div className="p-6">
      <Card
        title="角色权限管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
            新建角色
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={roles}
          rowKey="id"
          pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
        />
      </Card>

      <Modal
        title={`编辑角色权限: ${selectedRole?.name || ''}`}
        open={editModalOpen}
        onCancel={() => setEditModalOpen(false)}
        onOk={handleSavePermissions}
        width={600}
        confirmLoading={saving}
      >
        <div className="mb-4 text-gray-500">
          勾选该角色拥有的权限
        </div>
        <Checkbox.Group
          value={selectedPermissions}
          onChange={(values) => setSelectedPermissions(values)}
          className="w-full"
        >
          {permissionOptions.map((group) => (
            <Card key={group.label} size="small" className="mb-4">
              <div className="font-medium mb-2">{group.label}</div>
              <Checkbox.Group 
                value={selectedPermissions} 
                onChange={(vals) => setSelectedPermissions(vals)}
                className="w-full"
              >
                <div className="grid grid-cols-2 gap-2">
                  {group.options.map((opt) => (
                    <Checkbox 
                      key={opt.value} 
                      value={opt.value}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedPermissions([...selectedPermissions, opt.value])
                        } else {
                          setSelectedPermissions(selectedPermissions.filter(p => p !== opt.value))
                        }
                      }}
                    >
                      {opt.label}
                    </Checkbox>
                  ))}
                </div>
              </Checkbox.Group>
            </Card>
          ))}
        </Checkbox.Group>
      </Modal>

      <Modal
        title="新建角色"
        open={createModalOpen}
        onCancel={() => {
          setCreateModalOpen(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        confirmLoading={saving}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleCreateRole}
        >
          <Form.Item
            name="name"
            label="角色名称"
            rules={[{ required: true, message: '请输入角色名称' }]}
          >
            <Input placeholder="请输入角色名称" />
          </Form.Item>
          
          <Form.Item
            name="description"
            label="角色描述"
          >
            <Input.TextArea rows={3} placeholder="请输入角色描述" />
          </Form.Item>

          <Form.Item
            name="permissions"
            label="初始权限"
          >
            <Checkbox.Group options={permissionOptions} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
