import React, { useState } from 'react';
import { Button, Card, Spin } from 'antd';
import { useBrand } from '../components/BrandProvider';

const LoginPage: React.FC = () => {
  const { config, loading } = useBrand();
  const [redirecting, setRedirecting] = useState(false);

  const handleLogin = () => {
    setRedirecting(true);
    const iamUrl = import.meta.env.VITE_IAM_URL;
    const redirectUri = encodeURIComponent(window.location.origin + '/api/auth/callback');
    window.location.href = `${iamUrl}/oauth/authorize?client_id=${import.meta.env.VITE_CLIENT_ID}&redirect_uri=${redirectUri}`;
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