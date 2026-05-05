import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag, Alert } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined, UploadOutlined, SendOutlined, MailOutlined } from '@ant-design/icons'
import { staffApi, sitesApi, iamAdminApi, iamApi } from '../services/api'
import { useLocation, useNavigate } from 'react-router-dom'
import UserCreateDialog from '../components/UserCreateDialog'
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
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [editingUser, setEditingUser] = useState(null)
  const [userForm] = Form.useForm()
  const [createUserForm] = Form.useForm()
  const [positionOptions, setPositionOptions] = useState(['店长', '店员', '库管', '维修师'])
  const [conflictModalVisible, setConflictModalVisible] = useState(false)
  const [conflictUsers, setConflictUsers] = useState([])
  const [currentNewUser, setCurrentNewUser] = useState(null)
  const [syncLoading, setSyncLoading] = useState(false)
  const [userRole, setUserRole] = useState('')
  const [selectedRowKeys, setSelectedRowKeys] = useState([])
  const [batchLoading, setBatchLoading] = useState(false)
  const location = useLocation()

  useEffect(() => {
    const userInfo = localStorage.getItem('user_info')
    if (userInfo) {
      try {
        const info = JSON.parse(userInfo)
        setUserRole(info.role || '')
      } catch (e) {
        setUserRole('')
      }
    }
  }, [])

  useEffect(() => {
    fetchStaffList()
    fetchSiteTree()
    fetchPositions()
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
        setStaffList(result.data?.list || [])
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

  const fetchPositions = async () => {
    try {
      const result = await iamAdminApi.listPositions()
      if (result.code === 20000 && result.data?.list) {
        setPositionOptions(result.data.list.map(p => p.name))
      }
    } catch (error) {
      console.error('加载职位数据失败:', error)
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
      siteId: values.siteId || null
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
      const checkResult = await staffApi.checkUserExists(values.email || values.phone)
      if (checkResult.code === 20000 && checkResult.data?.exists) {
        setConflictUsers(checkResult.data.users || [])
        setCurrentNewUser(values)
        setConflictModalVisible(true)
        setCreateModalVisible(false)
        return
      }

      const result = await staffApi.createUser(values)
      if (result.code === 20000) {
        message.success('创建用户成功')
        setCreateModalVisible(false)
        createUserForm.resetFields()
        fetchStaffList()
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

  const handleResendConfirmation = async (userIds) => {
    try {
      const result = await staffApi.resendConfirmation(userIds)
      if (result.code === 20000) {
        const { sent, skipped } = result.data
        if (skipped > 0) {
          message.success(`已发送 ${sent} 封确认邮件，${skipped} 个用户被跳过（非待确认状态）`)
        } else {
          message.success(`已成功发送 ${sent} 封确认邮件`)
        }
      } else {
        throw new Error(result.message || '发送失败')
      }
    } catch (error) {
      message.error('重发确认邮件失败: ' + error.message)
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

  const handleBatchResendConfirmation = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要重发确认邮件的用户')
      return
    }
    setBatchLoading(true)
    try {
      await handleResendConfirmation(selectedRowKeys)
      setSelectedRowKeys([])
    } finally {
      setBatchLoading(false)
    }
  }

  const handleRowSelection = {
    selectedRowKeys,
    onChange: (selectedKeys) => setSelectedRowKeys(selectedKeys)
  }

  const columns = [
    {
      title: '姓名',
      dataIndex: 'name',
      key: 'name',
      width: 120
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      key: 'email'
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
      title: '职位',
      dataIndex: 'position',
      key: 'position',
      width: 100,
      render: (position) => position || '-'
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
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status) => {
        if (status === 'active') return <Tag color="green">正常</Tag>
        if (status === 'pending') return <Tag color="orange">待确认</Tag>
        return <Tag color="red">禁用</Tag>
      }
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
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
          {record.status === 'pending' && (
            <Button
              type="link"
              size="small"
              icon={<MailOutlined />}
              onClick={() => handleResendConfirmation([record.id])}
            >
              重发确认
            </Button>
          )}
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
            {(userRole === 'ADMIN' || userRole === 'OWNER' || userRole === 'admin' || userRole === 'owner') && (
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
              onClick={() => setCreateModalVisible(true)}
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
                    onClick={handleBatchResendConfirmation}
                    loading={batchLoading}
                  >
                    重发确认邮件
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

        <Table
          columns={columns}
          dataSource={staffList}
          rowKey="id"
          loading={loading}
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
      <Modal
        title="创建用户"
        visible={createModalVisible}
        onCancel={() => {
          setCreateModalVisible(false)
          createUserForm.resetFields()
        }}
        footer={null}
        width={600}
      >
        <UserCreateDialog
          form={createUserForm}
          onSubmit={handleCreateUser}
          onCancel={() => {
            setCreateModalVisible(false)
            createUserForm.resetFields()
          }}
          siteOptions={siteOptions}
          positionOptions={positionOptions}
        />
      </Modal>

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
          positionOptions={positionOptions}
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
