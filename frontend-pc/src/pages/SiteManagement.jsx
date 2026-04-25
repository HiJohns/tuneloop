import { useState, useEffect } from 'react'
import { Card, Tree, Descriptions, Button, Modal, Form, Input, Select, message, Spin, Empty, Space, Popconfirm, Tabs } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons'
import { sitesApi } from '../services/api'
import { api } from '../services/api'
import Logger from '../utils/logger'
import SiteMemberManagement from '../components/SiteMemberManagement'
import UserSelectionDialog from '../components/UserSelectionDialog'

const { Option } = Select

export default function SiteManagement() {
  const [treeData, setTreeData] = useState([])
  const [selectedSite, setSelectedSite] = useState(null)
  const [editingSite, setEditingSite] = useState(null)
  const [loading, setLoading] = useState(true)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [managerInfo, setManagerInfo] = useState({ name: '', id: null, email: '', phone: '' })
  const [managerInput, setManagerInput] = useState('')  // Add state for the input field
  const [createUserModalVisible, setCreateUserModalVisible] = useState(false)
  const [createUserForm] = Form.useForm()
  const [viewMode, setViewMode] = useState('detail') // 'detail' | 'form'
  const [formMode, setFormMode] = useState('create') // 'create' | 'edit'
  const [lookupLoading, setLookupLoading] = useState(false)
  const [lookupError, setLookupError] = useState({ message: '', visible: false })
  const [expandedKeys, setExpandedKeys] = useState([])
  const [selectedKeys, setSelectedKeys] = useState([])
  const [userDialogVisible, setUserDialogVisible] = useState(false)

  useEffect(() => {
    fetchSiteTree()
  }, [])

  const fetchSiteTree = async () => {
    Logger.state('SiteManagement', { status: 'fetchSiteTree', action: 'START' })
    try {
      setLoading(true)
      Logger.api('/sites/tree', 'GET', { action: 'fetchSiteTree' })
      const result = await sitesApi.getTree()
      if (result.code === 20000) {
        const sites = result.data?.list || []
        Logger.state('SiteManagement', { sitesCount: sites.length, treeNodesCount: sites.length })
        setTreeData(sites.map(site => convertToTreeNode(site)))
      } else {
        Logger.warn('SITE', 'Unexpected result code:', result.code)
      }
    } catch (err) {
      Logger.error('SITE', 'fetchSiteTree error:', err)
      message.error('加载网点数据失败: ' + err.message)
    } finally {
      setLoading(false)
      Logger.state('SiteManagement', { status: 'fetchSiteTree', action: 'END' })
    }
  }

  const convertToTreeNode = (site) => ({
    key: site.id,
    title: site.name,
    data: site,
    // For dynamic loading, set empty children array so Tree shows expand icon
    // Actual children will be loaded via loadData when node is expanded
    children: []
  })

  const loadChildren = async (siteId) => {
    try {
      const result = await sitesApi.getTree(siteId)
      if (result.code === 20000) {
        return (result.data?.list || []).map(convertToTreeNode)
      }
      return []
    } catch (err) {
      Logger.error('SITE', 'Failed to load children:', err)
      return []
    }
  }

  const onSelect = (selectedKeys) => {
    setSelectedKeys(selectedKeys)
    if (selectedKeys.length > 0) {
      const findSite = (nodes, id) => {
        for (const node of nodes) {
          if (node.key === id) return node.data
          if (node.children) {
            const found = findSite(node.children, id)
            if (found) return found
          }
        }
        return null
      }
      const site = findSite(treeData, selectedKeys[0])
      setSelectedSite(site)
    } else {
      setSelectedSite(null)
    }
  }

  const refreshAndSelectTreeNode = async (siteId) => {
    Logger.state('SiteManagement', { action: 'refreshAndSelectTreeNode', siteId })
    
    // 刷新 Tree 数据
    await fetchSiteTree()
    
    // 设置选中的 keys
    setSelectedKeys([siteId])
    
    // 尝试展开父节点（简化处理：只展开到一级）
    // 实际需要根据 tree 结构计算需要展开的节点
    setExpandedKeys(prev => [...new Set([...prev, siteId])])
  }

  const handleCreateTopLevel = () => {
    setEditingSite({ parent_id: null })
    setFormMode('create')
    form.resetFields()
    setManagerInfo({ name: '', id: null, email: '', phone: '' })
    setManagerInput('')
    setViewMode('form')
    setLookupError({ message: '', visible: false })
  }

  const handleCreateSubSite = () => {
    if (!selectedSite) {
      message.warning('请先选择一个网点')
      return
    }
    Logger.state('SiteManagement', { action: 'handleCreateSubSite', parentId: selectedSite.id })
    setEditingSite({ parent_id: selectedSite.id })
    setFormMode('create')
    form.resetFields()
    setManagerInfo({ name: '', id: null, email: '', phone: '' })
    setManagerInput('')
    setViewMode('form')
    setLookupError({ message: '', visible: false })
  }

  const handleEdit = () => {
    if (!selectedSite) return
    setEditingSite({ ...selectedSite })
    setFormMode('edit')
    form.setFieldsValue(selectedSite)
    // Set manager display info if exists
    if (selectedSite.manager?.id) {
      setManagerInfo({ 
        name: selectedSite.manager.name, 
        id: selectedSite.manager.id,
        email: selectedSite.manager.email || '',
        phone: selectedSite.manager.phone || ''
      })
      setManagerInput(selectedSite.manager.email || selectedSite.manager.phone || '')
    } else {
      setManagerInfo({ name: '', id: null, email: '', phone: '' })
      setManagerInput('')
    }
    setViewMode('form')
    setLookupError({ message: '', visible: false })
  }

  const handleUserSelectFromDialog = (selectedUsers) => {
    if (!selectedUsers || selectedUsers.length === 0) return
    
    // Take the first user from the list (site manager typically only needs one)
    const firstUser = selectedUsers[0]
    setManagerInfo({
      name: firstUser.name || firstUser.user_name,
      id: firstUser.user_id || firstUser.id,
      email: firstUser.email || firstUser.user_email || '',
      phone: firstUser.phone || ''
    })
    setManagerInput(firstUser.email || firstUser.user_email || firstUser.phone || '')
    setUserDialogVisible(false)
    form.setFieldsValue({ manager_id: firstUser.user_id || firstUser.id })
  }

  const handleCreateUser = async () => {
    try {
      const values = await createUserForm.validateFields()
      setSaving(true)
      
      const result = await api.post('/iam/users', {
        email: values.email,
        phone: values.phone,
        name: values.name,
        role: 'site_manager',  // 添加角色字段
        password: values.password || undefined,
      })

      if (result.code === 20000 && result.data) {
        message.success('IAM用户创建成功')
        const user = result.data
        setManagerInfo({ name: user.name, id: user.id })
        form.setFieldsValue({ manager_id: user.id })
        setCreateUserModalVisible(false)
        createUserForm.resetFields()
        
        // 自动提交网点创建请求
        await handleSubmit()
      } else {
        message.error('创建失败：' + (result.message || '未知错误'))
      }
    } catch (err) {
      if (err.errorFields) return
      message.error('创建失败：' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!selectedSite) return
    try {
      await sitesApi.delete(selectedSite.id)
      message.success('删除成功')
      setSelectedSite(null)
      fetchSiteTree()
    } catch (err) {
      message.error('删除失败: ' + err.message)
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      
      Logger.state('SiteManagement', { action: 'handleSubmit', editingSite, values })
      
      let managerData = managerInfo
      
      // 获取 manager_id 值（优先使用 form 值，回退到 managerInput）
      const managerIdValue = values.manager_id || managerInput
      
      // 如果填写了 manager_id 但 managerInfo.id 为 null，需要验证
      // 添加检查：如果 identifier 是 UUID 格式，跳过验证（因为 lookup 不支持 ID 查询）
      const isUUID = managerIdValue && managerIdValue.match(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i)
      
      if (managerIdValue && !managerInfo.id && !isUUID) {
        setLookupLoading(true)
        setLookupError({ message: '', visible: false })
        
        try {
          const lookupIdentifier = managerIdValue
          const result = await api.get(`/iam/users/lookup?identifier=${encodeURIComponent(lookupIdentifier)}`)
          
          if (result.code === 20000 && result.data) {
            // 用户存在，更新 managerInfo（用于显示）
            const user = result.data
            managerData = {
              name: user.name || user.username || lookupIdentifier,
              id: user.id,
              email: user.email || '',
              phone: user.phone || ''
            }
            setManagerInfo(managerData)
            message.success(`已找到用户：${user.name || user.username}`)
          } else if (result.code === 40400) {
            // 用户不存在，显示错误指示
            setLookupError({
              message: '此用户不存在',
              visible: true
            })
            setLookupLoading(false)
            return // 阻止表单提交
          }
        } catch (lookupErr) {
          Logger.error('SITE', 'Lookup error:', lookupErr)
          message.error('用户查询失败：' + lookupErr.message)
          setLookupLoading(false)
          return
        } finally {
          setLookupLoading(false)
        }
      } else if (isUUID) {
        // 如果 identifier 是 UUID，直接使用（已经拥有用户 ID）
        managerData = {
          ...managerInfo,
          id: managerIdValue
        }
      }
      
      // 继续原有的创建/更新逻辑
      Logger.log('SITE', 'Creating siteData with managerData:', managerData)
      const siteData = {
        name: values.name,
        address: values.address || '',
        type: values.type || '',
        phone: values.phone || '',
        parent_id: editingSite?.parent_id,
        manager_id: managerData.id || null,
      }
      Logger.log('SITE', 'siteData:', siteData)
      
      setSaving(true)
      
      if (formMode === 'edit' && editingSite?.id) {
        Logger.log('SITE', 'Edit mode - updating site')
        await sitesApi.update(editingSite.id, siteData)
        message.success('更新成功')
        setViewMode('detail')
      } else {
        Logger.log('SITE', 'Create mode - creating site')
        const result = await sitesApi.create(siteData)
        
        // 如果是创建模式，自动选中新建的网点
        if (result.data?.id) {
          Logger.state('SiteManagement', { action: 'siteCreated', siteId: result.data.id })
          await refreshAndSelectTreeNode(result.data.id)
          
          // 设置选中的网点详情
          const newSite = { 
            id: result.data.id, 
            ...siteData,
            manager: managerData.id ? { id: managerData.id, name: managerData.name } : null
          }
          setSelectedSite(newSite)
          setViewMode('detail')
        } else {
          Logger.error('SITE', 'Create failed - result.data?.id is falsy:', result.data?.id)
        }
      }
      
      // 重置状态
      setManagerInfo({ name: '', id: null, email: '', phone: '' })
      setLookupError({ message: '', visible: false })
      form.resetFields()
      setEditingSite(null)
      
    } catch (err) {
      if (err.errorFields) return
      Logger.error('SITE', 'handleSubmit error:', err)
      message.error('操作失败: ' + err.message)
    } finally {
      setSaving(false)
      setLookupLoading(false)
    }
  }

  const renderEmptyState = () => (
    <div className="flex flex-col items-center justify-center h-full text-gray-400">
      <TeamOutlined style={{ fontSize: 64, marginBottom: 16 }} />
      <p className="text-lg mb-4">暂无网点数据</p>
      <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>
        创建顶级网点
      </Button>
    </div>
  )

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <div className="flex gap-4">
        <Card 
          title="网点结构" 
          className="w-1/3"
          extra={
            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>
              创建顶级网点
            </Button>
          }
        >
          {treeData.length === 0 ? (
            renderEmptyState()
          ) : (
            <Tree
              showIcon
              treeData={treeData}
              selectedKeys={selectedKeys}
              expandedKeys={expandedKeys}
              onSelect={onSelect}
              onExpand={setExpandedKeys}
              loadData={async (node) => {
                const children = await loadChildren(node.key)
                const updateTree = (nodes) => {
                  return nodes.map(n => {
                    if (n.key === node.key) {
                      return { ...n, children }
                    }
                    if (n.children) {
                      return { ...n, children: updateTree(n.children) }
                    }
                    return n
                  })
                }
                setTreeData(updateTree(treeData))
              }}
            />
          )}
        </Card>

        <div className="w-2/3">
          {viewMode === 'detail' && (
            <Card 
              title="网点详情" 
              extra={
                selectedSite && (
                  <Space>
                    <Button icon={<PlusOutlined />} onClick={handleCreateSubSite}>
                      创建下级网点
                    </Button>
                    <Button icon={<EditOutlined />} onClick={handleEdit}>
                      编辑
                    </Button>
                    <Popconfirm
                      title="确定要删除该网点吗？"
                      onConfirm={handleDelete}
                      okText="确定"
                      cancelText="取消"
                    >
                      <Button danger icon={<DeleteOutlined />}>
                        删除
                      </Button>
                    </Popconfirm>
                  </Space>
                )
              }
            >
              {selectedSite ? (
                <Tabs defaultActiveKey="info">
                  <Tabs.TabPane tab="基本信息" key="info">
                    <Descriptions column={2} bordered>
                      <Descriptions.Item label="网点名称">{selectedSite.name}</Descriptions.Item>
                      <Descriptions.Item label="网点类型">{selectedSite.type || '-'}</Descriptions.Item>
                      <Descriptions.Item label="地址" span={2}>{selectedSite.address || '-'}</Descriptions.Item>
                      <Descriptions.Item label="负责人">
                        {selectedSite.manager?.name || '-'}
                      </Descriptions.Item>
                      <Descriptions.Item label="父级网点">
                        {selectedSite.parent_id ? '有' : '顶级网点'}
                      </Descriptions.Item>
                    </Descriptions>
                  </Tabs.TabPane>
                  
                  <Tabs.TabPane tab="成员管理" key="members">
                    <SiteMemberManagement 
                      siteId={selectedSite.id} 
                      onRefresh={() => {}}
                    />
                  </Tabs.TabPane>
                </Tabs>
              ) : (
                <Empty description="请选择左侧网点查看详情" />
              )}
            </Card>
          )}
          
          {viewMode === 'form' && (
            <Card 
              title={formMode === 'edit' ? '编辑网点' : '创建网点'}
              extra={
                <Space>
                  <Button onClick={() => {
                    setViewMode('detail')
                    form.resetFields()
                    setManagerInfo({ name: '', id: null, email: '', phone: '' })
                    setLookupError({ message: '', visible: false })
                  }}>取消</Button>
                  <Button 
                    type="primary" 
                    onClick={handleSubmit}
                    loading={lookupLoading || saving}
                  >
                    提交
                  </Button>
                </Space>
              }
            >
              <Form form={form} layout="vertical" data-testid="site-form">
                <Form.Item
                  name="name"
                  label="网点名称"
                  rules={[{ required: true, message: '请输入网点名称' }]}
                  data-testid="site-form-name"
                >
                  <Input placeholder="请输入网点名称" />
                </Form.Item>

                <Form.Item
                  name="type"
                  label="网点类型"
                  data-testid="site-form-type"
                >
                  <Select placeholder="请选择网点类型" allowClear>
                    <Option value="直营店">直营店</Option>
                    <Option value="加盟店">加盟店</Option>
                    <Option value="合作店">合作店</Option>
                  </Select>
                </Form.Item>

                <Form.Item
                  name="address"
                  label="地址"
                  data-testid="site-form-address"
                >
                  <Input placeholder="请输入地址" />
                </Form.Item>

                <Form.Item
                  name="phone"
                  label="联系电话"
                  data-testid="site-form-phone"
                >
                  <Input placeholder="请输入联系电话" />
                </Form.Item>

                <Form.Item
                  name="manager_id"
                  label="负责人"
                >
                  {managerInfo.id ? (
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <UserOutlined style={{ fontSize: 20, color: '#52c41a' }} />
                      <span style={{ fontWeight: 500 }}>{managerInfo.name}</span>
                      {managerInfo.email && <span style={{ color: '#999' }}>({managerInfo.email})</span>}
                      <Button
                        type="link"
                        onClick={() => setUserDialogVisible(true)}
                        data-testid="site-form-change-manager"
                      >
                        更换
                      </Button>
                    </div>
                  ) : (
                    <Button
                      type="dashed"
                      icon={<TeamOutlined />}
                      onClick={() => setUserDialogVisible(true)}
                      data-testid="site-form-select-manager"
                    >
                      选择负责人
                    </Button>
                  )}
                </Form.Item>
              </Form>
            </Card>
          )}
        </div>
      </div>

      <UserSelectionDialog
        visible={userDialogVisible}
        onClose={() => setUserDialogVisible(false)}
        onConfirm={handleUserSelectFromDialog}
        merchantId="current-merchant-id"
        title="选择网点管理员"
      />

      <Modal
        title="创建IAM用户"
        open={createUserModalVisible}
        onCancel={() => {
          setCreateUserModalVisible(false)
          createUserForm.resetFields()
        }}
        onOk={handleCreateUser}
        confirmLoading={saving}
        width={500}
      >
        <Form form={createUserForm} layout="vertical" data-testid="create-user-form">
          <Form.Item
            name="phone"
            label="手机号"
            rules={[]}
            data-testid="create-user-form-phone"
          >
            <Input placeholder="请输入手机号（选填）" />
          </Form.Item>
          <Form.Item
            name="name"
            label="姓名"
            rules={[{ required: true, message: '请输入姓名' }]}
            data-testid="create-user-form-name"
          >
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { type: 'email', message: '请输入有效的邮箱地址' },
              { required: true, message: '请输入邮箱' }
            ]}
            data-testid="create-user-form-email"
          >
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            data-testid="create-user-form-password"
          >
            <Input.Password placeholder="留空则使用默认密码" />
          </Form.Item>
          <p style={{ color: '#999', fontSize: '12px' }}>
            该用户将自动加入当前租户组织
          </p>
        </Form>
      </Modal>
    </div>
  )
}
