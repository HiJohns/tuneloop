import React, { useEffect } from 'react';
import { Spin, message } from 'antd';

const AuthCallback: React.FC = () => {
  // OAuth URL 构建函数
  const getOAuthUrl = () => {
    const IAM_URL = window.APP_CONFIG?.iamExternalUrl || 'http://opencode.linxdeep.com:5552';
    const CLIENT_ID = window.APP_CONFIG?.iamClientId || 'tuneloop';
    const redirectUri = encodeURIComponent(window.location.origin + '/callback');
    return `${IAM_URL}/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=${redirectUri}&response_type=code`;
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const state = params.get('state');

    if (!code) {
      message.error('认证失败：未收到授权码');
      setTimeout(() => {
        window.location.href = getOAuthUrl();
      }, 2000);
      return;
    }

    // Decode state to get original URL
    let originalUrl = sessionStorage.getItem('original_request_url');
    if (state) {
      try {
        const stateData = JSON.parse(atob(decodeURIComponent(state)));
        originalUrl = stateData.originalUrl || originalUrl;
      } catch (e) {
        console.error('Failed to decode state:', e);
      }
    }

    // Call the backend callback endpoint to exchange code for token
    fetch(`/api/auth/callback?code=${encodeURIComponent(code)}${state ? `&state=${encodeURIComponent(state)}` : ''}`)
      .then(response => response.json())
      .then(data => {
        if (data.code === 20000 && data.data && data.data.access_token) {
          // Store the token
          localStorage.setItem('access_token', data.data.access_token);
          if (data.data.refresh_token) {
            localStorage.setItem('refresh_token', data.data.refresh_token);
          }
          
          // Redirect to original URL or dashboard
          if (originalUrl) {
            sessionStorage.removeItem('original_request_url');
            
            // Add code parameter to original URL
            const redirectUrl = new URL(originalUrl);
            redirectUrl.searchParams.set('code', code);
            if (state) {
              redirectUrl.searchParams.set('state', state);
            }
            
            window.location.href = redirectUrl.toString();
          } else {
            // Fallback to dashboard
            window.location.href = '/dashboard';
          }
        } else {
          message.error('认证失败：无法获取访问令牌');
          setTimeout(() => {
            window.location.href = getOAuthUrl();
          }, 2000);
        }
      })
      .catch(error => {
        console.error('Callback error:', error);
        message.error('认证失败：网络错误');
        setTimeout(() => {
          window.location.href = getOAuthUrl();
        }, 2000);
      });
  }, []);

  return (
    <div style={{ 
      display: 'flex', 
      justifyContent: 'center', 
      alignItems: 'center', 
      height: '100vh',
      background: '#f0f2f5'
    }}>
      <Spin size="large" tip="正在完成登录..." />
    </div>
  );
};

export default AuthCallback;