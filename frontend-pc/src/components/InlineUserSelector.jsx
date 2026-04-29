import { useState, useCallback, useRef, useEffect } from 'react';
import { AutoComplete, Button, Input, Alert, message, Tabs } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import api from '../services/api';

const InlineUserSelector = ({
  merchantId,
  mode = 'multi',
  value,
  onChange,
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOptions, setSearchOptions] = useState([]);
  const [formErrors, setFormErrors] = useState({});
  const [createName, setCreateName] = useState('');
  const [createEmail, setCreateEmail] = useState('');
  const [createPhone, setCreatePhone] = useState('');
  const [activeTab, setActiveTab] = useState('search');

  const selectedUsers = value || [];
  const debounceRef = useRef(null);

  const resetCreateForm = () => {
    setCreateName('');
    setCreateEmail('');
    setCreatePhone('');
    setFormErrors({});
  };

  const debouncedSearch = useCallback((query) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      if (!query || query.length < 2) {
        setSearchOptions([]);
        return;
      }

      try {
        const response = await api.get('/api/iam/users/search', {
          params: { q: query, limit: 10, merchant_id: merchantId },
        });

        if (response.data.code === 20000) {
          const users = response.data.data.users || [];
          const options = users.map((user) => ({
            label: (
              <div style={{ padding: '4px 0' }}>
                <div style={{ fontWeight: 'bold' }}>{user.name}</div>
                <div style={{ fontSize: '12px', color: '#666' }}>
                  匹配:{' '}
                  {user.matched_field === 'email'
                    ? user.email
                    : user.matched_field === 'phone'
                    ? user.phone
                    : user.name}{' '}
                  {user.associated ? ' ✓ 已关联' : ' ⚠ 未关联'}
                </div>
              </div>
            ),
            value: user.id,
            user,
          }));
          setSearchOptions(options);
        }
      } catch (error) {
        console.error('Search failed:', error);
        message.error('搜索失败');
      }
    }, 300);
  }, [merchantId]);

  const handleSearch = (val) => {
    setSearchTerm(val);
    debouncedSearch(val);
  };

  const handleSelectUser = (_value, option) => {
    const user = option.user;

    if (selectedUsers.find((u) => u.id === user.id)) {
      message.warning('该用户已添加到列表');
      setSearchTerm('');
      setSearchOptions([]);
      return;
    }

    let next;
    if (mode === 'single') {
      next = [{ ...user, isNew: false }];
    } else {
      next = [...selectedUsers, { ...user, isNew: false }];
    }

    onChange(next);
    setSearchTerm('');
    setSearchOptions([]);
    message.success(`已选择 ${user.name}`);
  };

  const handleRemoveUser = (userId) => {
    onChange(selectedUsers.filter((u) => u.id !== userId));
  };

	const checkFieldUniqueness = async (field, val) => {
		if (!val) return;
		// Bug 3 fix: Skip API call for name field (backend only supports email/phone)
		if (field === 'name') {
			return { conflict: false };
		}
		try {
			const response = await api.get('/users/check', {
				params: field === 'email' ? { email: val } : field === 'phone' ? { phone: val } : {},
			});

      if (response.data.code === 20000 && response.data.data.exists) {
        const user = response.data.data.user;
        setFormErrors((prev) => ({ ...prev, [field]: { conflict: true, user } }));
        return { user, conflict: true };
      }
      setFormErrors((prev) => {
        const errors = { ...prev };
        delete errors[field];
        return errors;
      });
      return { conflict: false };
    } catch (error) {
      console.error('Uniqueness check failed:', error);
    }
  };

  const handleUseExistingUser = (field) => {
    const user = formErrors[field].user;
    if (user) {
      let next;
      if (mode === 'single') {
        next = [{ ...user, associated: true, isNew: false }];
      } else {
        if (selectedUsers.find((u) => u.id === user.id)) {
          message.warning('该用户已在列表中');
          resetCreateForm();
          return;
        }
        next = [...selectedUsers, { ...user, associated: true, isNew: false }];
      }
      onChange(next);
      resetCreateForm();
      message.success('已使用现有用户');
    }
  };

  const handleCreateUser = async () => {
    if (!createName.trim()) {
      message.error('请输入姓名');
      return;
    }
    if (!createEmail.trim()) {
      message.error('请输入邮箱');
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(createEmail)) {
      message.error('请输入有效的邮箱地址');
      return;
    }

    const emailCheck = formErrors.email?.conflict
      ? formErrors.email
      : await checkFieldUniqueness('email', createEmail);
    const phoneCheck = createPhone
      ? formErrors.phone?.conflict
        ? formErrors.phone
        : await checkFieldUniqueness('phone', createPhone)
      : { conflict: false };

    if (emailCheck?.conflict || phoneCheck?.conflict) {
      message.error('请处理重复用户冲突后再提交');
      return;
    }

    const newUser = {
      id: `new_${Date.now()}`,
      isNew: true,
      name: createName,
      email: createEmail,
      phone: createPhone,
      associated: false,
      status: 'pending',
    };

    let next;
    if (mode === 'single') {
      next = [newUser];
    } else {
      next = [...selectedUsers, newUser];
    }

    onChange(next);
    resetCreateForm();
    message.success('已添加新用户信息');
  };

  const handleTabChange = (key) => {
    setActiveTab(key);
    if (key === 'create') {
      resetCreateForm();
    }
  };

  // Check if form is complete and valid for auto-add
  const isFormCompleteAndValid = () => {
    if (!createName.trim() || !createEmail.trim()) {
      return false;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(createEmail)) {
      return false;
    }
    if (formErrors.email?.conflict || formErrors.phone?.conflict) {
      return false;
    }
	return true;
  };

  const hasUnassociatedUsers = selectedUsers.some((u) => !u.associated);

  const tabItems = [
    {
      key: 'search',
      label: '搜索',
      children: (
        <div>
          <AutoComplete
            style={{ width: '100%' }}
            placeholder="输入用户名、邮箱或手机号搜索"
            value={searchTerm}
            onChange={handleSearch}
            onSelect={handleSelectUser}
            options={searchOptions}
            allowClear
          />
        </div>
      ),
    },
    {
      key: 'create',
      label: '创建',
      children: (
        <div>
          <div style={{ marginBottom: 16 }}>
            <div style={{ marginBottom: 4, color: 'rgba(0,0,0,0.85)' }}>姓名</div>
            <Input
              placeholder="请输入姓名"
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
            />
          </div>

          <div style={{ marginBottom: 16 }}>
            <div style={{ marginBottom: 4, color: 'rgba(0,0,0,0.85)' }}>邮箱</div>
            <Input
              placeholder="请输入邮箱地址"
              value={createEmail}
              onChange={(e) => setCreateEmail(e.target.value)}
              onBlur={() => checkFieldUniqueness('email', createEmail)}
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
                style={{ marginTop: 4 }}
              />
            )}
          </div>

          <div style={{ marginBottom: 16 }}>
            <div style={{ marginBottom: 4, color: 'rgba(0,0,0,0.85)' }}>手机号</div>
            <Input
              placeholder="请输入手机号"
              value={createPhone}
              onChange={(e) => setCreatePhone(e.target.value)}
              onBlur={() => checkFieldUniqueness('phone', createPhone)}
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
                style={{ marginTop: 4 }}
              />
		    )}
          </div>

          <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end' }}>
            <Button
              type="primary"
              onClick={handleCreateUser}
              disabled={!isFormCompleteAndValid()}
            >
              添加
            </Button>
          </div>
        </div>
      ),
    },
  ];

  return (
    <div>
      <Tabs
        items={tabItems}
        onChange={handleTabChange}
        size="small"
      />

      {hasUnassociatedUsers && (
        <Alert
          message="注意：以下用户尚未与当前商户关联，将在确认后收到确认邮件或短信"
          type="warning"
          showIcon
          style={{ marginTop: 8 }}
        />
      )}

      {selectedUsers.length > 0 && (
        <div style={{ marginTop: 8 }}>
          {selectedUsers.map((user) => (
            <div
              key={user.id}
              style={{
                padding: '6px 12px',
                border: '1px solid #e8e8e8',
                borderRadius: 4,
                marginBottom: 4,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                backgroundColor: !user.associated ? '#fff7e6' : '#f6ffed',
              }}
            >
              <div style={{ flex: 1 }}>
                <span style={{ fontWeight: 500 }}>{user.name}</span>
                <span style={{ fontSize: '12px', color: '#666', marginLeft: 8 }}>
                  {user.email}
                  {user.phone && ` • ${user.phone}`}
                </span>
                {!user.associated && (
                  <span style={{ fontSize: '12px', color: '#fa8c16', marginLeft: 8 }}>
                    ⚠ 未关联
                  </span>
                )}
              </div>
              <Button
                type="text"
                danger
                size="small"
                icon={<DeleteOutlined />}
                onClick={() => handleRemoveUser(user.id)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default InlineUserSelector;