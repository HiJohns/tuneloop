import React, { useState } from 'react';
import { Button, Card, Spin, Alert } from 'antd';
import { useBrand } from '../components/BrandProvider';

const LoginPage: React.FC = () => {
  const { config, loading } = useBrand();
  const [redirecting, setRedirecting] = useState(false);

  const params = new URLSearchParams(window.location.search);
  const reason = params.get('reason');

  const handleLogin = () => {
    const originalUrl = window.location.href;
    sessionStorage.setItem('original_request_url', originalUrl);

    setRedirecting(true);
    const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '';
    const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc';
    const redirectUri = encodeURIComponent(window.location.origin + '/callback');
    const state = btoa(JSON.stringify({ originalUrl }));
    window.location.href = `${iamUrl}/oauth/authorize?client_id=${clientId}&redirect_uri=${redirectUri}&state=${encodeURIComponent(state)}`;
  };

  if (redirecting) {
    return (
      <div className="login-container">
        <Spin size="large" tip="正在跳转至安全身份验证中心..." />
      </div>
    );
  }

  return (
    <div 
      className="login-container" 
      style={{ '--brand-primary': config?.primary_color || '#6366F1' } as React.CSSProperties}
    >
      <Card className="login-card">
        <img 
          src={config?.logo_url || '/logo.png'} 
          alt="logo" 
          className="brand-logo"
        />
        <h1 className="brand-name">{config?.brand_name || 'TuneLoop'}</h1>
        {reason === 'session_expired' && (
          <Alert
            message="会话已过期，请重新登录"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}
        {reason === 'access_denied' && (
          <Alert
            message="登录失败，请重试"
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}
        <Button 
          type="primary" 
          size="large" 
          block
          onClick={handleLogin}
          style={{ 
            backgroundColor: 'var(--brand-primary)',
            borderColor: 'var(--brand-primary)'
          }}
        >
          立即登录
        </Button>
      </Card>
    </div>
  );
};

export default LoginPage;