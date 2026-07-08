import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm, Modal, Input, Select, Checkbox, Typography, Alert } from 'antd';
import { PlusOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';
import { adminApi } from '../services/api';

const ROLE_COLORS = {
  owner: 'red', merchant_admin: 'red',
  admin: 'blue', site_admin: 'blue',
  staff: 'green', site_member: 'green',
  repair_technician: 'purple',
};
const ROLE_NAMES = {
  admin: '网点管理员',
  site_admin: '网点管理员',
  staff: '网点员工',
  site_member: '网点员工',
  repair_technician: '维修师傅',
};

const SITE_ROLES = ['site_admin', 'site_member', 'repair_technician'];

const roleToCode = (role) => {
  if (!role) return 'site_member'
  const map = { Staff: 'site_member', Manager: 'site_admin' }
  return map[role] || role
}

const SiteMemberManagement = ({ siteId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [adding, setAdding] = useState(false);
  const [availableRoles, setAvailableRoles] = useState([]);
  const [selectedRole, setSelectedRole] = useState('site_admin');
  const [skipActivation, setSkipActivation] = useState(false);

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
  }, [siteId]);

  const fetchRoles = async () => {
    try {
      const resp = await adminApi.listRoles();
      if (resp.code === 20000) setAvailableRoles((resp.data || []).filter(r => SITE_ROLES.includes(r.code)));
    } catch { /* non-critical */ }
  };

  const fetchMembers = async () => {
    setLoading(true);
    try {
      const response = await api.get(`/sites/${siteId}/members`);
      if (response && response.code === 20000) {
        setMembers(response.data?.list || []);
      }
    } catch (error) {
      message.error('获取成员列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId, newRole) => {
    try {
      const resp = await api.put(`/sites/${siteId}/members/${userId}`, { role: newRole });
      if (resp.code === 20000) {
        message.success('角色已更新');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch { message.error('更新角色失败') }
  };

  const handleRemoveMember = async (userId) => {
    try {
      const response = await api.delete(`/sites/${siteId}/members/${userId}`);
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
    setSkipActivation(false);
  };

  const isFormValid = () => {
    if (formMode === 'existing') return !!existingUser;
    return username.trim() && name.trim() && email.trim() && phone.trim();
  };

  const handleSubmit = async () => {
    setAdding(true);
    try {
      if (formMode === 'existing') {
        const response = await api.post(`/sites/${siteId}/members`, {
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
        } else {
          message.error(response.message || '绑定失败');
        }
      } else {
        const response = await api.post(`/sites/${siteId}/members`, {
          new_users: [{ username: username.trim(), name: name.trim(), email: email.trim(), phone: phone.trim(), role: selectedRole }],
          skip_activation: skipActivation,
        });
        if (response.code === 20000 || response.code === 20100) {
          const data = response.data;
          const directCount = data.directly_added?.length || 0;
          message.success(`成功添加 ${directCount} 个用户`);
          setModalVisible(false);
          resetForm();
          fetchMembers();
          onRefresh && onRefresh();

          if (skipActivation && data.initial_passwords?.length > 0) {
            const lines = data.initial_passwords.map((p, i) => (
              <div key={i} style={{ marginBottom: 8 }}>
                <span style={{ color: '#666' }}>{p.email}：</span>
                <Typography.Text copyable code style={{ fontSize: 14, padding: '2px 8px' }}>
                  {p.password}
                </Typography.Text>
              </div>
            ));
            Modal.success({
              title: '成员已创建',
              content: (
                <div>
                  <p>初始密码（请立即复制保存）：</p>
                  <div style={{ margin: '12px 0' }}>{lines}</div>
                  <p style={{ color: '#ff4d4f' }}>此为仅显示一次密码，请妥善保存。</p>
                </div>
              ),
            });
          }
          if (data.role_errors?.length > 0) {
            message.warning('用户创建成功，但部分角色权限分配失败。');
          }
        } else if (response.code === 40901) {
          const data = response.data;
          if (data?.conflicts) {
            message.warning(`用户已存在：${data.conflicts.map(c => c.email || c.name).join(', ')}`);
          }
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
      render: (_, record) => (
        <Select
          value={roleToCode(record.role)}
          onChange={(val) => handleUpdateRole(record.user_id, val)}
          size="small"
          style={{ width: 140 }}
        >
          {availableRoles.map(r => (
            <Select.Option key={r.code} value={r.code}>
              {r.name}
            </Select.Option>
          ))}
        </Select>
      ),
    },
    {
      title: '加入时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => new Date(date).toLocaleDateString(),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Popconfirm
            title="确认移除此成员？"
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
              <label style={{ display: 'block', marginBottom: 4, color: '#666' }}>邮箱 <span style={{ color: '#ff4d4f' }}>*</span></label>
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
            <div style={{ marginTop: 12 }}>
              <Checkbox checked={skipActivation} onChange={e => setSkipActivation(e.target.checked)}>
                跳过邮箱验证（直接激活）
              </Checkbox>
            </div>
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

      <Table
        columns={columns}
        dataSource={filteredMembers}
        loading={loading}
        rowKey="user_id"
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: '暂无成员' }}
      />
    </div>
  );
};

export default SiteMemberManagement;
