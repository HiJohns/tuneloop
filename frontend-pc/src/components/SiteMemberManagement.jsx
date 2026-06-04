import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm, Modal, Input, Select, Checkbox, Typography } from 'antd';
import { PlusOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';
import { adminApi } from '../services/api';
import InlineUserSelector from './InlineUserSelector';

const ROLE_COLORS = {
  owner: 'red', merchant_admin: 'red',
  admin: 'blue', site_admin: 'blue',
  staff: 'green', site_member: 'green',
  worker: 'orange',
};
const ROLE_NAMES = {
  admin: '网点管理员',
  site_admin: '网点管理员',
  staff: '网点员工',
  site_member: '网点员工',
  worker: '维修工程师',
};

const SITE_ROLES = ['site_admin', 'site_member', 'worker'];

const roleToCode = (role) => {
  if (!role) return 'site_member'
  const map = { Staff: 'site_member', Manager: 'site_admin' }
  return map[role] || role
}

const SiteMemberManagement = ({ siteId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [adding, setAdding] = useState(false);
const [availableRoles, setAvailableRoles] = useState([]);
const [selectedRole, setSelectedRole] = useState('site_admin');
const [skipActivation, setSkipActivation] = useState(false);

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

  const handleConfirmAddMembers = async () => {
    setAdding(true);
    if (!selectedUsers || selectedUsers.length === 0) {
      message.warning('请至少选择一个用户');
      return;
    }
    try {
      const existingUsers = [];
      const newUsers = [];
      selectedUsers.forEach(user => {
        if (user.isNew) {
          newUsers.push({ name: user.name, email: user.email, phone: user.phone, role: selectedRole });
        } else {
          existingUsers.push({ user_id: user.id || user.user_id, role: selectedRole });
        }
      });
      const response = await api.post(`/sites/${siteId}/members`, {
        user_ids: existingUsers,
        new_users: newUsers,
        skip_activation: skipActivation,
      });
      if (response.code === 20000 || response.code === 20100) {
        const data = response.data;
        const directCount = data.directly_added?.length || 0;
        let msg = `成功添加 ${directCount} 个用户`;
        message.success(msg);
        setSelectedUsers([]);
        setModalVisible(false);
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
      } else {
        const data = response.data;
        if (data?.conflicts) {
          message.warning(`以下用户已存在：${data.conflicts.map(c => c.email || c.name).join(', ')}`);
        } else {
          message.error(response.message || '添加成员失败');
        }
      }
    } catch { message.error('添加成员失败') }
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
          setSelectedUsers([]);
          setSelectedRole('site_admin');
          setSkipActivation(false);
          setModalVisible(true);
        }}>
          添加成员
        </Button>
      </div>

      <Modal
        title="添加成员"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={600}
      >
        <InlineUserSelector
          mode="multi"
          merchantId="current-merchant-id"
          value={selectedUsers}
          onChange={setSelectedUsers}
        />
        <div style={{ marginTop: 16 }}>
          <label style={{ display: 'block', marginBottom: 8, color: '#666' }}>初始角色</label>
          <Select value={selectedRole} onChange={setSelectedRole} style={{ width: '100%' }}>
            {availableRoles.map(r => (
              <Select.Option key={r.code} value={r.code}>{r.name}</Select.Option>
            ))}
          </Select>
          <div style={{ marginTop: 12 }}>
            <Checkbox checked={skipActivation} onChange={e => setSkipActivation(e.target.checked)}>
              跳过邮箱验证（直接激活）
            </Checkbox>
          </div>
        </div>
        <div style={{ textAlign: 'right', marginTop: 16 }}>
          <Button onClick={() => setModalVisible(false)} style={{ marginRight: 8 }}>
            取消
          </Button>
          <Button
            type="primary"
            onClick={handleConfirmAddMembers}
            loading={adding}
            disabled={!selectedUsers || selectedUsers.length === 0}
          >
            确认添加 ({selectedUsers.length})
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
