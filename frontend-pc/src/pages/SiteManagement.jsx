import { useState, useEffect } from 'react'
import { Card, Tree, Descriptions, Button, Modal, Form, Input, Select, message, Spin, Empty, Space, Popconfirm, Tabs, Tag } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, TeamOutlined, UserOutlined, EnvironmentOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { sitesApi, iamApi } from '../services/api'
import Logger from '../utils/logger'
import SiteMemberManagement from '../components/SiteMemberManagement'
import ManagerSelector from '../components/ManagerSelector'

const { Option } = Select

export default function SiteManagement() {
  const navigate = useNavigate()
  const [treeData, setTreeData] = useState([])
  const [selectedSite, setSelectedSite] = useState(null)
  const [editingSite, setEditingSite] = useState(null)
  const [loading, setLoading] = useState(true)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [creatingManager, setCreatingManager] = useState(false)
  const [managerInfo, setManagerInfo] = useState({ name: '', id: null, email: '', phone: '' })
  const [createUserModalVisible, setCreateUserModalVisible] = useState(false)
  const [createUserForm] = Form.useForm()
  const [viewMode, setViewMode] = useState('detail') // 'detail' | 'form'
  const [formMode, setFormMode] = useState('create') // 'create' | 'edit'
  const [lookupLoading, setLookupLoading] = useState(false)
  const [lookupError, setLookupError] = useState({ message: '', visible: false })
  const [expandedKeys, setExpandedKeys] = useState([])
  const [selectedKeys, setSelectedKeys] = useState([])
  const [syncLoading, setSyncLoading] = useState(false)
  const [userRole, setUserRole] = useState('')
  const [searchText, setSearchText] = useState('')
  const [searchResults, setSearchResults] = useState([])
  const [conflictMessage, setConflictMessage] = useState('')

  useEffect(() => {
    // Load user role from localStorage
    const userInfo = localStorage.getItem('user_info')
    if (userInfo) {
      try {
        const info = JSON.parse(userInfo)
        setUserRole(info.role || '')
      } catch (e) {
        setUserRole('')
      }
    }
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

  const handleSyncFromIAM = async () => {
    setSyncLoading(true)
    try {
      const result = await iamApi.syncOrganizations()
      if (result.code === 20000) {
        message.success(`同步成功：新增 ${result.data.synced} 个组织，跳过 ${result.data.skipped} 个`)
        // Refresh the tree after sync
        await fetchSiteTree()
      } else {
        message.error('同步失败：' + (result.message || '未知错误'))
      }
    } catch (err) {
      message.error('同步失败：' + err.message)
    } finally {
      setSyncLoading(false)
    }
  }

  const convertToTreeNode = (site) => ({
    key: site.id,
    title: site.name,
    icon: site.hasChildren ? <TeamOutlined /> : <EnvironmentOutlined />,
    data: site,
    isLeaf: !site.hasChildren,
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
    
    const result = await sitesApi.getTree()
    const sites = result.data?.list || []
    setTreeData(sites.map(site => convertToTreeNode(site)))
    setSelectedKeys([siteId])
    
    const freshSite = sites.find(s => s.id === siteId)
    if (freshSite) setSelectedSite(freshSite)
    
    setExpandedKeys(prev => [...new Set([...prev, siteId])])
  }

  const resetManagerState = () => {
    setSearchResults([])
    setConflictMessage('')
  }

  const handleCreateTopLevel = () => {
    setEditingSite({ parent_id: null })
    setFormMode('create')
    form.resetFields()
    setManagerInfo({ name: '', id: null, email: '', phone: '' })
    setViewMode('form')
    setLookupError({ message: '', visible: false })
    resetManagerState()
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
    setViewMode('form')
    setLookupError({ message: '', visible: false })
    resetManagerState()
  }

  const handleEdit = () => {
    if (!selectedSite) return
    setEditingSite({ ...selectedSite })
    setFormMode('edit')
    form.setFieldsValue(selectedSite)
    resetManagerState()
    if (selectedSite.manager?.id) {
      setManagerInfo({ 
        name: selectedSite.manager.name, 
        id: selectedSite.manager.id,
        email: selectedSite.manager.email || '',
        phone: selectedSite.manager.phone || ''
      })
    } else {
      setManagerInfo({ name: '', id: null, email: '', phone: '' })
    }
    setViewMode('form')
    setLookupError({ message: '', visible: false })
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
      
      const siteData = {
        name: values.name,
        address: values.address || '',
        type: values.type || '',
        phone: values.phone || '',
        parent_id: editingSite?.parent_id,
      }
      
      if (managerInfo.id) {
        siteData.manager_id = managerInfo.id
      }
      
      Logger.log('SITE', 'siteData:', siteData)
      
      setSaving(true)
      setConflictMessage('')
      
      let result;
      if (formMode === 'edit' && editingSite?.id) {
        result = await sitesApi.update(editingSite.id, siteData);
      } else {
        result = await sitesApi.create(siteData);
      }

      if (result.code === 40901 && result.data?.conflicts) {
        const options = result.data.conflicts.map(u => ({
          value: u.id,
          label: `${u.name || ''} (${u.email || ''}) - ${(u.matched_fields || []).join(', ')}`,
          user: u,
        }))
        setSearchResults(options)
        setConflictMessage('以下用户已存在，请选择：')
        return
      }
      
      if (result.code === 40900) {
        message.error(result.message || '网点名称已存在')
        return
      }
      
      if (formMode === 'edit') {
        if (result.data?.updated) {
          message.success('更新成功')
          setViewMode('detail')
        }
      } else if (result.data?.id) {
        Logger.state('SiteManagement', { action: 'siteCreated', siteId: result.data.id })
        await refreshAndSelectTreeNode(result.data.id)
        setViewMode('detail')
      }
      
      form.resetFields()
      setEditingSite(null)
      
    } catch (err) {
      if (err.errorFields) return
      Logger.error('SITE', 'handleSubmit error:', err)
      message.error('操作失败: ' + err.message)
    } finally {
      setSaving(false)
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
      <div className="flex gap-4" style={{ minHeight: 'calc(100vh - 160px)' }}>
        <Card 
          title="网点结构" 
          className="w-1/3"
          extra={
            <Space>
              <Button 
                type="primary" 
                size="small" 
                icon={<PlusOutlined />} 
                onClick={handleCreateTopLevel}
              >
                创建顶级网点
              </Button>
              <Button
                size="small"
                icon={<UploadOutlined />}
                onClick={() => navigate('/organization/sites/bulk-import')}
              >
                批量导入
              </Button>
              {(userRole === 'ADMIN' || userRole === 'OWNER' || userRole === 'admin' || userRole === 'owner') && (
                <Button 
                  type="default" 
                  size="small" 
                  onClick={handleSyncFromIAM}
                  loading={syncLoading}
                  disabled={syncLoading}
                  title="从 IAM 同步组织数据"
                >
                  从 IAM 同步
                </Button>
              )}
            </Space>
          }
        >
          {treeData.length === 0 ? (
            renderEmptyState()
          ) : (
            <>
              <Input
                prefix={<SearchOutlined />}
                placeholder="搜索网点"
                allowClear
                size="small"
                style={{ marginBottom: 12 }}
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
              />
              <Tree
                showIcon
                treeData={treeData}
                selectedKeys={selectedKeys}
                expandedKeys={expandedKeys}
                onSelect={onSelect}
                onExpand={setExpandedKeys}
filterTreeNode={(node) => {
                  if (!searchText) return true
                  return String(node.title).toLowerCase().includes(searchText.toLowerCase())
                }}
                loadData={async (node) => {
                  const children = await loadChildren(node.key)
                  const updateTree = (nodes) => {
                    return nodes.map((n, i) => {
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
            </>
          )}
        </Card>

        <div className="flex-1">
          {viewMode === 'detail' && selectedSite ? (
            <>
              <Card className="mb-4">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: 12 }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Space align="center" wrap>
                      <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>{selectedSite.name}</h2>
                      {selectedSite.type && <Tag color="blue">{selectedSite.type}</Tag>}
                    </Space>
                  </div>
                  <Space wrap>
                    <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateSubSite}>创建下级网点</Button>
                    <Button icon={<EditOutlined />} onClick={handleEdit}>编辑</Button>
                    <Popconfirm
                      title="确定要删除该网点吗？"
                      onConfirm={handleDelete}
                      okText="确定"
                      cancelText="取消"
                    >
                      <Button danger icon={<DeleteOutlined />}>删除</Button>
                    </Popconfirm>
                  </Space>
                </div>
              </Card>

              <Card>
                <Tabs defaultActiveKey="info">
                  <Tabs.TabPane tab="基本信息" key="info">
                    <Descriptions column={2} bordered size="small">
                      <Descriptions.Item label="网点ID">{selectedSite.id}</Descriptions.Item>
                      <Descriptions.Item label="网点名称">{selectedSite.name}</Descriptions.Item>
                      <Descriptions.Item label="网点类型">{selectedSite.type || '-'}</Descriptions.Item>
                      <Descriptions.Item label="地址" span={2}>{selectedSite.address || '-'}</Descriptions.Item>
                      <Descriptions.Item label="联系电话">{selectedSite.phone || '-'}</Descriptions.Item>
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
              </Card>
            </>
          ) : viewMode === 'detail' && !selectedSite ? (
            <Empty description="请选择左侧网点查看详情" />
          ) : null}
          
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
                    setConflictMessage('')
                  }}>取消</Button>
                  <Button 
                    type="primary" 
                    onClick={handleSubmit}
                    loading={lookupLoading || saving || creatingManager}
                    disabled={lookupLoading || saving || creatingManager}
                  >
                    提交
                  </Button>
                </Space>
              }
            >
              <Form form={form} layout="vertical" data-testid="site-form" onFinish={handleSubmit}>
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

                <Form.Item label="负责人">
                  <ManagerSelector
                    value={managerInfo}
                    createReason="网点负责人"
                    onCreatingChange={setCreatingManager}
                    onChange={(info) => {
                      if (info.id) {
                        setManagerInfo({ name: info.name, id: info.id, email: info.email || '', phone: info.phone || '', username: info.username || '' })
                      } else {
                        setManagerInfo({ name: '', id: null, email: '', phone: '', username: '' })
                      }
                    }}
                    conflictMessage={conflictMessage}
                    conflictOptions={searchResults}
                  />
                </Form.Item>
              </Form>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
