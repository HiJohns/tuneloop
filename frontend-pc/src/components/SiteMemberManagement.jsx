import { useState, useEffect } from 'react';
import { Table, Button, Space, message, Tag, Popconfirm } from 'antd';
import { PlusOutlined, SwapOutlined, DeleteOutlined } from '@ant-design/icons';
import api from '../services/api';
import UserSelectionDialog from './UserSelectionDialog';

const SiteMemberManagement = ({ siteId, onRefresh }) => {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [userDialogVisible, setUserDialogVisible] = useState(false);

  useEffect(() => {
    if (siteId) {
      fetchMembers();
    }
  }, [siteId]);

  const fetchMembers = async () => {
    setLoading(true);
    try {
      const response = await api.get(`/api/sites/${siteId}/members`);
      if (response.data.code === 20000) {
        setMembers(response.data.data.list || []);
      }
    } catch (error) {
      message.error('Failed to fetch members');
      console.error('Fetch error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleAddMember = (userData) => {
    setUserDialogVisible(true);
  };

  const handleUserSelect = async (selectedUser) => {
    if (!selectedUser) return;

    try {
      const response = await api.post(`/api/sites/${siteId}/members`, {
        user_id: selectedUser.user_id,
        role: 'Staff'
      });

      if (response.data.code === 20100) {
        message.success('Member added successfully');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || 'Failed to add member');
    }
  };

  const handleSwitchRole = async (userId, currentRole) => {
    const newRole = currentRole === 'Manager' ? 'Staff' : 'Manager';

    try {
      const response = await api.put(`/api/sites/${siteId}/members/${userId}`, {
        role: newRole
      });

      if (response.data.code === 20000) {
        message.success('Role updated successfully');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || 'Failed to update role');
    }
  };

  const handleRemoveMember = async (userId) => {
    try {
      const response = await api.delete(`/api/sites/${siteId}/members/${userId}`);

      if (response.data.code === 20000) {
        message.success('Member removed successfully');
        fetchMembers();
        onRefresh && onRefresh();
      }
    } catch (error) {
      message.error(error.response?.data?.message || 'Failed to remove member');
    }
  };

  const isLastManager = (userId, role) => {
    if (role !== 'Manager') return false;
    const managerCount = members.filter(m => m.role === 'Manager').length;
    return managerCount === 1;
  };

  const columns = [
    {
      title: 'Name',
      dataIndex: 'user_name',
      key: 'user_name',
    },
    {
      title: 'Email',
      dataIndex: 'user_email',
      key: 'user_email',
    },
    {
      title: 'Role',
      dataIndex: 'role',
      key: 'role',
      render: (role) => (
        <Tag color={role === 'Manager' ? 'blue' : 'default'}>
          {role}
        </Tag>
      ),
    },
    {
      title: 'Joined At',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => new Date(date).toLocaleDateString(),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button
            type="link"
            icon={<SwapOutlined />}
            onClick={() => handleSwitchRole(record.user_id, record.role)}
            disabled={isLastManager(record.user_id, record.role)}
          >
            Switch Role
          </Button>
          <Popconfirm
            title="Remove this member?"
            onConfirm={() => handleRemoveMember(record.user_id)}
            disabled={isLastManager(record.user_id, record.role)}
          >
            <Button type="link" danger icon={<DeleteOutlined />} disabled={isLastManager(record.user_id, record.role)}>
              Remove
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleAddMember}>
          Add Member
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={members}
        loading={loading}
        rowKey="user_id"
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: 'No members found' }}
      />

      <UserSelectionDialog
        visible={userDialogVisible}
        onClose={() => setUserDialogVisible(false)}
        onSelect={handleUserSelect}
        merchantId="current-merchant-id"
      />
    </div>
  );
};

export default SiteMemberManagement;
