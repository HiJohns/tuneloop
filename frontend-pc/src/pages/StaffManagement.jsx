import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons'
import { staffApi, sitesApi, iamAdminApi, iamApi } from '../services/api'
import { useLocation } from 'react-router-dom'
import UserCreateDialog from '../components/UserCreateDialog'
import UserEditDialog from '../components/UserEditDialog'

const { Option } = Select

export default function StaffManagement() {
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
  const location = useLocation()

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
      // First check if user exists
      const checkResult = await staffApi.checkUserExists(values.email || values.phone)
      if (checkResult.code === 20000 && checkResult.data?.exists) {
        // Show conflict dialog
        setConflictUsers(checkResult.data.users || [])
        setCurrentNewUser(values)
        setConflictModalVisible(true)
        setCreateModalVisible(false)
        return
      }

      // Create user
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
      dataIndex: 'site_id',
      key: 'site_id',
      width: 150,
      render: (siteId) => {
        if (!siteId) return '-';
        const site = findSiteById(siteTree, siteId);
        return site ? site.name : '-';
      }
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
        return status === 'active' ? 
          <Tag color="green">正常</Tag> : 
          <Tag color="red">禁用</Tag>
      }
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
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

        <Table
          columns={columns}
          dataSource={staffList}
          rowKey="id"
          loading={loading}
          pagination={{
            ...pagination,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条记录`
          }}
          onChange={handleTableChange}
          scroll={{ x: 1200 }}
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

