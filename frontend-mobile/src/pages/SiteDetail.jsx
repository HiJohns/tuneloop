import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { apiFetch } from '../services/api';
import { Card, Button as AntButton, Image, Tag, Divider } from 'antd';
import { EnvironmentOutlined, PhoneOutlined, ClockCircleOutlined } from '@ant-design/icons';
import { View, Text, Button } from '@tarojs/components';
import { phone } from '../platform';

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api';

export default function SiteDetail() {
  const { id } = useParams();
  const [site, setSite] = useState(null);
  const [loading, setLoading] = useState(true);
  const [stockStatus, setStockStatus] = useState({});

  const fetchSiteDetail = useCallback(async () => {
    try {
      const response = await apiFetch(`${API_BASE}/common/sites/${id}`);
      const result = await response.json();
      
      if (result.code === 20000) {
        setSite(result.data.site);
        setStockStatus(result.data.stock_status || {});
      }
    } catch (error) {
      console.error('Failed to fetch site:', error);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchSiteDetail();
  }, [fetchSiteDetail]);

  const handleNavigate = () => {
    if (site && site.latitude && site.longitude) {
      wx.openLocation({
        latitude: parseFloat(site.latitude),
        longitude: parseFloat(site.longitude),
        name: site.name,
        address: site.address,
        scale: 18
      });
    }
  };

  const handleCall = () => {
    if (site && site.phone) {
      phone.call(site.phone);
    }
  };

  if (loading) return <View className="p-4">加载中...</View>;
  if (!site) return <View className="p-4">网点不存在</View>;

  return (
    <View className="min-h-screen bg-gray-50 pb-20">
      <View className="relative h-48 bg-white">
        {site.images && site.images.length > 0 ? (
          <Image.PreviewGroup>
            <View className="flex overflow-x-auto">
              {site.images.map((img, idx) => (
                <Image key={idx} src={img} className="w-full h-48 object-cover" />
              ))}
            </View>
          </Image.PreviewGroup>
        ) : (
          <View className="w-full h-48 bg-gray-200 flex items-center justify-center">
            <EnvironmentOutlined style={{ fontSize: '48px', color: '#ccc' }} />
          </View>
        )}
      </View>

      <View className="p-4">
        <Card className="mb-4">
          <Text className="text-lg font-bold">{site.name}</Text>
          <View className="mt-2 space-y-1">
            <View className="flex items-center text-gray-600">
              <EnvironmentOutlined className="mr-2" />
              <Text>{site.address}</Text>
            </View>
            {site.phone && (
              <View className="flex items-center text-gray-600">
                <PhoneOutlined className="mr-2" />
                <Text>{site.phone}</Text>
              </View>
            )}
            {site.business_hours && (
              <View className="flex items-center text-gray-600">
                <ClockCircleOutlined className="mr-2" />
                <Text>营业时间: {site.business_hours}</Text>
              </View>
            )}
          </View>
        </Card>

        <Card className="mb-4">
          <Text className="text-base font-bold">实时库存</Text>
          <View className="mt-2">
            {Object.entries(stockStatus).map(([category, status]) => (
              <View key={category} className="mb-3">
                <Text strong className="capitalize">{category}</Text>
                <View className="flex gap-2 mt-1">
                  <Tag color="green">可租: {status.available || 0}</Tag>
                  <Tag color="blue">在租: {status.renting || 0}</Tag>
                  <Tag color="orange">维保: {status.maintenance || 0}</Tag>
                </View>
              </View>
            ))}
          </View>
        </Card>

        <View className="fixed bottom-0 left-0 right-0 bg-white p-4 border-t flex gap-2">
          <AntButton icon={<PhoneOutlined />} onClick={handleCall} className="flex-1">
            联系门店
          </AntButton>
          <AntButton type="primary" icon={<EnvironmentOutlined />} onClick={handleNavigate} className="flex-1">
            导航前往
          </AntButton>
        </View>
      </View>
    </View>
  );
}
