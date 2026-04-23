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
      message.error('Failed to check system status');
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
      
      message.success('System admin created successfully! Redirecting to login...');
      
      // In production: Redirect to OIDC URL for first authentication
      if (response.data.oidc_url) {
        window.location.href = response.data.oidc_url;
      } else {
        navigate('/');
      }
    } catch (error) {
      if (error.response?.status === 403) {
        message.error('System already initialized');
        navigate('/');
      } else {
        message.error(error.response?.data?.message || 'Failed to create system admin');
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
        <Card title="System Initialization" style={{ width: 400, boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)' }}>
          <p style={{ marginBottom: 24, color: '#666' }}>
            Create the first system administrator account
          </p>
          
          <Form form={form} onFinish={handleSubmit} layout="vertical">
            <Form.Item
              name="email"
              label="Email"
              rules={[
                { required: true, message: 'Please enter email' },
                { type: 'email', message: 'Please enter a valid email' },
              ]}
            >
              <Input placeholder="admin@example.com" size="large" />
            </Form.Item>
            
            <Form.Item
              name="password"
              label="Password"
              rules={[
                { required: true, message: 'Please enter password' },
                { min: 8, message: 'Password must be at least 8 characters' },
              ]}
            >
              <Input.Password placeholder="Enter password" size="large" />
            </Form.Item>
            
            <Form.Item
              name="confirmPassword"
              label="Confirm Password"
              dependencies={['password']}
              rules={[
                { required: true, message: 'Please confirm password' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('password') === value) {
                      return Promise.resolve();
                    }
                    return Promise.reject(new Error('Passwords do not match'));
                  },
                }),
              ]}
            >
              <Input.Password placeholder="Confirm password" size="large" />
            </Form.Item>
            
            <Form.Item>
              <Button type="primary" htmlType="submit" size="large" block loading={loading}>
                Create System Admin
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
