import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm, Modal, Input } from 'antd';
import { PlusOutlined, SwapOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import api from '../services/api';
import InlineUserSelector from './InlineUserSelector';

const SiteMemberManagement = ({ siteId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    if (siteId) {
      fetchMembers();
    }
  }, [siteId]);

  const fetchMembers = async () => {
    setLoading(true);
    try {
      const response = await api.get(`/sites/${siteId}/members`);
      if (response && response.code === 20000) {
        setMembers(response.data?.list || []);
      }
    } catch (error) {
      message.error('获取成员列表失败');
      console.error('Fetch error:', error);
    } finally {
      setLoading(false);
    }
  };
  const handleConfirmAddMembers = async () => {
    setAdding(true);
    if (!selectedUsers || selectedUsers.length === 0) {
      message.warning('请至少选择一个用户');
      return;
    }

    try {
      // Separate existing users and new users
      const existingUsers = []
      const newUsers = []
      
      selectedUsers.forEach(user => {
        if (user.isNew) {
          const defaultRole = members.length === 0 ? 'Manager' : 'Staff'
          newUsers.push({
            name: user.name,
            email: user.email,
            phone: user.phone,
            role: defaultRole
          })
        } else {
          existingUsers.push({
            user_id: user.id || user.user_id,
            role: defaultRole
          })
        }
      })

      const response = await api.post(`/sites/${siteId}/members`, {
        user_ids: existingUsers,
        new_users: newUsers
      });

      if (response.data.code === 20100) {
        const data = response.data.data;
        const directCount = data.directly_added?.length || 0;
        const pendingCount = data.confirmation_sessions?.length || 0;
        
        let messageText = `成功添加 ${directCount} 个用户`;
        if (pendingCount > 0) {
          messageText += `，${pendingCount} 个用户需等待邮件/短信确认`;
        }
        
        message.success(messageText);
        setSelectedUsers([]);
        setModalVisible(false);
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || '添加成员失败');
    } finally {
      setAdding(false);
    }
  };

  const handleSwitchRole = async (userId, currentRole) => {
    const newRole = currentRole === 'Manager' ? 'Staff' : 'Manager';

    try {
      const response = await api.put(`/sites/${siteId}/members/${userId}`, {
        role: newRole
      });

      if (response.data.code === 20000) {
        message.success('角色更新成功');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || '更新角色失败');
    }
  };

  const handleRemoveMember = async (userId) => {
    try {
      const response = await api.delete(`/sites/${siteId}/members/${userId}`);

      if (response.data.code === 20000) {
        message.success('成员移除成功');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || '移除成员失败');
    }
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
      dataIndex: 'role',
      key: 'role',
      render: (role) => (
        <Tag color={role === 'Manager' ? 'blue' : 'default'}>
          {role === 'Manager' ? '管理员' : '员工'}
        </Tag>
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
          <Button
            type="link"
            icon={<SwapOutlined />}
            onClick={() => handleSwitchRole(record.user_id, record.role)}
          >
            切换角色
          </Button>
          <Popconfirm
            title="确认移除此成员？"
            onConfirm={() => handleRemoveMember(record.user_id)}
          >
            <Button type="link" danger icon={<DeleteOutlined />}>
              移除
            </Button>
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
