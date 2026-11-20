import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Popconfirm, Modal, Input, Select, Form, Alert } from 'antd';
import { PlusOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';
import { formatBeijingDate } from '../utils/date';
import { adminApi, invitesApi } from '../services/api';
import PasswordSettingFields from './PasswordSettingFields';
import InitialPasswordModal from './InitialPasswordModal';

const ROLE_NAMES = {
  admin: '网点管理员',
  site_admin: '网点管理员',
  staff: '网点员工',
  site_member: '网点员工',
  merchant_staff: '商户直属员工',
  repair_technician: '维修师傅',
};

const SITE_ROLES = ['site_admin', 'site_member']; // #2122：维修师直属商户，网点不再创建/管理维修师

const roleToCode = (role) => {
  if (!role) return 'site_member'
  const map = { Staff: 'site_member', Manager: 'site_admin' }
  return map[role] || role
}

// #1938: 参数化以复用——membersBase 支持中转中心（/admin/transit-sites）；
// roles 注入静态角色（中转仅两类，且 /admin/roles 权限门与中转中心不同）。
const SiteMemberManagement = ({ siteId, onRefresh, membersBase = '/sites', roles = null }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [modalVisible, setModalVisible] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [adding, setAdding] = useState(false);
  const [availableRoles, setAvailableRoles] = useState([]);
  const [selectedRole, setSelectedRole] = useState('site_admin');
  // #2373：密码模式（D1）——不再暴露「跳过邮箱验证」独立开关；密码模式即直接激活。
  const [autoGenerate, setAutoGenerate] = useState(true);
  const [forcePasswordChange, setForcePasswordChange] = useState(true);
  const [passwordDraft, setPasswordDraft] = useState('');
  const [initialPasswords, setInitialPasswords] = useState([]);
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);
  const [pwdForm] = Form.useForm();

  const [formMode, setFormMode] = useState('new');
  const [username, setUsername] = useState('');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [existingUser, setExistingUser] = useState(null);
  const [duplicates, setDuplicates] = useState({});
  const [checking, setChecking] = useState(false);

  useEffect(() => {
    if (siteId) {
      fetchMembers();
      fetchRoles();
    }
  }, [siteId, page, pageSize]);

  const fetchRoles = async () => {
    // #1938: 调用方注入静态角色时优先使用（中转中心仅 site_admin/site_member）
    if (roles && roles.length > 0) {
      setAvailableRoles(roles);
      return;
    }
    try {
      const resp = await adminApi.listRoles({ page_size: 100 });
      if (resp.code === 20000) setAvailableRoles((resp.data?.list || []).filter(r => SITE_ROLES.includes(r.code)));
    } catch { /* non-critical */ }
  };

  const fetchMembers = async () => {
    setLoading(true);
    try {
      const response = await api.get(`${membersBase}/${siteId}/members`, { params: { page, page_size: pageSize } });
      if (response && response.code === 20000) {
        setMembers(response.data?.list || []);
        setTotal(response.data?.total || 0);
      }
    } catch (error) {
      message.error('获取成员列表失败');
    } finally {
      setLoading(false);
    }
  };

  // #2035: 多角色提交（role=主，roles=全集）
  const handleUpdateRole = async (userId, roles) => {
    const list = Array.isArray(roles) ? roles.filter(Boolean) : [roles].filter(Boolean)
    if (list.length === 0) {
      message.warning('至少保留一个角色')
      return
    }
    try {
      const resp = await api.put(`${membersBase}/${siteId}/members/${userId}`, { role: list[0], roles: list });
      if (resp.code === 20000) {
        message.success('角色已更新');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch { message.error('更新角色失败') }
  };

  const handleRemoveMember = async (userId) => {
    try {
      const response = await api.delete(`${membersBase}/${siteId}/members/${userId}`);
      if (response.code === 20000) {
        message.success('成员移除成功');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error('移除成员失败');
    }
  };

  const checkField = async (field, value) => {
    if (!value || !value.trim()) return;
    setChecking(true);
    try {
      const resp = await api.get(`/users/check?${field}=${encodeURIComponent(value.trim())}`);
      if (resp.code === 20000 && resp.data?.exists) {
        setDuplicates(prev => ({ ...prev, [field]: true }));
        setExistingUser(resp.data.user);
      } else {
        setDuplicates(prev => ({ ...prev, [field]: false }));
      }
    } catch { /* non-critical */ }
    setChecking(false);
  };

  const hasDuplicate = duplicates.username || duplicates.email || duplicates.phone;

  const switchToExisting = () => {
    setFormMode('existing');
  };

  const switchToNew = () => {
    setFormMode('new');
    setExistingUser(null);
    setDuplicates({});
  };

  const resetForm = () => {
    setFormMode('new');
    setUsername('');
    setName('');
    setEmail('');
    setPhone('');
    setExistingUser(null);
    setDuplicates({});
    setSelectedRole('site_admin');
    setAutoGenerate(true);
    setForcePasswordChange(true);
    setPasswordDraft('');
    setInitialPasswords([]);
    setPasswordModalOpen(false);
    pwdForm.resetFields();
  };

  const isFormValid = () => {
    if (formMode === 'existing') return !!existingUser;
    // #2373：邮箱选填（后端 #2371：无邮箱时需有密码通道）。
    const hasPasswordInput = autoGenerate || !!passwordDraft;
    return !!username.trim() && !!name.trim() && !!phone.trim() && (!!email.trim() || hasPasswordInput);
  };

  // #2114 P2/P3：网点管理员无权限直接添加非本商户成员 → 引导提交加入申请（商户管理员审批）。
  const promptApply = (payload) => {
    Modal.confirm({
      title: '需要商户管理员同意',
      content: '您没有权限邀请该用户，需商户管理员同意。是否提出加入申请？',
      okText: '提出申请',
      cancelText: '取消',
      onOk: async () => {
        try {
          const resp = await invitesApi.applySiteMembership(siteId, payload);
          if (resp.code === 20100) {
            message.success('申请已提交，等待商户管理员审批');
            setModalVisible(false);
            resetForm();
          } else {
            message.error(resp.message || '提交申请失败');
          }
        } catch (e) {
          message.error('提交申请失败');
        }
      },
    });
  };

  const handleSubmit = async () => {
    setAdding(true);
    try {
      if (formMode === 'existing') {
        // #2428 D3：跨租户账号禁止 direct bind，改为提交加入申请。
        if (existingUser.same_tenant === false) {
          promptApply({ identifier: phone.trim() || email.trim() || username.trim(), role: selectedRole });
          setAdding(false);
          return;
        }
        const response = await api.post(`${membersBase}/${siteId}/members`, {
          user_ids: [{ user_id: existingUser.id, role: selectedRole }],
        });
        if (response.code === 20000 || response.code === 20100) {
          message.success('成员已绑定');
          setModalVisible(false);
          resetForm();
          fetchMembers();
          onRefresh && onRefresh();
          if (response.data?.role_errors?.length > 0) {
            message.warning('绑定成功，但部分角色权限分配失败。');
          }
        } else if (response.code === 40311 || response.code === 40911) {
          promptApply({ identifier: phone.trim() || email.trim() || username.trim(), role: selectedRole });
        } else {
          message.error(response.message || '绑定失败');
        }
      } else {
        // #2373：手动密码先做前端强度校验（与后端一致），通过后再提交 #2371 契约字段。
        if (!autoGenerate) {
          try {
            await pwdForm.validateFields();
          } catch {
            setAdding(false);
            return;
          }
        }
        const password = autoGenerate ? '' : passwordDraft;
        if (!email.trim() && !autoGenerate && !password) {
          message.error('需填写邮箱或设置密码');
          setAdding(false);
          return;
        }
        const newUser = {
          username: username.trim(),
          name: name.trim(),
          phone: phone.trim(),
          role: selectedRole,
          auto_generate: autoGenerate,
          force_password_change: forcePasswordChange,
        };
        if (email.trim()) newUser.email = email.trim();
        if (!autoGenerate) newUser.password = password;
        const response = await api.post(`${membersBase}/${siteId}/members`, {
          new_users: [newUser],
        });
        if (response.code === 20000 || response.code === 20100) {
          const data = response.data;
          const directCount = data.directly_added?.length || 0;
          if (data.bind_errors?.length > 0) {
            message.error(`添加成员失败：${data.bind_errors.map(e => e.error || '未知错误').join('；')}`);
            setModalVisible(false);
            resetForm();
            fetchMembers();
            onRefresh && onRefresh();
            return;
          }
          message.success(`成功添加 ${directCount} 个用户`);
          // #2373：先重置表单，再打开一次性密码 Modal（避免 resetForm 清空刚拿到的密码）。
          const passwords = (data.initial_passwords || []).map(p => ({
            identifier: p.identifier || p.email || p.username || '',
            password: p.password,
          }));
          setModalVisible(false);
          resetForm();
          fetchMembers();
          onRefresh && onRefresh();
          if (passwords.length > 0) {
            setInitialPasswords(passwords);
            setPasswordModalOpen(true);
          }
          if (data.role_errors?.length > 0) {
            message.warning('用户创建成功，但部分角色权限分配失败。');
          }
        } else if (response.code === 40311 || response.code === 40911) {
          // #2114 P2/P3：无权限直接添加非本商户成员 → 提交加入申请
          promptApply({ identifier: phone.trim() || email.trim(), role: selectedRole });
        } else if (response.code === 40902) {
          // #2031：既有账户不允许管理员挂接（一人一号）→ 引导本人自助加入
          Modal.info({
            title: '该手机号/邮箱已注册',
            content: response.message || '该手机号/邮箱已在本平台注册，请让本人登录后自助加入（作为员工加入 / 注册为顾客）。',
          });
        } else {
          message.error(response.message || '添加成员失败');
        }
      }
    } catch { message.error('操作失败') }
    setAdding(false);
  };

  const filteredMembers = searchKeyword
    ? members.filter(m =>
        (m.user_name || '').toLowerCase().includes(searchKeyword.toLowerCase()) ||
        (m.user_email || '').toLowerCase().includes(searchKeyword.toLowerCase())
      )
    : members;

  const columns = [
    {
      title: '姓名',
      dataIndex: 'user_name',
      key: 'user_name',
    },
    {
      title: '邮箱',
      dataIndex: 'user_email',
      key: 'user_email',
    },
    {
      title: '角色',
      key: 'role',
      render: (_, record) => {
        // #2035: 多选编辑；roles 全集优先，回退主 role
        const all = (Array.isArray(record.roles) && record.roles.length > 0)
          ? record.roles.map(roleToCode)
          : [roleToCode(record.role)].filter(Boolean)
        return (
          <Select
            mode="multiple"
            value={all}
            onChange={(vals) => handleUpdateRole(record.user_id, vals)}
            size="small"
            style={{ minWidth: 200 }}
            placeholder="选择角色"
          >
            {availableRoles.map(r => (
              <Select.Option key={r.code} value={r.code}>
                {r.name}
              </Select.Option>
            ))}
          </Select>
        )
      },
    },
    {
      title: '加入时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => formatBeijingDate(date),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Popconfirm
            title="移除该成员的本网点员工身份？"
            description="仅解除其本网点的员工身份，账户保留，可重新加入。"
            onConfirm={() => handleRemoveMember(record.user_id)}
          >
            <Button type="link" danger icon={<DeleteOutlined />}>移除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索姓名或邮箱"
          allowClear
          style={{ width: 260 }}
          value={searchKeyword}
          onChange={(e) => setSearchKeyword(e.target.value)}
        />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => {
          resetForm();
          setModalVisible(true);
        }}>
          添加成员
        </Button>
      </div>

      <Modal
        title="添加成员"
        open={modalVisible}
        onCancel={() => { setModalVisible(false); resetForm(); }}
        footer={null}
        width={520}
      >
        {formMode === 'existing' && existingUser ? (
          <>
            <Alert
              type="info"
              message="绑定现有用户"
              description="以下用户已在系统中注册，将直接绑定到本网点。"
              style={{ marginBottom: 16 }}
            />
            <div style={{ marginBottom: 24, padding: 12, background: '#fafafa', borderRadius: 6 }}>
              <p><strong>用户名：</strong>{existingUser.username || '-'}</p>
              <p><strong>姓名：</strong>{existingUser.name || '-'}</p>
              <p><strong>邮箱：</strong>{existingUser.email || '-'}</p>
              <p><strong>手机：</strong>{existingUser.phone_masked || '-'}</p>
            </div>
            <Button type="link" onClick={switchToNew} style={{ padding: 0, marginBottom: 16 }}>
              改为新建用户
            </Button>
          </>
        ) : (
          <>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 4, color: '#666' }}>用户名 <span style={{ color: '#ff4d4f' }}>*</span></label>
              <Input
                value={username}
                onChange={e => { setUsername(e.target.value); setDuplicates(prev => ({ ...prev, username: false })); }}
                onBlur={e => checkField('username', e.target.value)}
                status={duplicates.username ? 'error' : undefined}
              />
              {duplicates.username && <span style={{ color: '#ff4d4f', fontSize: 12 }}>该用户名已注册</span>}
            </div>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 4, color: '#666' }}>姓名 <span style={{ color: '#ff4d4f' }}>*</span></label>
              <Input value={name} onChange={e => setName(e.target.value)} />
            </div>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 4, color: '#666' }}>邮箱（选填）</label>
              <Input
                value={email}
                onChange={e => { setEmail(e.target.value); setDuplicates(prev => ({ ...prev, email: false })); }}
                onBlur={e => checkField('email', e.target.value)}
                status={duplicates.email ? 'error' : undefined}
              />
              {duplicates.email && <span style={{ color: '#ff4d4f', fontSize: 12 }}>该邮箱已注册</span>}
            </div>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 4, color: '#666' }}>手机 <span style={{ color: '#ff4d4f' }}>*</span></label>
              <Input
                value={phone}
                onChange={e => { setPhone(e.target.value); setDuplicates(prev => ({ ...prev, phone: false })); }}
                onBlur={e => checkField('phone', e.target.value)}
                status={duplicates.phone ? 'error' : undefined}
              />
              {duplicates.phone && <span style={{ color: '#ff4d4f', fontSize: 12 }}>该手机号已注册</span>}
            </div>
            {hasDuplicate && existingUser && (
              <div style={{ marginBottom: 16, padding: 8, background: '#fff2f0', borderRadius: 4 }}>
                <span style={{ color: '#ff4d4f' }}>该用户已注册</span>
                <Button type="link" onClick={switchToExisting} style={{ padding: '0 0 0 8px', height: 'auto' }}>
                  改为绑定现有用户
                </Button>
              </div>
            )}
          </>
        )}

        <div style={formMode === 'existing' ? { borderTop: '1px solid #f0f0f0', paddingTop: 16 } : {}}>
          <label style={{ display: 'block', marginBottom: 8, color: '#666' }}>角色</label>
          <Select value={selectedRole} onChange={setSelectedRole} style={{ width: '100%' }}>
            {availableRoles.map(r => (
              <Select.Option key={r.code} value={r.code}>{r.name}</Select.Option>
            ))}
          </Select>
          {formMode !== 'existing' && (
            <Form form={pwdForm} component={false} onValuesChange={(_, vals) => setPasswordDraft(vals.password || '')}>
              <PasswordSettingFields
                autoGenerate={autoGenerate}
                forcePasswordChange={forcePasswordChange}
                onAutoGenerateChange={setAutoGenerate}
                onForcePasswordChangeChange={setForcePasswordChange}
              />
            </Form>
          )}
        </div>

        <div style={{ textAlign: 'right', marginTop: 16 }}>
          <Button onClick={() => { setModalVisible(false); resetForm(); }} style={{ marginRight: 8 }}>
            取消
          </Button>
          <Button
            type="primary"
            onClick={handleSubmit}
            loading={adding}
            disabled={!isFormValid()}
          >
            确认添加
          </Button>
        </div>
      </Modal>

      <InitialPasswordModal
        open={passwordModalOpen}
        passwords={initialPasswords}
        onClose={() => { setPasswordModalOpen(false); setInitialPasswords([]); }}
        title="已创建 · 初始密码仅显示一次"
      />

      <Table
        columns={columns}
        dataSource={filteredMembers}
        loading={loading}
        rowKey="user_id"
        pagination={{
          current: page, pageSize, total, showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
        locale={{ emptyText: '暂无成员' }}
      />
    </div>
  );
};

export default SiteMemberManagement;
