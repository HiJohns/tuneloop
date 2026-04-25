import { useState, useEffect, useCallback } from 'react';
import { Modal, AutoComplete, Button, message, Form, Input, Alert } from 'antd';
import { UserAddOutlined, UserOutlined } from '@ant-design/icons';
import api from '../services/api';
import { debounce } from 'lodash';

const UserSelectionDialog = ({ visible, onClose, onConfirm, merchantId, title = '选择用户' }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOptions, setSearchOptions] = useState([]);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [createForm] = Form.useForm();
  const [formErrors, setFormErrors] = useState({});

  // Debounced search function
  const debouncedSearch = useCallback(
    debounce(async (query) => {
      if (!query || query.length < 2) {
        setSearchOptions([]);
        return;
      }

      try {
        const response = await api.get('/api/iam/users/search', {
          params: { q: query, limit: 10, merchant_id: merchantId }
        });

        if (response.data.code === 20000) {
          const users = response.data.data.users || [];
          const options = users.map(user => ({
            label: (
              <div style={{ padding: '8px' }}>
                <div style={{ fontWeight: 'bold' }}>{user.name}</div>
                <div style={{ fontSize: '12px', color: '#666' }}>
                  匹配: {user.matched_field === 'email' ? user.email : 
                        user.matched_field === 'phone' ? user.phone : 
                        user.name} {user.associated ? ' ✓ 已关联' : ' ⚠ 未关联'}
                </div>
              </div>
            ),
            value: user.id,
            user: user
          }));
          setSearchOptions(options);
        }
      } catch (error) {
        console.error('Search failed:', error);
        message.error('搜索失败');
      }
    }, 300),
    [merchantId]
  );

  // Handle search input change
  const handleSearch = (value) => {
    setSearchTerm(value);
    debouncedSearch(value);
  };

  // Handle user selection from AutoComplete
  const handleSelectUser = (value, option) => {
    const user = option.user;
    
    // Check if already selected
    if (selectedUsers.find(u => u.id === user.id)) {
      message.warning('该用户已添加到列表');
      return;
    }

    // Add to selected users
    setSelectedUsers([...selectedUsers, user]);
    setSearchTerm('');
    setSearchOptions([]);
    message.success(`已添加 ${user.name}`);
  };

  // Remove user from selected list
  const handleRemoveUser = (userId) => {
    setSelectedUsers(selectedUsers.filter(u => u.id !== userId));
    message.success('已移除用户');
  };

  // Open create user modal
  const openCreateModal = () => {
    setCreateModalVisible(true);
    createForm.resetFields();
    setFormErrors({});
  };

  // Close create user modal
  const closeCreateModal = () => {
    setCreateModalVisible(false);
    createForm.resetFields();
    setFormErrors({});
  };

  // Check uniqueness for form fields
  const checkFieldUniqueness = async (field, value) => {
    if (!value) return;

    try {
      const response = await api.get('/api/users/check', {
        params: field === 'email' ? { email: value } : 
                field === 'phone' ? { phone: value } : {}
      });

      if (response.data.code === 20000 && response.data.data.exists) {
        const user = response.data.data.user;
        setFormErrors(prev => ({
          ...prev,
          [field]: {
            conflict: true,
            user: user
          }
        }));
        return { user, conflict: true };
      } else {
        setFormErrors(prev => {
          const errors = { ...prev };
          delete errors[field];
          return errors;
        });
        return { conflict: false };
      }
    } catch (error) {
      console.error('Uniqueness check failed:', error);
    }
  };

  // Handle create user form submission
  const handleCreateUser = async (values) => {
    try {
      // Check for conflicts before submission
      const emailCheck = formErrors.email?.conflict ? formErrors.email : await checkFieldUniqueness('email', values.email);
      const phoneCheck = values.phone ? (formErrors.phone?.conflict ? formErrors.phone : await checkFieldUniqueness('phone', values.phone)) : { conflict: false };

      if (emailCheck?.conflict || phoneCheck?.conflict) {
        message.error('请处理重复用户冲突后再提交');
        return;
      }

      // Submit to create user API
      const response = await api.post('/api/iam/users', {
        name: values.name,
        email: values.email,
        phone: values.phone,
        password: values.password
      });

      if (response.data.code === 20000) {
        const newUser = {
          ...values,
          id: response.data.data.id,
          name: values.name,
          email: values.email,
          phone: values.phone,
          associated: false, // New users are always unassociated initially
          status: 'pending'
        };

        setSelectedUsers([...selectedUsers, newUser]);
        closeCreateModal();
        message.success('用户创建成功并已添加到列表');
      }
    } catch (error) {
      console.error('Create user failed:', error);
      message.error('创建用户失败');
    }
  };

  // Handle conflict resolution (use existing user)
  const handleUseExistingUser = (field) => {
    const user = formErrors[field].user;
    if (user) {
      // Add to selected users
      setSelectedUsers([...selectedUsers, {
        ...user,
        associated: true // Existing user is associated by default
      }]);
      createForm.resetFields();
      closeCreateModal();
      message.success('已使用现有用户');
    }
  };

  // Handle confirm selection
  const handleConfirm = () => {
    if (selectedUsers.length === 0) {
      message.warning('请至少选择一个用户');
      return;
    }
    onConfirm(selectedUsers);
    setSelectedUsers([]);
    setSearchTerm('');
  };

  // Check if there are unassociated users
  const hasUnassociatedUsers = selectedUsers.some(u => !u.associated);

  return (
    <>
      <Modal
        title={title}
        visible={visible}
        onCancel={onClose}
        onOk={handleConfirm}
        width={600}
        okText="确认选择"
        cancelText="取消"
      >
        {/* AutoComplete search */}
        <div style={{ marginBottom: 16 }}>
          <AutoComplete
            style={{ width: '100%' }}
            placeholder="输入用户名、邮箱或手机号搜索"
            value={searchTerm}
            onChange={handleSearch}
            onSelect={handleSelectUser}
            options={searchOptions}
            allowClear
            size="large"
          >
            {/* Custom input with search icon would go here */}
          </AutoComplete>
        </div>

        {/* Create user button */}
        <div style={{ marginBottom: 16 }}>
          <Button
            type="dashed"
            icon={<UserAddOutlined />}
            onClick={openCreateModal}
            block
          >
            创建新用户
          </Button>
        </div>

        {/* Unassociated users warning */}
        {hasUnassociatedUsers && (
          <Alert
            message="注意：以下用户尚未与当前商户关联，将在确认后收到确认邮件或短信"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

        {/* Selected users list */}
        {selectedUsers.length > 0 && (
          <div>
            <h4>已选择的用户 ({selectedUsers.length})</h4>
            <div>
              {selectedUsers.map(user => (
                <div
                  key={user.id}
                  style={{
                    padding: '8px 12px',
                    border: '1px solid #e8e8e8',
                    borderRadius: '4px',
                    marginBottom: '8px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    backgroundColor: !user.associated ? '#fff7e6' : '#f6ffed'
                  }}
                >
                  <div style={{ flex: 1 }}>
                    <div style={{ fontWeight: 'bold' }}>{user.name}</div>
                    <div style={{ fontSize: '12px', color: '#666' }}>
                      {user.email} {user.phone && ` • ${user.phone}`}
                    </div>
                    {!user.associated && (
                      <div style={{ fontSize: '12px', color: '#fa8c16', marginTop: '4px' }}>
                        ⚠ 未关联
                      </div>
                    )}
                  </div>
                  <Button
                    type="text"
                    danger
                    size="small"
                    onClick={() => handleRemoveUser(user.id)}
                  >
                    删除
                  </Button>
                </div>
              ))}
            </div>
          </div>
        )}
      </Modal>

      {/* Create user modal */}
      <Modal
        title="创建新用户"
        visible={createModalVisible}
        onCancel={closeCreateModal}
        footer={null}
        width={500}
      >
        <Form
          form={createForm}
          layout="vertical"
          onFinish={handleCreateUser}
        >
          <Form.Item
            name="name"
            label="姓名"
            rules={[{ required: true, message: '请输入姓名' }]}
          >
            <Input placeholder="请输入姓名" />
          </Form.Item>

          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效的邮箱地址' }
            ]}
          >
            <Input 
              placeholder="请输入邮箱地址"
              onBlur={(e) => checkFieldUniqueness('email', e.target.value)}
              validationStatus={formErrors.email?.conflict ? 'error' : ''}
            />
            {formErrors.email?.conflict && (
              <Alert
                message={
                  <div>
                    邮箱已被用户 {formErrors.email.user.name} 占用
                    <Button
                      size="small"
                      style={{ marginLeft: 8 }}
                      onClick={() => handleUseExistingUser('email')}
                    >
                      使用该用户
                    </Button>
                  </div>
                }
                type="error"
                showIcon
              />
            )}
          </Form.Item>

          <Form.Item
            name="phone"
            label="手机号"
          >
            <Input 
              placeholder="请输入手机号"
              onBlur={(e) => checkFieldUniqueness('phone', e.target.value)}
              validationStatus={formErrors.phone?.conflict ? 'error' : ''}
            />
            {formErrors.phone?.conflict && (
              <Alert
                message={
                  <div>
                    手机号已被用户 {formErrors.phone.user.name} 占用
                    <Button
                      size="small"
                      style={{ marginLeft: 8 }}
                      onClick={() => handleUseExistingUser('phone')}
                    >
                      使用该用户
                    </Button>
                  </div>
                }
                type="error"
                showIcon
              />
            )}
          </Form.Item>

          <Form.Item
            name="password"
            label="初始密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="请输入初始密码" />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" block>
              创建并添加
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default UserSelectionDialog;