import React, { useEffect } from 'react';
import { Spin, message } from 'antd';

const AuthCallback: React.FC = () => {
  // OAuth URL 构建函数
  const getOAuthUrl = () => {
    const IAM_URL = window.APP_CONFIG?.iamExternalUrl || import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || '';
    const CLIENT_ID = window.APP_CONFIG?.iamClientId || import.meta.env.VITE_IAM_PC_CLIENT_ID || 'tuneloop-pc';
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
    fetch(`/api/auth/callback?code=${encodeURIComponent(code)}${state ? `&state=${encodeURIComponent(state)}` : ''}`, {
      credentials: 'include'
    })
      .then(response => response.json())
      .then(data => {
        console.log('[AUTH_CALLBACK] Response:', data)
        if (data.code === 20000 && data.data && data.data.access_token) {
          // Store the token with the correct key that getToken() expects
          const token = data.data.access_token;
          const expiresIn = data.data.expires_in || 3600;
          localStorage.setItem('token', token);
          localStorage.setItem('token_expiry', (Date.now() + expiresIn * 1000).toString());
          sessionStorage.setItem('debug_auth', JSON.stringify({ ok: true, code: data.code, expiresIn, tokenLen: token.length }))
          console.log('[AUTH_CALLBACK] Token stored, expires_in:', expiresIn, 'token_len:', token.length);
          if (data.data.refresh_token) {
            localStorage.setItem('refresh_token', data.data.refresh_token);
          }
          
          // Redirect to original URL or dashboard
          if (originalUrl) {
            sessionStorage.removeItem('original_request_url');
            window.location.href = originalUrl;
          } else {
            window.location.href = '/dashboard';
          }
        } else {
          sessionStorage.setItem('debug_auth', JSON.stringify({ ok: false, code: data.code, error: data.message || 'unknown' }))
          message.error('认证失败：无法获取访问令牌');
          setTimeout(() => {
            window.location.href = getOAuthUrl();
          }, 2000);
        }
      })
      .catch(error => {
        sessionStorage.setItem('debug_auth', JSON.stringify({ ok: false, error: 'network: ' + (error?.message || 'unknown') }))
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