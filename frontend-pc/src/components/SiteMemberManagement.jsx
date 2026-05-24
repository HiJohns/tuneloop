import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm } from 'antd';
import { SwapOutlined, DeleteOutlined } from '@ant-design/icons';
import api from '../services/api';
import InlineUserSelector from './InlineUserSelector';

const SiteMemberManagement = ({ siteId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);

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

  const handleSelectedUsersChange = (users) => {
    setSelectedUsers(users);
  };

  const handleConfirmAddMembers = async () => {
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
          newUsers.push({
            name: user.name,
            email: user.email,
            phone: user.phone,
            role: 'Staff'
          })
        } else {
          existingUsers.push({
            user_id: user.id || user.user_id,
            role: 'Staff'
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
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || '添加成员失败');
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

  const isLastManager = (userId, role) => {
    if (role !== 'Manager') return false;
    const managerCount = members.filter(m => m.role === 'Manager').length;
    return managerCount === 1;
  };

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
            disabled={isLastManager(record.user_id, record.role)}
          >
            切换角色
          </Button>
          <Popconfirm
            title="确认移除此成员？"
            onConfirm={() => handleRemoveMember(record.user_id)}
            disabled={isLastManager(record.user_id, record.role)}
          >
            <Button type="link" danger icon={<DeleteOutlined />} disabled={isLastManager(record.user_id, record.role)}>
              移除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <InlineUserSelector
          mode="multi"
          merchantId="current-merchant-id"
          value={selectedUsers}
          onChange={handleSelectedUsersChange}
        />
        {selectedUsers.length > 0 && (
          <Button
            type="primary"
            onClick={handleConfirmAddMembers}
            style={{ marginTop: 8 }}
          >
            确认添加 ({selectedUsers.length})
          </Button>
        )}
      </div>

      <Table
        columns={columns}
        dataSource={members}
        loading={loading}
        rowKey="user_id"
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: '暂无成员' }}
      />
    </div>
  );
};

export default SiteMemberManagement;
