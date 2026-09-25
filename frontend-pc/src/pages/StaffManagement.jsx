import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag, Alert, Radio, Checkbox } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined, UploadOutlined, SendOutlined, MailOutlined, ReloadOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import PhotoUploader from '../components/PhotoUploader'
import 'react-quill/dist/quill.snow.css'
import { staffApi, sitesApi, personnelApi } from '../services/api'
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
  const [userType, setUserType] = useState('site_staff') // #2068: 用户类型
  const [createPhotoFile, setCreatePhotoFile] = useState(null) // #2073: 创建照片文件（头像，创建后上传）
  const [createPhotoUrl, setCreatePhotoUrl] = useState('') // #2073: 本地预览 URL（选择即预览）
  const [createUserForm] = Form.useForm()
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [lockedSiteId, setLockedSiteId] = useState(null)

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

  // #2065: 人员管理三视图——数据源按角色感知（merchant/site/system）；
  // 列裁剪沿用文件既有 isSiteLevel 逻辑（按 site_name 键过滤，覆盖 site_member）
  const fetchStaffList = async () => {
    setLoading(true)
    try {
      const params = {
        page: pagination.current,
        page_size: pagination.pageSize,
        ...searchParams
      }
      const result = await personnelApi.list(params)
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
      site_id: values.siteId || null,
      ...(values.direct ? { direct: 'true' } : {}) // #2067: 只看直属员工
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
        // #2052：撞库 → 不再硬报冲突，改为向既有账户发送「加入邀请通知」，
        // 由被邀请人本人在「系统消息」接受/拒绝（一人一号，禁止管理员代挂）。
        const users = checkResult.data.users || []
        const who = users[0]?.name || users[0]?.phone || users[0]?.email || ''
        const identifier = values.phone || values.email || ''
        const invite = await staffApi.sendStaffInvite(values.site_id, {
          identifier,
          role: values.role || 'site_member',
        })
        if (invite.code === 20100) {
          Modal.info({
            title: '已发送邀请通知',
            width: 480,
            content: (
              <div>
                <p>该手机号/邮箱已有账户{who ? `（${who}）` : ''}。</p>
                <p>已向其发送<b>邀请通知</b>，请其打开「系统消息」选择<b>接受</b>或<b>拒绝</b>。</p>
                <p style={{ fontSize: 12, color: '#999' }}>接受后即绑定为本人账户成员，无需管理员代为创建。</p>
              </div>
            ),
          })
          setViewMode('list')
          createUserForm.resetFields()
        } else {
          message.error(invite.message || '发送邀请失败')
        }
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
        // #2073: 全类型照片 → 头像（创建后经管理员媒体管线上传）
        if (result.data?.id && createPhotoFile) {
          try {
            const fd = new FormData()
            fd.append('file', createPhotoFile)
            await staffApi.uploadUserAvatar(result.data.id, fd)
          } catch (e) {
            message.warning('照片上传失败，可在后续编辑中重试')
          }
        }
        setCreatePhotoFile(null)
        setCreatePhotoUrl('')
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
      title: '所属网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 150,
      render: (siteName) => siteName || '-'
    },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
      width: 120,
      render: (position, record) => {
        if (position) return position
        const roleMap = { 'site_admin': '管理员', 'site_member': '成员', 'STAFF': '成员', 'repair_technician': '维修师傅', 'WORKER': '员工' }
        return record.is_technician ? '维修师傅' : (roleMap[record.role] || record.role || '-')
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
          {/* #2067: 只看直属员工（商户直属+维修师傅）；仅商户管理员可见 */}
          {isMerchantAdmin && (
            <Form.Item name="direct" valuePropName="checked">
              <Checkbox>只看直属员工（含维修师傅）</Checkbox>
            </Form.Item>
          )}
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
        {/* #2068: 去「搜索用户」Tab（死 Tab：state 从未写入）；直接展示创建表单 */}
        <Form
          form={createUserForm}
          layout="vertical"
          onFinish={handleCreateUser}
        >
          {/* #2068: 用户类型置首（不同类型后续可扩展字段） */}
          <Form.Item name="user_type" label="用户类型" initialValue="site_staff" rules={[{ required: true }]}>
            <Radio.Group onChange={e => setUserType(e.target.value)}>
              <Radio value="site_staff">网点员工</Radio>
              <Radio value="repair_technician">维修师傅</Radio>
              <Radio value="merchant_direct">商户直属员工</Radio>
            </Radio.Group>
          </Form.Item>

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

          {/* 网点员工：归属网点 + 角色 */}
          {userType === 'site_staff' && (
            <>
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
            </>
          )}

          {/* 商户直属员工：职位 */}
          {userType === 'merchant_direct' && (
            <Form.Item name="position" label="职位">
              <Input placeholder="如：客服 / 运营" />
            </Form.Item>
          )}

          {/* #2073: 简介（富文本）与照片对**所有类型**开放（照片=头像，创建后经媒体管线上传） */}
          <Form.Item name="bio" label="简介（富文本）">
            <ReactQuill theme="snow" placeholder="如：钢琴维修 12 年 · 小提琴维修 8 年" style={{ background: '#fff' }} />
          </Form.Item>
          <Form.Item label="照片（头像）">
            <PhotoUploader
              photoUrl={createPhotoUrl}
              onSelect={(f) => { setCreatePhotoFile(f); setCreatePhotoUrl(URL.createObjectURL(f)) }}
            />
            <div style={{ fontSize: 12, color: '#999', marginTop: 4 }}>
              照片将在创建成功后上传；也可稍后在用户编辑中更换。
            </div>
          </Form.Item>

          <Form.Item name="force_password_change" valuePropName="checked" initialValue={true}>
            <Checkbox>首次登录时强制修改密码</Checkbox>
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">创建用户</Button>
            <Button onClick={() => { setViewMode('list'); createUserForm.resetFields() }}>取消</Button>
          </Space>
        </Form>

      </Card>
      )}

      {/* #2052：原「用户已存在」冲突选择对话框已移除 —— 改为发送邀请通知 */}

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
