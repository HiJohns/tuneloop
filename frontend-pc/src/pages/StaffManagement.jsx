import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag, Alert, Tabs, Radio, Checkbox } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined, UploadOutlined, SendOutlined, MailOutlined, ReloadOutlined } from '@ant-design/icons'
import { staffApi, sitesApi, iamApi } from '../services/api'
import { useLocation, useNavigate } from 'react-router-dom'
import UserEditDialog from '../components/UserEditDialog'

const { Option } = Select

export default function StaffManagement() {
  const navigate = useNavigate()
  const [staffList, setStaffList] = useState([])
  const [loading, setLoading] = useState(false)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10, total: 0 })
  const [searchParams, setSearchParams] = useState({ name: '', siteId: null })
  const [siteTree, setSiteTree] = useState([])
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [inlineFormVisible, setInlineFormVisible] = useState(false)
  const [createTab, setCreateTab] = useState('search')
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [editingUser, setEditingUser] = useState(null)
  const [userForm] = Form.useForm()
  const [createUserForm] = Form.useForm()
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [lockedSiteId, setLockedSiteId] = useState(null)

  const [conflictModalVisible, setConflictModalVisible] = useState(false)
  const [conflictUsers, setConflictUsers] = useState([])
  const [currentNewUser, setCurrentNewUser] = useState(null)
  const [syncLoading, setSyncLoading] = useState(false)
  const [userRole, setUserRole] = useState('')
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

        // Auto-sync IAM status when any user is still pending
        if (list.some(u => u.status === 'pending')) {
          iamApi.syncUsers().then(r => {
            if (r.code === 20000) fetchStaffList()
          }).catch(() => {})
        }
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

  const handleSyncUsersFromIAM = async () => {
    setSyncLoading(true)
    try {
      const result = await iamApi.syncUsers()
      if (result.code === 20000) {
        message.success(`同步完成: ${result.data.synced} 新增, ${result.data.skipped} 跳过`)
        fetchStaffList()
      } else {
        message.error('同步失败: ' + result.message)
      }
    } catch (err) {
      message.error('同步失败: ' + err.message)
    } finally {
      setSyncLoading(false)
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
        setCreateModalVisible(false)
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
        setCreateModalVisible(false)
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
    setCreateModalVisible(true)
  }

  const handleUpdateUser = async (values) => {
    try {
      const emailChanged = editingUser.email && values.email && values.email !== editingUser.email
      const result = await staffApi.updateUser(editingUser.id, values)
      if (result.code === 20000) {
        if (emailChanged) {
          try {
            await staffApi.updateIAMUser(editingUser.iam_sub || editingUser.id, {
              name: values.name,
              email: values.email,
              phone: values.phone,
            })
            message.success('用户更新成功，邮箱变更需确认后生效')
          } catch (iamError) {
            message.warning('用户更新成功，但邮箱变更请求发送失败')
          }
        } else {
          message.success('更新用户成功')
        }
        setEditModalVisible(false)
        setEditingUser(null)
        userForm.resetFields()
        fetchStaffList()
      }
    } catch (error) {
      message.error('更新用户失败: ' + error.message)
    }
  }

  const handleEditUser = (user) => {
    setEditingUser(user)
    userForm.setFieldsValue({
      name: user.name,
      email: user.email,
      phone: user.phone,
      site_id: user.site_id,
      position: user.position,
      user_type: user.user_type || 'staff'
    })
    setEditModalVisible(true)
  }

  const handleDeleteUser = (user) => {
    Modal.confirm({
      title: '删除确认',
      content: `确定要删除用户「${user.name}」吗？此操作不可恢复。`,
      okText: '确定删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          const result = await staffApi.batchDelete([user.id])
          if (result.code === 20000) {
            message.success('删除成功')
            fetchStaffList()
          } else {
            throw new Error(result.message || '删除失败')
          }
        } catch (error) {
          message.error('删除失败: ' + error.message)
        }
      }
    })
  }

  const handleResetPassword = async (userIds) => {
    try {
      const redirectUrl = window.location.origin
      const result = await staffApi.resetPassword(userIds, redirectUrl)
      if (result.code === 20000) {
        const { sent, skipped } = result.data
        if (skipped > 0) {
          message.success(`已发送 ${sent} 封重设密码邮件，${skipped} 个用户被跳过`)
        } else {
          message.success(`已成功发送 ${sent} 封重设密码邮件`)
        }
      } else {
        throw new Error(result.message || '发送失败')
      }
    } catch (error) {
      message.error('重设密码邮件发送失败: ' + error.message)
    }
  }

  const handleActivateUser = async (user) => {
    try {
      const result = await staffApi.activateUser(user.id)
      if (result.code === 20000) {
        message.success(`用户「${user.name}」激活成功`)
        handleSearch()
      } else {
        throw new Error(result.message || '激活失败')
      }
    } catch (error) {
      message.error('激活失败: ' + error.message)
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
      title: '用户类型',
      dataIndex: 'user_type',
      key: 'user_type',
      width: 100,
      render: (userType) => {
        const typeMap = {
          'staff': '员工',
          'admin': '管理员',
          'manager': '网点经理',
          'owner': '所有者'
        }
        return typeMap[userType] || '员工'
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
            onClick={() => handleResetPassword([record.id])}
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
      <Card 
        title="人员管理" 
        extra={
          <Space>
            {isMerchantAdmin && (
              <Button 
                onClick={handleSyncUsersFromIAM}
                loading={syncLoading}
                disabled={syncLoading}
                title="从 IAM 同步用户数据"
              >
                从 IAM 同步
              </Button>
            )}
            <Button
              icon={<UploadOutlined />}
              onClick={() => navigate('/staff/bulk-import')}
            >
              批量导入
            </Button>
            <Button 
              type="primary" 
              icon={<PlusOutlined />}
              onClick={() => setInlineFormVisible(!inlineFormVisible)}
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

      {/* 创建用户对话框 */}
      {/* 内嵌创建用户表单 */}
      {inlineFormVisible && (
        <Card className="mb-4" size="small">
          <Tabs activeKey={createTab} onChange={setCreateTab}>
            <Tabs.TabPane tab="搜索用户" key="search">
              <Form layout="inline" className="mb-3">
                <Form.Item style={{ flex: 1 }}>
                  <Input placeholder="输入用户名/邮箱/手机搜索" />
                </Form.Item>
                <Button type="primary">搜索</Button>
              </Form>
            </Tabs.TabPane>
            <Tabs.TabPane tab="创建用户" key="create">
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
                  </Select>
                </Form.Item>
                <Space>
                  <Button type="primary" htmlType="submit">创建用户</Button>
                  <Button onClick={() => { setInlineFormVisible(false); createUserForm.resetFields() }}>取消</Button>
                </Space>
              </Form>
            </Tabs.TabPane>
          </Tabs>
        </Card>
      )}

      {/* 编辑用户对话框 */}
      <Modal
        title="编辑用户"
        visible={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false)
          setEditingUser(null)
          userForm.resetFields()
        }}
        footer={null}
        width={600}
      >
        <UserEditDialog
          form={userForm}
          onSubmit={handleUpdateUser}
          onCancel={() => {
            setEditModalVisible(false)
            setEditingUser(null)
            userForm.resetFields()
          }}
          siteOptions={siteOptions}
          initialValues={editingUser}
        />
      </Modal>

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
    </div>
  )
}
