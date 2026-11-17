import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, message, Spin, Space, Popconfirm, Tag, Alert, Checkbox, Badge } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, SearchOutlined, UploadOutlined, SendOutlined, MailOutlined, ReloadOutlined, TeamOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import PhotoUploader from '../components/PhotoUploader'
import PasswordSettingFields from '../components/PasswordSettingFields'
import InitialPasswordModal from '../components/InitialPasswordModal'
import { resolveCreateFeedback } from '../utils/createFeedback'
import 'react-quill/dist/quill.snow.css'
import { staffApi, sitesApi, personnelApi, invitesApi } from '../services/api'
import { resolveErrorMessage } from '../services/errorMessages'
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
  const [userType, setUserType] = useState('merchant_direct') // #2390: 仅保留商户直属员工
  const [createPhotoFile, setCreatePhotoFile] = useState(null) // #2073: 创建照片文件（头像，创建后上传）
  const [createPhotoUrl, setCreatePhotoUrl] = useState('') // #2073: 本地预览 URL（选择即预览）
  const [createFormKey, setCreateFormKey] = useState(0) // #2074: 创建表单重挂载 key（清空 ReactQuill 等非受控残留）
  const [createUserForm] = Form.useForm()
  // #2372：密码模式（默认自动生成 + 强制改密，D1）；邀请模式仅「有邮箱且未设密码」时由后端走邮件。
  const [createAutoGenerate, setCreateAutoGenerate] = useState(true)
  const [createForcePasswordChange, setCreateForcePasswordChange] = useState(true)
  const [initialPasswords, setInitialPasswords] = useState([])
  const [passwordModalOpen, setPasswordModalOpen] = useState(false)
  const [lockedSiteId, setLockedSiteId] = useState(null)

  const [userRole, setUserRole] = useState('')
  const [editModalVisible, setEditModalVisible] = useState(false)
  const [editingUser, setEditingUser] = useState(null)
  const [editForm] = Form.useForm()
  const [isMerchantAdmin, setIsMerchantAdmin] = useState(false)
  const [invitePending, setInvitePending] = useState(0) // #2114 待审批徽标
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
    // #2114: 待审批徽标（无权限时静默忽略）
    invitesApi.pendingCount().then(res => {
      if (res.code === 20000) setInvitePending(res.data?.count || 0)
    }).catch(() => {})
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

  // #2072: 创建表单状态重置（表单字段 + 类型 + 照片预览）；四出口统一调用
  // #2390: 默认类型改为 merchant_direct（网点员工走网点页）
  // #2400 D1：本函数**不触碰**一次性密码弹窗状态——成功分支先开后关会把弹窗立即关掉。
  const resetCreateForm = () => {
    createUserForm.resetFields()
    setCreateAutoGenerate(true)
    setCreateForcePasswordChange(true)
    setUserType('merchant_direct')
    setCreatePhotoFile(null)
    setCreatePhotoUrl('')
    setCreateFormKey(k => k + 1) // #2074: 强制重挂载（ReactQuill 忽略 value=undefined，仅 resetFields 清不掉编辑器）
  }

  // 全量重置（表单 + 一次性弹窗）；用于入口/取消/无弹窗路径，弹窗关闭由 onClose 清理。
  const resetCreateState = () => {
    resetCreateForm()
    setInitialPasswords([])
    setPasswordModalOpen(false)
  }

  const handleCreateUser = async (values) => {
    try {
      // #2372：无邮箱时必须给出登录名（后端 #2376 以 username 为唯一必填登录标识）。
      if (!values.username?.trim()) {
        message.error('请填写登录名')
        return
      }
      const payload = {
        ...values,
        // #2401：表单不再提供员工类型选择；固定为商户直属员工（后端契约不变）。
        user_type: 'merchant_direct',
        username: values.username.trim(),
        auto_generate: createAutoGenerate,
        force_password_change: createForcePasswordChange,
      }
      if (values.email?.trim()) payload.email = values.email.trim()
      else delete payload.email
      if (createAutoGenerate) delete payload.password
      const checkResult = await staffApi.checkUserExists(values.phone, values.email, values.username)
      if (checkResult.code === 20000 && checkResult.data?.exists) {
        // #2402 Rev.2：撞库不自动发邀请；先提示冲突，由管理员显式选择。
        const users = checkResult.data.users || []
        const existing = users[0] || {}
        const who = existing.name || existing.phone || existing.email || ''
        const identifier = values.phone || values.email || values.username || ''
        let merchantId = null
        if (userType === 'merchant_direct') {
          try {
            const m = await invitesApi.myMerchant()
            if (m.code === 20000) merchantId = m.data?.id || null
          } catch { /* 无商户上下文 → 禁用邀请按钮 */ }
        }
        const canInvite = userType === 'merchant_direct'
          ? !!merchantId
          : (userType === 'site_staff' ? !!values.site_id : false)
        const inviteHint = userType === 'repair_technician'
          ? '维修师请到「直属成员管理」邀请，或让本人登录后加入。'
          : (canInvite
            ? '将向该账号发送「加入邀请」，由本人在系统消息中接受。'
            : '当前上下文无法发送邀请，请到「负责人管理 / 邀请管理」操作。')
        Modal.confirm({
          title: '该手机号/邮箱已注册',
          width: 520,
          content: (
            <div>
              <p>已有账户{who ? `（${who}）` : ''}使用该手机号/邮箱/登录名。</p>
              <p>{inviteHint}</p>
            </div>
          ),
          okText: '发送加入邀请',
          okButtonProps: { disabled: !canInvite },
          cancelText: '修改信息',
          onOk: async () => {
            if (!canInvite) return
            const resp = userType === 'merchant_direct'
              ? await invitesApi.inviteToMerchant(merchantId, { identifier, kind: 'merchant_staff' })
              : await staffApi.sendStaffInvite(values.site_id, { identifier, role: values.role || 'site_member' })
            if (resp.code === 20100) {
              message.success('邀请已发出，待对方接受')
              resetCreateState()
              setViewMode('list')
            } else {
              message.error(resp.message || '发送邀请失败')
            }
          },
          onCancel: () => {
            // 修改信息：清空冲突字段并聚焦，不发任何请求。
            if (values.phone && existing.phone === values.phone) createUserForm.setFieldsValue({ phone: '' })
            else if (values.email && existing.email === values.email) createUserForm.setFieldsValue({ email: '' })
            else if (values.username && existing.username === values.username) createUserForm.setFieldsValue({ username: '' })
            createUserForm.scrollToField('phone')
            setTimeout(() => document.getElementById('staff-create_phone')?.focus(), 0)
          },
        })
        return
      }

      const result = await staffApi.createUser(payload)
      if (result.code === 20000) {
        // #2372：密码模式（手动/自动生成）→ 一次性展示；否则保留邀请文案（有邮箱走 IAM 邮件）。
        // #2388：按创建通道分支（禁止再用「是否有 initial_password」猜模式）。
        const feedback = resolveCreateFeedback({
          autoGenerate: createAutoGenerate,
          password: values.password,
          email: values.email,
          username: values.username,
        })
        // #2400：成功分支不得在打开弹窗后调用会清空弹窗的 resetCreateState。
        let passwordModalOpened = false
        if (feedback.kind === 'password_auto') {
          const plain = result.data?.initial_password
          const list = (result.data?.initial_passwords || []).map(p => ({
            identifier: p.identifier || p.email || p.username || '',
            password: p.password,
          }))
          if (plain) {
            list.push({ identifier: values.username || values.email || '', password: plain })
          }
          if (!feedback.identifier) {
            console.error('[create-feedback] password_auto without identifier')
          }
          setInitialPasswords(list)
          setPasswordModalOpen(true)
          passwordModalOpened = list.length > 0
        } else if (feedback.kind === 'password_manual') {
          message.success('已创建，账号可用用户名 + 你设置的密码登录')
        } else if (feedback.kind === 'invite') {
          Modal.info({
            title: '已创建并发送邀请',
            width: 480,
            content: (
              <div>
                <p><b>邀请邮件</b>已发送至 {values.email}。</p>
                <p>请其查收 → 点击链接完成激活并设置密码；亦可从系统消息查看；激活前不可登录。</p>
              </div>
            ),
          })
        } else {
          Modal.error({ title: '无法创建', content: '请填写邮箱（走邀请）或设置密码（自动生成/手动设置）' })
        }
        if (result.data?.role_errors?.length > 0) {
          message.warning('已创建，但部分角色权限分配失败，可在成员详情中重新分配')
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
        // #2400 D1：有一次性密码弹窗时只重置表单，保留弹窗与明文；弹窗关闭由 onClose 清理。
        if (passwordModalOpened) {
          resetCreateForm()
        } else {
          resetCreateState() // #2072
        }
        setViewMode('list')
        fetchStaffList()
      } else if (result.code === 40900) {
        const users = result.data || []
        const names = users.map(u => `${u.name}(${u.phone})`).join(', ')
        message.error(`创建员工失败：姓名、手机号或邮箱与以下员工冲突: ${names}`)
      } else {
        // #2290：4091x（IAM 字段冲突）等未显式处理的 code 不得静默
        message.error(resolveErrorMessage(result, '创建员工失败，请稍后重试'))
      }
    } catch (error) {
      message.error('创建员工失败: ' + error.message)
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

  // #2114 §8.4: 补齐原先未定义的处理函数（原删除/激活/重设密码按钮会抛 ReferenceError）
  const handleDeleteUser = async (record) => {
    try {
      const res = await staffApi.batchDelete([record.id])
      if (res.code === 20000) {
        message.success('员工已删除')
        fetchStaffList()
      } else {
        message.error(res.message || '删除失败')
      }
    } catch (err) {
      message.error('删除失败: ' + (err.message || ''))
    }
  }

  const handleActivateUser = async (record) => {
    try {
      const res = await staffApi.activateUser(record.id)
      if (res.code === 20000) {
        message.success('员工已激活')
        fetchStaffList()
      } else {
        message.error(res.message || '激活失败')
      }
    } catch (err) {
      message.error('激活失败: ' + (err.message || ''))
    }
  }

  const handleResetPassword = async (userIds) => {
    if (!userIds || userIds.length === 0) {
      message.warning('请先选择要重设密码的员工')
      return
    }
    // #2389 Rev.3：混合批量先过滤无邮箱行；全部无邮箱则只提示、不发请求。
    const noEmail = staffList.filter(u => userIds.includes(u.id) && !(u.email || '').trim())
    const withEmail = userIds.filter(id => !noEmail.some(u => u.id === id))
    if (withEmail.length === 0) {
      Modal.warning({
        title: '该账号未配置邮箱',
        content: '无法通过邮件重置密码。请使用「直接设置密码」功能（对接中）；在此之前可由管理员在成员详情中重置。',
        okText: '知道了',
      })
      return
    }
    try {
      const res = await staffApi.resetPassword(withEmail)
      if (res.code === 20000) {
        const skipped = res.data?.skipped_no_email?.length || noEmail.length
        message.success(skipped > 0 ? `已发送 ${withEmail.length} 封；${skipped} 个账号因未配置邮箱未发送` : `已发送重设密码邮件（${withEmail.length} 个用户）`)
        setSelectedRowKeys([])
      } else {
        message.error(res.message || '重设密码失败')
      }
    } catch (err) {
      message.error('重设密码失败: ' + (err.message || ''))
    }
  }

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的员工')
      return
    }
    Modal.confirm({
      title: '批量删除确认',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个员工吗？此操作不可恢复。`,
      okText: '确定删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        setBatchLoading(true)
        try {
          const result = await staffApi.batchDelete(selectedRowKeys)
          if (result.code === 20000) {
            message.success(`已成功删除 ${result.data.deleted} 个员工`)
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
      message.warning('请先选择要重设密码的员工')
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
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 130,
      render: (role) => {
        const roleMap = { site_admin: '管理员', site_member: '成员', merchant_staff: '商户直属员工', repair_technician: '维修师傅', STAFF: '成员', WORKER: '员工' }
        return roleMap[role] || role || '-'
      }
    },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
      width: 120,
      render: (position, record) => {
        if (position) return position
        const roleMap = { 'site_admin': '管理员', 'site_member': '成员', 'merchant_staff': '商户直属员工', 'STAFF': '成员', 'repair_technician': '维修师傅', 'WORKER': '员工' }
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
            onClick={() => {
              // #2389 Rev.3：无邮箱账号不跳转，弹 Modal.warning（按钮保持可点）。
              if (!String(record.email || '').trim()) {
                Modal.warning({
                  title: '该账号未配置邮箱',
                  content: '无法通过邮件重置密码。请使用「直接设置密码」功能（对接中）；在此之前可由管理员在成员详情中重置。',
                  okText: '知道了',
                })
                return
              }
              navigate(`/staff/${record.id}/reset-password`, { state: { user: record } })
            }}
          >
            重设密码
          </Button>
          <Popconfirm
            title={`确定删除员工「${record.name}」？`}
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
            {/* #2114: 邀请管理 + 直属成员管理入口（商户管理员 / 系统管理员） */}
            {(isMerchantAdmin || businessRole === 'system_admin') && (
              <>
                <Badge count={invitePending} size="small">
                  <Button icon={<TeamOutlined />} onClick={() => navigate('/staff/invites')}>
                    邀请管理
                  </Button>
                </Badge>
                <Button icon={<TeamOutlined />} onClick={() => navigate('/staff/direct')}>
                  直属成员管理
                </Button>
              </>
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
              onClick={() => { resetCreateState(); setViewMode('create') }} // #2072: 进入即重置
            >
              创建员工
            </Button>
          </Space>
        }
      >
        <Form
          name="staff-search" /* #2072: 命名空间化字段 id，避免与创建表单同名 id 致浏览器自动填充 */
          layout="inline"
          className="mb-4"
          onFinish={handleSearch}
        >
          <Form.Item name="name">
            <Input placeholder="搜索姓名" prefix={<SearchOutlined />} autoComplete="off" />
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
        title="创建员工" 
        extra={
          <Button onClick={() => { resetCreateState(); setViewMode('list') }}>
            返回列表
          </Button>
        }
        className="mb-4" 
        size="small"
      >
        {/* #2390：网点员工创建统一走「网点管理 → 成员」；本页仅创建商户直属员工 */}
        <Alert
          type="info"
          showIcon
          className="mb-3"
          message="网点员工请在「网点管理 → 成员」中创建"
          action={<Button size="small" type="link" onClick={() => navigate('/organization/sites')}>前往网点管理</Button>}
        />
        <Form
          key={createFormKey}
          name="staff-create" /* #2072: 命名空间化字段 id */
          form={createUserForm}
          layout="vertical"
          onFinish={handleCreateUser}
        >
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
              <Input placeholder="姓名" />
            </Form.Item>
            <Form.Item
              name="username"
              label="登录名"
              rules={[{ required: true, message: '请填写登录名' }]}
              extra="仅用于登录，平台内唯一。员工姓名请填「姓名」。"
            >
              <Input placeholder="登录名" autoComplete="off" />
            </Form.Item>
            <Form.Item name="email" label="邮箱（选填）">
              <Input placeholder="留空则用下方密码模式直接创建" />
            </Form.Item>
            <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
              <Input placeholder="手机号" />
            </Form.Item>
          </div>

          {/* #2372：密码模式（自动生成默认 / 手动）；「无邮箱邀请」由后端按模式推导 */}
          <PasswordSettingFields
            autoGenerate={createAutoGenerate}
            forcePasswordChange={createForcePasswordChange}
            onAutoGenerateChange={setCreateAutoGenerate}
            onForcePasswordChangeChange={setCreateForcePasswordChange}
          />

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
              照片将在创建成功后上传；也可稍后在员工编辑中更换。
            </div>
          </Form.Item>

          {/* #2372：密码模式默认自动生成；「有邮箱且未设密码」时后端保留邀请（邮件）语义 */}
          <Space>
            <Button type="primary" htmlType="submit">创建员工</Button>
            <Button onClick={() => { resetCreateState(); setViewMode('list') }}>取消</Button>
          </Space>
        </Form>

      </Card>
      )}

      <InitialPasswordModal
        open={passwordModalOpen}
        title="已创建 · 初始密码仅显示一次"
        passwords={initialPasswords}
        onClose={() => { setPasswordModalOpen(false); setInitialPasswords([]) }}
      />

      {/* #2052：原「员工已存在」冲突选择对话框已移除 —— 改为发送邀请通知 */}

      <Modal
        title="编辑员工"
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
