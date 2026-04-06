import React, { createContext, useContext, useEffect, useState } from 'react';
import { ConfigProvider, Spin } from 'antd';
import type { ThemeConfig } from 'antd/es/config-provider/context';
import { api } from '../../services/api';

interface BrandConfig {
  primary_color: string;
  logo_url: string;
  brand_name: string;
  support_phone: string;
}

interface BrandContextType {
  config: BrandConfig | null;
  loading: boolean;
}

const BrandContext = createContext<BrandContextType>({
  config: null,
  loading: true,
});

export const useBrand = () => useContext(BrandContext);

interface BrandProviderProps {
  children: React.ReactNode;
  clientId?: string;
}

export const BrandProvider: React.FC<BrandProviderProps> = ({ 
  children, 
  clientId = 'default' 
}) => {
  const [config, setConfig] = useState<BrandConfig | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadBrandConfig = async () => {
      try {
        const data = await api.get(`/common/brand-config?client_id=${clientId}`)
        if (data) {
          setConfig(data)
          document.documentElement.style.setProperty(
            '--brand-primary', 
            data.primary_color
          )
        }
      } catch (error) {
        console.error('Failed to load brand config:', error)
      } finally {
        setLoading(false)
      }
    };

    loadBrandConfig();
  }, [clientId]);

  const theme: ThemeConfig = {
    token: {
      colorPrimary: config?.primary_color || '#6366F1',
      borderRadius: 6,
    },
  };

  if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh' 
      }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <BrandContext.Provider value={{ config, loading }}>
      <ConfigProvider theme={theme}>
        {children}
      </ConfigProvider>
    </BrandContext.Provider>
  );
};

export default BrandProvider;