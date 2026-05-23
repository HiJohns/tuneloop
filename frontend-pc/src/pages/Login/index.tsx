import React, { useState } from 'react';
import { Button, Card, Spin, Alert } from 'antd';
import { useBrand } from '../components/BrandProvider';

const LoginPage: React.FC = () => {
  const { config, loading } = useBrand();
  const [redirecting, setRedirecting] = useState(false);

  const params = new URLSearchParams(window.location.search);
  const reason = params.get('reason');

  const iamUrl = window.APP_CONFIG?.pc?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '';
  const clientId = window.APP_CONFIG?.pc?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc';
  const redirectUri = encodeURIComponent(window.location.origin + '/callback');

  // If session expired, redirect directly to IAM login
  if (reason === 'session_expired') {
    window.location.href = iamUrl + '/login?reason=session_expired&client_id=' + clientId + '&redirect_uri=' + redirectUri;
  }

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
        <Spin size="large" tip={reason === 'session_expired' ? '正在跳转至登录...' : '正在跳转至安全身份验证中心...'} />
      </div>
    );
  }

  if (reason === 'session_expired') {
    return (
      <div className="login-container">
        <Card className="login-card" style={{ textAlign: 'center' }}>
          <h1 style={{ fontSize: 24, marginBottom: 16 }}>会话已结束</h1>
          <p style={{ color: '#888', marginBottom: 24 }}>为了保证您的数据安全，本次登录会话已超时</p>
          <Button type="primary" size="large" onClick={handleLogin}>
            点击这里重新登录
          </Button>
        </Card>
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
