import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag, Alert, Tabs, Radio, Checkbox } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined, UploadOutlined, SendOutlined, MailOutlined, ReloadOutlined } from '@ant-design/icons'
import { staffApi, sitesApi } from '../services/api'
import { useLocation, useNavigate } from 'react-router-dom'

const { Option } = Select

export default function StaffManagement() {
  const navigate = useNavigate()
  const [staffList, setStaffList] = useState([])
  const [loading, setLoading] = useState(false)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10, total: 0 })
  const [searchParams, setSearchParams] = useState({ name: '', siteId: null })
  const [siteTree, setSiteTree] = useState([])
  const [viewMode, setViewMode] = useState('list') // 'list' | 'create'
  const [createTab, setCreateTab] = useState('search')
  const [searchKeyword, setSearchKeyword] = useState('')
  const [searchResults, setSearchResults] = useState([])
  const [searchLoading, setSearchLoading] = useState(false)
  const [debounceTimeout, setDebounceTimeout] = useState(null)
  const [createUserForm] = Form.useForm()
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [lockedSiteId, setLockedSiteId] = useState(null)

  const [conflictModalVisible, setConflictModalVisible] = useState(false)
  const [conflictUsers, setConflictUsers] = useState([])
  const [currentNewUser, setCurrentNewUser] = useState(null)
  const [userRole, setUserRole] = useState('')
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [editingUser, setEditingUser] = useState(null)
  const [editForm] = Form.useForm()
  const [isMerchantAdmin, setIsMerchantAdmin] = useState(false)
  const [selectedRowKeys, setSelectedRowKeys] = useState([])
  const [batchLoading, setBatchLoading] = useState(false)
  const location = useLocation()

  useEffect(() => {
    staffApi.getMe().then(res => {
      if (res.code === 20000 && res.data && res.data.site_id) {
        const businessRole = (res.data.business_role || '').toLowerCase()
        if (businessRole === 'site_admin' || businessRole === 'site_member') {
          createUserForm.setFieldsValue({ site_id: res.data.site_id })
          setLockedSiteId(res.data.site_id)
        }
      }
    }).catch(() => {})
  }, [createUserForm])

  useEffect(() => {
    const userInfo = localStorage.getItem('user_info')
    if (userInfo) {
      try {
        const info = JSON.parse(userInfo)
        setUserRole(info.role || '')
        setIsMerchantAdmin(info.tid && info.tid === info.oid)
      } catch (e) {
        setUserRole('')
      }
    }
  }, [])

  useEffect(() => {
    fetchStaffList()
    fetchSiteTree()
  }, [location])

  useEffect(() => {
    fetchStaffList()
  }, [pagination.current, pagination.pageSize, searchParams])

  const fetchStaffList = async () => {
    setLoading(true)
    try {
      const params = {
        page: pagination.current,
        page_size: pagination.pageSize,
        ...searchParams
      }
      const result = await staffApi.list(params)
      if (result.code === 20000) {
        const list = result.data?.list || []
        setStaffList(list)
        setPagination({
          ...pagination,
          total: result.data?.total || 0
        })
      }
    } catch (error) {
      message.error('加载人员列表失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const fetchSiteTree = async () => {
    try {
      const result = await sitesApi.getTree()
      if (result.code === 20000) {
        setSiteTree(result.data?.list || [])
      }
    } catch (error) {
      console.error('加载网点数据失败:', error)
    }
  }

  const convertSitesToOptions = (sites, parentPath = '') => {
    let options = []
    sites.forEach(site => {
      const path = parentPath ? `${parentPath} / ${site.name}` : site.name
      options.push({
        label: path,
        value: site.id,
        key: site.id
      })
      if (site.children && site.children.length > 0) {
        options = options.concat(convertSitesToOptions(site.children, path))
      }
    })
    return options
  }

  const handleSearch = (values) => {
    setSearchParams({
      name: values.name || '',
      site_id: values.siteId || null
    })
    setPagination({ ...pagination, current: 1 })
  }

  const handleTableChange = (newPagination) => {
    setPagination({
      ...pagination,
      current: newPagination.current,
      pageSize: newPagination.pageSize
    })
  }

  const handleCreateUser = async (values) => {
    try {
      const checkResult = await staffApi.checkUserExists(values.phone, values.email, values.username)
      if (checkResult.code === 20000 && checkResult.data?.exists) {
        setConflictUsers(checkResult.data.users || [])
        setCurrentNewUser(values)
        setConflictModalVisible(true)
        return
      }

      const result = await staffApi.createUser(values)
      if (result.code === 20000) {
        const initialPwd = result.data?.initial_password
        if (initialPwd) {
          Modal.info({
            title: '用户创建成功',
            width: 480,
            content: (
              <div>
                <p>以下为该用户的初始密码，仅展示一次，请妥善保存并告知用户：</p>
                <div style={{
                  padding: '12px 16px',
                  background: '#f5f5f5',
                  borderRadius: 4,
                  fontFamily: 'monospace',
                  fontSize: 18,
                  textAlign: 'center',
                  margin: '12px 0',
                  userSelect: 'all',
                }}>
                  {initialPwd}
                </div>
                <p style={{ fontSize: 12, color: '#999' }}>关闭后将无法再次查看。用户首次登录后建议立即修改密码。</p>
              </div>
            ),
            onOk() {
              navigator.clipboard?.writeText(initialPwd)
            },
            okText: '复制并关闭',
          })
        } else {
          message.success('创建用户成功')
        }
        setViewMode('list')
        createUserForm.resetFields()
        fetchStaffList()
      } else if (result.code === 40900) {
        const users = result.data || []
        const names = users.map(u => `${u.name}(${u.phone})`).join(', ')
        message.error(`创建用户失败：姓名、手机号或邮箱与以下用户冲突: ${names}`)
      }
    } catch (error) {
      message.error('创建用户失败: ' + error.message)
    }
  }

  const handleContinueCreate = async () => {
    try {
      const result = await staffApi.createUser(currentNewUser)
      if (result.code === 20000) {
        message.success('创建用户成功')
        setConflictModalVisible(false)
        createUserForm.resetFields()
        fetchStaffList()
      } else if (result.code === 40900) {
        const users = result.data || []
        const names = users.map(u => `${u.name}(${u.phone})`).join(', ')
        message.error(`创建用户失败：姓名、手机号或邮箱与以下用户冲突: ${names}`)
        setCurrentNewUser(null)
        return
      }
    } catch (error) {
      message.error('创建用户失败: ' + error.message)
    } finally {
      setCurrentNewUser(null)
    }
  }

  const handleCancelCreate = () => {
    setConflictModalVisible(false)
    setCurrentNewUser(null)
  }

  const handleEditUser = (record) => {
    setEditingUser(record)
    editForm.setFieldsValue({
      name: record.name,
      phone: record.phone,
      email: record.email,
      position: record.position,
      site_id: record.site_id,
    })
    setEditModalVisible(true)
  }

  const handleSubmitEdit = async () => {
    try {
      const values = await editForm.validateFields()
      const res = await staffApi.updateUser(editingUser.id, values)
      if (res.code === 20000) {
        message.success('编辑成功')
        setEditModalVisible(false)
        fetchStaffList()
      } else {
        message.error(res.message || '编辑失败')
      }
    } catch (err) {
      if (err.errorFields) return
      message.error('编辑失败: ' + (err.message || ''))
    }
  }

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的用户')
      return
    }
    Modal.confirm({
      title: '批量删除确认',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个用户吗？此操作不可恢复。`,
      okText: '确定删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setBatchLoading(true)
        try {
          const result = await staffApi.batchDelete(selectedRowKeys)
          if (result.code === 20000) {
            message.success(`已成功删除 ${result.data.deleted} 个用户`)
            setSelectedRowKeys([])
            fetchStaffList()
          } else {
            throw new Error(result.message || '批量删除失败')
          }
        } catch (error) {
          message.error('批量删除失败: ' + error.message)
        } finally {
          setBatchLoading(false)
        }
      }
    })
  }

  const handleBatchResetPassword = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要重设密码的用户')
      return
    }
    setBatchLoading(true)
    try {
      await handleResetPassword(selectedRowKeys)
      setSelectedRowKeys([])
    } finally {
      setBatchLoading(false)
    }
  }

  const handleRowSelection = {
    selectedRowKeys,
    onChange: (selectedKeys) => setSelectedRowKeys(selectedKeys)
  }

  const allColumns = [
    {
      title: '姓名',
      dataIndex: 'name',
      key: 'name',
      width: 120
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      key: 'email',
      width: 220,
      ellipsis: true
    },
    {
      title: '手机号',
      dataIndex: 'phone',
      key: 'phone',
      width: 120
    },
    {
      title: '归属网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 150,
      render: (siteName) => siteName || '-'
    },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 100,
      render: (role) => {
        const roleMap = { 'site_admin': '管理员', 'site_member': '成员', 'repair_technician': '维修师傅' }
        return roleMap[role] || role || '-'
      }
    },
    {
      title: '状态',
      key: 'status',
      width: 80,
      render: (_, record) => {
        if (!record.iam_sub) return <Tag color="red">未激活</Tag>
        if (record.status === 'pending') return <Tag color="orange">待确认</Tag>
        if (record.status === 'active') return <Tag color="green">正常</Tag>
        return <Tag color="red">禁用</Tag>
      }
    },
    {
      title: '操作',
      key: 'action',
      width: 280,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button 
            type="link" 
            size="small" 
            icon={<EditOutlined />}
            onClick={() => handleEditUser(record)}
          >
            编辑
          </Button>
          {!record.iam_sub && (
            <Button
              type="link"
              size="small"
              icon={<ReloadOutlined />}
              onClick={() => handleActivateUser(record)}
            >
              激活
            </Button>
          )}
          <Button
            type="link"
            size="small"
            icon={<MailOutlined />}
              onClick={() => navigate(`/staff/${record.id}/reset-password`, { state: { user: record } })}
          >
            重设密码
          </Button>
          <Popconfirm
            title={`确定删除用户「${record.name}」？`}
            onConfirm={() => handleDeleteUser(record)}
            okText="确定"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
            >
              删除
            </Button>
          </Popconfirm>
        </Space>
      )
    }
  ]

  // Hide site column for site-level roles (they only see their own site)
  const businessRole = localStorage.getItem('user_business_role') || ''
  const isSiteLevel = businessRole === 'site_admin' || businessRole === 'site_member'
  const columns = isSiteLevel
    ? allColumns.filter(col => col.key !== 'site_name')
    : allColumns

  const findSiteById = (sites, id) => {
    for (const site of sites) {
      if (site.id === id) return site
      if (site.children) {
        const found = findSiteById(site.children, id)
        if (found) return found
      }
    }
    return null
  }

  const siteOptions = convertSitesToOptions(siteTree)

  return (
    <div className="p-6">
      {viewMode === 'list' ? (
      <Card 
        title="人员管理" 
        extra={
          <Space>
            <Button
              icon={<UploadOutlined />}
              onClick={() => navigate('/staff/bulk-import')}
            >
              批量导入
            </Button>
            <Button 
              type="primary" 
              icon={<PlusOutlined />}
              onClick={() => setViewMode('create')}
            >
              创建用户
            </Button>
          </Space>
        }
      >
        <Form
          layout="inline"
          className="mb-4"
          onFinish={handleSearch}
        >
          <Form.Item name="name">
            <Input placeholder="搜索姓名" prefix={<SearchOutlined />} />
          </Form.Item>
          <Form.Item name="siteId">
            <Select 
              placeholder="选择网点" 
              style={{ width: 200 }}
              allowClear
            >
              {siteOptions.map(option => (
                <Option key={option.key} value={option.value}>
                  {option.label}
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              搜索
            </Button>
          </Form.Item>
        </Form>

        {selectedRowKeys.length > 0 && (
          <Alert
            className="mb-4"
            type="info"
            showIcon
            message={
              <div className="flex justify-between items-center">
                <span>已选择 <strong>{selectedRowKeys.length}</strong> 项</span>
                <Space>
                  <Button
                    size="small"
                    icon={<MailOutlined />}
                    onClick={handleBatchResetPassword}
                    loading={batchLoading}
                  >
                    重设密码
                  </Button>
                  <Button
                    size="small"
                    danger
                    onClick={handleBatchDelete}
                    loading={batchLoading}
                  >
                    批量删除
                  </Button>
                  <Button size="small" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
                </Space>
              </div>
            }
          />
        )}

        <style>{`.ant-table-row-inactive { background-color: #fff1f0 !important; } .ant-table-row-inactive:hover > td { background-color: #ffccc7 !important; }`}</style>
        <Table
          columns={columns}
          dataSource={staffList}
          rowKey="id"
          loading={loading}
          rowClassName={(record) => record.iam_sub ? '' : 'ant-table-row-inactive'}
          rowSelection={handleRowSelection}
          pagination={{
            ...pagination,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条记录`
          }}
          onChange={handleTableChange}
          scroll={{ x: 1400 }}
        />
      </Card>
      ) : (
      <Card 
        title="创建用户" 
        extra={
          <Button onClick={() => { setViewMode('list'); createUserForm.resetFields() }}>
            返回列表
          </Button>
        }
        className="mb-4" 
        size="small"
      >
        <Tabs 
          activeKey={createTab} 
          onChange={setCreateTab}
          items={[
            {
              key: 'search',
              label: '搜索用户',
              children: (
                <div className="mb-3">
                  <Input.Search
                    placeholder="输入用户名/邮箱/手机搜索"
                    value={searchKeyword}
                    onChange={e => handleSearchInput(e.target.value)}
                    loading={searchLoading}
                    enterButton
                  />
                  {searchKeyword.trim() && !searchLoading && (
                    <div className="mt-2" style={{ maxHeight: 240, overflow: 'auto' }}>
                      {searchResults.length > 0 ? (
                        searchResults.map(u => (
                          <div
                            key={u.id}
                            className="flex items-center justify-between p-2 hover:bg-gray-50 rounded cursor-pointer"
                          >
                            <div>
                              <span className="font-medium">{u.name}</span>
                              <span className="text-gray-400 ml-2">{u.phone}</span>
                              {u.email && <span className="text-gray-400 ml-2">{u.email}</span>}
                            </div>
                            <Tag color={u.iam_sub ? 'green' : 'orange'}>{u.iam_sub ? '已注册' : '未激活'}</Tag>
                          </div>
                        ))
                      ) : (
                        <div
                          className="text-center py-4 text-blue-500 cursor-pointer hover:text-blue-700"
                          onClick={() => setCreateTab('create')}
                        >
                          未找到匹配用户 → 创建新用户
                        </div>
                      )}
                    </div>
                  )}
                </div>
              ),
            },
            {
              key: 'create',
              label: '创建用户',
              children: (
                <Form
                  form={createUserForm}
                  layout="vertical"
                  onFinish={handleCreateUser}
                >
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                    <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
                      <Input placeholder="姓名" />
                    </Form.Item>
                    <Form.Item name="username" label="用户名">
                      <Input placeholder="用户名" />
                    </Form.Item>
                    <Form.Item name="email" label="邮箱">
                      <Input placeholder="邮箱（选填）" />
                    </Form.Item>
                    <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
                      <Input placeholder="手机号" />
                    </Form.Item>
                    <Form.Item name="auto_generate" label="密码设置" initialValue={true}>
                      <Radio.Group onChange={e => setAutoGenerate(e.target.value)}>
                        <Radio value={true}>自动生成</Radio>
                        <Radio value={false}>手动设置</Radio>
                      </Radio.Group>
                    </Form.Item>
                    {!autoGenerate && (
                      <Form.Item name="password" label="密码">
                        <Input.Password placeholder="8位+大写+小写+数字" />
                      </Form.Item>
                    )}
                  </div>
                  <Form.Item name="force_password_change" valuePropName="checked" initialValue={true}>
                    <Checkbox>首次登录时强制修改密码</Checkbox>
                  </Form.Item>
                  <Form.Item name="site_id" label="归属网点" rules={[{ required: true }]}>
                    <Select placeholder="选择网点" disabled={!!lockedSiteId}>
                      {siteOptions.map(o => (
                        <Option key={o.key} value={o.value}>{o.label}</Option>
                      ))}
                    </Select>
                  </Form.Item>
                  <Form.Item name="role" label="角色" initialValue="site_member">
                    <Select>
                      <Option value="site_admin">管理员</Option>
                      <Option value="site_member">成员</Option>
                      <Option value="repair_technician">维修师傅</Option>
                    </Select>
                  </Form.Item>
                  <Space>
                    <Button type="primary" htmlType="submit">创建用户</Button>
                    <Button onClick={() => { setViewMode('list'); createUserForm.resetFields() }}>取消</Button>
                  </Space>
                </Form>
              ),
            },
          ]}
        />
      </Card>
      )}

      {/* 冲突选择对话框 */}
      <Modal
        title="用户已存在"
        visible={conflictModalVisible}
        onCancel={() => setConflictModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setConflictModalVisible(false)}>
            取消
          </Button>,
          <Button key="continue" type="primary" onClick={handleContinueCreate}>
            继续创建
          </Button>
        ]}
      >
        <p>发现以下用户已存在：</p>
        <ul>
          {conflictUsers.map(user => (
            <li key={user.id}>
              {user.name} ({user.email || user.phone})
            </li>
          ))}
        </ul>
        <p>是否继续创建新用户？</p>
      </Modal>

      <Modal
        title="编辑用户"
        open={editModalVisible}
        onCancel={() => setEditModalVisible(false)}
        onOk={handleSubmitEdit}
        destroyOnClose
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="phone" label="手机" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input placeholder="请输入手机号" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          <Form.Item name="position" label="职位">
            <Input placeholder="请输入职位" />
          </Form.Item>
          <Form.Item name="site_id" label="归属网点" rules={[{ required: true, message: '请选择网点' }]}>
            <Select placeholder="选择网点">
              {siteOptions.map(o => (
                <Option key={o.key} value={o.value}>{o.label}</Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
