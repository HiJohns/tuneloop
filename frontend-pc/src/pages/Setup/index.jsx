import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, message, Card, Spin } from 'antd';
import api from '../../services/api';

const Setup = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [requiresSetup, setRequiresSetup] = useState(false);

  useEffect(() => {
    // Check if system needs setup
    checkSetupStatus();
  }, []);

  const checkSetupStatus = async () => {
    try {
      const response = await api.get('/api/setup/status');
      const { requires_setup } = response.data;
      
      if (!requires_setup) {
        // System already initialized, redirect to login
        navigate('/');
        return;
      }
      
      setRequiresSetup(true);
    } catch (error) {
      message.error('检查系统状态失败');
      console.error('Setup check error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (values) => {
    try {
      setLoading(true);
      
      const response = await api.post('/api/setup/init', {
        email: values.email,
        password: values.password,
      });
      
      message.success('系统管理员创建成功！正在跳转登录...');
      
      // In production: Redirect to OIDC URL for first authentication
      if (response.data.oidc_url) {
        window.location.href = response.data.oidc_url;
      } else {
        navigate('/');
      }
    } catch (error) {
      if (error.response?.status === 403) {
        message.error('系统已初始化');
        navigate('/');
      } else {
        message.error(error.response?.data?.message || '创建系统管理员失败');
      }
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (requiresSetup) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
        <Card title="系统初始化" style={{ width: 400, boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)' }}>
          <p style={{ marginBottom: 24, color: '#666' }}>
            创建第一个系统管理员账户
          </p>
          
          <Form form={form} onFinish={handleSubmit} layout="vertical">
            <Form.Item
              name="email"
              label="邮箱"
              rules={[
                { required: true, message: '请输入邮箱' },
                { type: 'email', message: '请输入有效的邮箱地址' },
              ]}
            >
              <Input placeholder="admin@example.com" size="large" />
            </Form.Item>
            
            <Form.Item
              name="password"
              label="密码"
              rules={[
                { required: true, message: '请输入密码' },
                { min: 8, message: '密码长度至少 8 位' },
              ]}
            >
              <Input.Password placeholder="输入密码" size="large" />
            </Form.Item>
            
            <Form.Item
              name="confirmPassword"
              label="确认密码"
              dependencies={['password']}
              rules={[
                { required: true, message: '请确认密码' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('password') === value) {
                      return Promise.resolve();
                    }
                    return Promise.reject(new Error('两次输入的密码不一致'));
                  },
                }),
              ]}
            >
              <Input.Password placeholder="确认密码" size="large" />
            </Form.Item>
            
            <Form.Item>
              <Button type="primary" htmlType="submit" size="large" block loading={loading}>
                创建系统管理员
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </div>
    );
  }

  return null;
};

export default Setup;
