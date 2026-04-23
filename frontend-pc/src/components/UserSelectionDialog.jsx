import { useState } from 'react';
import { Modal, Input, Button, List, message, Card } from 'antd';
import { SearchOutlined, UserAddOutlined, UserOutlined } from '@ant-design/icons';
import api from '../services/api';

const UserSelectionDialog = ({ visible, onClose, onSelect, merchantId }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchResults, setSearchResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUser, setSelectedUser] = useState(null);

  const handleSearch = async () => {
    if (!searchTerm.trim()) {
      message.warning('Please enter search term');
      return;
    }

    setLoading(true);
    try {
      const response = await api.get('/api/iam/users/lookup', {
        params: { identifier: searchTerm, merchant_id: merchantId },
      });

      if (response.data.code === 20000) {
        // Scenario A: User found and belongs to merchant
        setSearchResults([
          {
            ...response.data.data,
            scenario: 'A',
            display: `✓ Found user: ${response.data.data.name} (${response.data.data.email})`,
          },
        ]);
      } else if (response.data.code === 40400) {
        // Scenario C: User not found
        setSearchResults([
          {
            scenario: 'C',
            display: '✗ User not found. Create new user?',
            searchTerm: searchTerm,
          },
        ]);
      } else {
        // Scenario B: User exists but not in merchant
        setSearchResults([
          {
            scenario: 'B',
            display: `⚠ User exists in platform but not in this merchant. Invite?`,
            userInfo: response.data.data,
          },
        ]);
      }
    } catch (error) {
      message.error('Search failed');
      console.error('Search error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSelect = (user) => {
    setSelectedUser(user);
  };

  const handleConfirm = async () => {
    if (!selectedUser) {
      message.warning('Please select a user');
      return;
    }

    try {
      if (selectedUser.scenario === 'B') {
        // Invite user to merchant
        await api.post(`/api/iam/users/${selectedUser.userInfo.user_id}/invite`, {
          merchant_id: merchantId,
        });
        message.success('Invitation sent successfully');
      } else if (selectedUser.scenario === 'C') {
        // Create new user
        message.info('User creation flow would be triggered here');
      }

      onSelect(selectedUser);
      handleClose();
    } catch (error) {
      message.error('Operation failed');
    }
  };

  const handleClose = () => {
    setSearchTerm('');
    setSearchResults([]);
    setSelectedUser(null);
    onClose();
  };

  return (
    <Modal
      title="Select User"
      visible={visible}
      onOk={handleConfirm}
      onCancel={handleClose}
      width={600}
    >
      <Card size="small" style={{ marginBottom: 16 }}>
        <Input.Search
          placeholder="Enter username, email, or phone"
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          onSearch={handleSearch}
          enterButton={<Button type="primary" icon={<SearchOutlined />} />}
          size="large"
        />
      </Card>

      {loading && (
        <div style={{ textAlign: 'center', padding: '20px' }}>
          <span>Searching...</span>
        </div>
      )}

      {!loading && searchResults.length > 0 && (
        <List
          size="small"
          bordered
          dataSource={searchResults}
          renderItem={(item) => (
            <List.Item
              style={{
                cursor: 'pointer',
                backgroundColor: selectedUser === item ? '#e6f7ff' : 'white',
              }}
              onClick={() => handleSelect(item)}
            >
              <List.Item.Meta
                avatar={
                  item.scenario === 'C' ? (
                    <UserAddOutlined style={{ color: '#faad14' }} />
                  ) : (
                    <UserOutlined style={{ color: item.scenario === 'A' ? '#52c41a' : '#1890ff' }} />
                  )
                }
                title={item.display}
                description={
                  item.scenario === 'C'
                    ? 'Would you like to create this user?'
                    : item.scenario === 'B'
                    ? `Email: ${item.userInfo.email}, Phone: ${item.userInfo.phone}`
                    : `Member of current merchant`
                }
              />
            </List.Item>
          )}
        />
      )}

      {!loading && searchResults.length === 0 && searchTerm && (
        <div style={{ textAlign: 'center', padding: '20px', color: '#999' }}>
          No results found. Try searching by email, phone, or username.
        </div>
      )}
    </Modal>
  );
};

export default UserSelectionDialog;
