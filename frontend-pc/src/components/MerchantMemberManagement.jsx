import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm, Modal, Input, Select, Form, Alert } from 'antd';
import { PlusOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';
import { formatBeijingDate } from '../utils/date';
import PasswordSettingFields from './PasswordSettingFields';
import InitialPasswordModal from './InitialPasswordModal'
import { resolveCreateFeedback } from '../utils/createFeedback';

const ROLE_COLORS = {
  owner: 'red', merchant_admin: 'red',
  admin: 'blue', site_admin: 'blue',
  staff: 'green', site_member: 'green',
  repair_technician: 'purple',
};
const ROLE_NAMES = {
  merchant_admin: '商户管理员',
  site_admin: '网点管理员',
  site_member: '网点员工',
  merchant_staff: '商户直属员工',
  repair_technician: '维修师傅',
};



const MerchantMemberManagement = ({ merchantId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [modalVisible, setModalVisible] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [adding, setAdding] = useState(false);
  const [selectedRole] = useState('merchant_admin'); // 负责人管理：只能创建商户管理员 (#1718)
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
    if (merchantId) {
      fetchMembers();
    }
  }, [merchantId, page, pageSize]);

  const fetchMembers = async () => {
    setLoading(true);
    try {
      const response = await api.get(`/admin/merchants/${merchantId}/members`, { params: { page, page_size: pageSize } });
      if (response && response.code === 20000) {
        setMembers(response.data?.list || []);
        setTotal(response.data?.total || 0);
      }
    } catch (error) {
      message.error('获取负责人列表失败');
    } finally {
      setLoading(false);
    }
  };


  const handleRemoveMember = async (userId) => {
    try {
      const response = await api.delete(`/admin/merchants/${merchantId}/members/${userId}`);
      if (response.code === 20000) {
        message.success('负责人移除成功');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error('移除负责人失败');
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

  const handleSubmit = async () => {
    setAdding(true);
    try {
      if (formMode === 'existing') {
        const response = await api.post(`/admin/merchants/${merchantId}/members`, {
          user_ids: [{ user_id: existingUser.id, role: selectedRole }],
        });
        if (response.code === 20000 || response.code === 20100) {
          message.success('管理员已添加');
          setModalVisible(false);
          resetForm();
          fetchMembers();
          onRefresh && onRefresh();
          if (response.data?.role_errors?.length > 0) {
            message.warning('绑定成功，但部分角色权限分配失败。');
          }
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
          Modal.error({ title: '无法创建', content: '请填写邮箱（走邀请）或设置密码（自动生成/手动设置）' });
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
        const response = await api.post(`/admin/merchants/${merchantId}/members`, {
          new_users: [newUser],
        });
        if (response.code === 20000 || response.code === 20100) {
          const data = response.data;
          const directCount = data.directly_added?.length || 0;
          if (data.bind_errors?.length > 0) {
            message.error(`添加管理员失败：${data.bind_errors.map(e => e.error || '未知错误').join('；')}`);
            setModalVisible(false);
            resetForm();
            fetchMembers();
            onRefresh && onRefresh();
            return;
          }
          message.success(`成功添加 ${directCount} 个用户`);
          // #2373：先重置表单，再打开一次性密码 Modal（避免 resetForm 清空刚拿到的密码）。
          // #2388：按创建通道分支，禁止用「是否有 initial_passwords」猜模式。
          const feedback = resolveCreateFeedback({
            autoGenerate,
            password: passwordDraft,
            email,
            username,
          });
          const passwords = (data.initial_passwords || []).map(p => ({
            identifier: p.identifier || p.email || p.username || '',
            password: p.password,
          }));
          setModalVisible(false);
          resetForm();
          fetchMembers();
          onRefresh && onRefresh();
          if (feedback.kind === 'password_auto') {
            setInitialPasswords(passwords);
            setPasswordModalOpen(true);
          } else if (feedback.kind === 'password_manual') {
            message.success('已创建，账号可用用户名 + 你设置的密码登录');
          } else if (feedback.kind === 'invite') {
            Modal.info({
              title: '已创建并发送邀请',
              width: 480,
              content: <div><p><b>邀请邮件</b>已发送至 {email}。</p><p>请其查收 → 点击链接完成激活并设置密码；亦可从系统消息查看；激活前不可登录。</p></div>,
            });
          } else {
            Modal.error({ title: '无法创建', content: '请填写邮箱（走邀请）或设置密码（自动生成/手动设置）' });
          }
          if (data.role_errors?.length > 0) {
            message.warning('已创建，但部分角色权限分配失败，可在成员详情中重新分配');
          }
        } else if (response.code === 40902) {
          // #2031：既有账户不允许管理员挂接（一人一号）→ 引导本人自助加入
          Modal.info({
            title: '该手机号/邮箱已注册',
            content: response.message || '该手机号/邮箱已在本平台注册，请让本人登录后自助加入（作为员工加入 / 注册为顾客）。',
          });
        } else if (response.code === 40901) {
          const c = response.data?.conflicts?.[0];
          if (!c) {
            message.error('未知冲突，请刷新后重试');
          } else if (c.same_merchant && c.merchants?.length > 0) {
            // 场景 A：同商户成员 → 确认后直接绑定
            Modal.confirm({
              title: '用户已存在',
              content: `用户 ${c.name || c.username}（${c.phone || '无电话'} / ${c.email || '无邮箱'}）已是「${c.merchants.map(o => o.merchant_name).join('、')}」成员，是否直接加入本商户？`,
              onOk: () => bindExistingUser(c.user_id),
            });
          } else if (c.same_merchant) {
            // 场景 B：已是本商户成员（兜底）
            message.warning(`用户 ${c.name || c.username} 已在成员列表中`);
          } else {
            // 场景 C：与商户无关
            Modal.info({
              title: '用户已存在',
              content: `用户 ${c.name || c.username}（${c.email || '无邮箱'}）已注册但未加入本商户，如需邀请请联系商户管理员。`,
            });
          }
        } else {
          message.error(response.message || '添加成员失败');
        }
      }
    } catch { message.error('操作失败') }
    setAdding(false);
  };

  const bindExistingUser = async (userId) => {
    setAdding(true);
    try {
      const response = await api.post(`/admin/merchants/${merchantId}/members`, {
        user_ids: [{ user_id: userId, role: selectedRole }],
      });
      if (response.code === 20000 || response.code === 20100) {
        message.success('管理员已添加');
        setModalVisible(false);
        resetForm();
        fetchMembers();
        onRefresh && onRefresh();
        if (response.data?.role_errors?.length > 0) {
          message.warning('绑定成功，但部分角色权限分配失败。');
        }
      } else {
        message.error(response.message || '绑定失败');
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
      render: (_, record) => (
        // 负责人管理（#1718）：所有成员均为商户管理员，无角色切换
        <Tag color="red">{ROLE_NAMES[record.role] || '商户管理员'}</Tag>
      ),
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
      render: (_, record) => {
        // 最后负责人保护（#1718）：仅剩 1 个负责人时禁用移除（后端同样拒绝）
        const isLastAdmin = members.length <= 1
        return (
          <Space>
            <Popconfirm
              title="移除该成员的商户身份？"
              description="将解除其在商户及下属网点的全部身份，账户保留，可重新加入。"
              onConfirm={() => handleRemoveMember(record.user_id)}
              disabled={isLastAdmin}
            >
              <Button type="link" danger icon={<DeleteOutlined />} disabled={isLastAdmin}>移除</Button>
            </Popconfirm>
          </Space>
        )
      },
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
          添加管理员
        </Button>
      </div>

      <Modal
        title="添加管理员"
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
              description="以下用户已在系统中注册，将直接绑定到本商户。"
              style={{ marginBottom: 16 }}
            />
            <div style={{ marginBottom: 24, padding: 12, background: '#fafafa', borderRadius: 6 }}>
              <p><strong>用户名：</strong>{existingUser.username || '-'}</p>
              <p><strong>姓名：</strong>{existingUser.name || '-'}</p>
              <p><strong>邮箱：</strong>{existingUser.email || '-'}</p>
              <p><strong>手机：</strong>{existingUser.phone || '-'}</p>
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
          {/* 负责人管理（#1718）：只能创建商户管理员，无需角色选择 */}
          <input type="hidden" value={selectedRole} />
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
        title="已创建 · 初始密码仅显示一次"
        passwords={initialPasswords}
        onClose={() => { setPasswordModalOpen(false); setInitialPasswords([]); }}
        title="管理员初始密码（仅展示一次）"
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

export default MerchantMemberManagement;
