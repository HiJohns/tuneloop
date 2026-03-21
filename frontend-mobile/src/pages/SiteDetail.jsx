import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { Card, Button, Image, Tag, Divider, Typography } from 'antd';
import { EnvironmentOutlined, PhoneOutlined, ClockCircleOutlined } from '@ant-design/icons';

const { Title, Text } = Typography;
const API_BASE = process.env.REACT_APP_API_BASE || 'http://localhost:5553';

export default function SiteDetail() {
  const { id } = useParams();
  const [site, setSite] = useState(null);
  const [loading, setLoading] = useState(true);
  const [stockStatus, setStockStatus] = useState({});

  useEffect(() => {
    fetchSiteDetail();
  }, [id]);

  const fetchSiteDetail = async () => {
    try {
      const response = await fetch(`${API_BASE}/api/common/sites/${id}`);
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
  };

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
      window.location.href = `tel:${site.phone}`;
    }
  };

  if (loading) return <div className="p-4">加载中...</div>;
  if (!site) return <div className="p-4">网点不存在</div>;

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      <div className="relative h-48 bg-white">
        {site.images && site.images.length > 0 ? (
          <Image.PreviewGroup>
            <div className="flex overflow-x-auto">
              {site.images.map((img, idx) => (
                <Image key={idx} src={img} className="w-full h-48 object-cover" />
              ))}
            </div>
          </Image.PreviewGroup>
        ) : (
          <div className="w-full h-48 bg-gray-200 flex items-center justify-center">
            <EnvironmentOutlined style={{ fontSize: '48px', color: '#ccc' }} />
          </div>
        )}
      </div>

      <div className="p-4">
        <Card className="mb-4">
          <Title level={4}>{site.name}</Title>
          <div className="mt-2 space-y-1">
            <div className="flex items-center text-gray-600">
              <EnvironmentOutlined className="mr-2" />
              <Text>{site.address}</Text>
            </div>
            {site.phone && (
              <div className="flex items-center text-gray-600">
                <PhoneOutlined className="mr-2" />
                <Text>{site.phone}</Text>
              </div>
            )}
            {site.business_hours && (
              <div className="flex items-center text-gray-600">
                <ClockCircleOutlined className="mr-2" />
                <Text>营业时间: {site.business_hours}</Text>
              </div>
            )}
          </div>
        </Card>

        <Card className="mb-4">
          <Title level={5}>实时库存</Title>
          <div className="mt-2">
            {Object.entries(stockStatus).map(([category, status]) => (
              <div key={category} className="mb-3">
                <Text strong className="capitalize">{category}</Text>
                <div className="flex gap-2 mt-1">
                  <Tag color="green">可租: {status.available || 0}</Tag>
                  <Tag color="blue">在租: {status.renting || 0}</Tag>
                  <Tag color="orange">维保: {status.maintenance || 0}</Tag>
                </div>
              </div>
            ))}
          </div>
        </Card>

        <div className="fixed bottom-0 left-0 right-0 bg-white p-4 border-t flex gap-2">
          <Button icon={<PhoneOutlined />} onClick={handleCall} className="flex-1">
            联系门店
          </Button>
          <Button type="primary" icon={<EnvironmentOutlined />} onClick={handleNavigate} className="flex-1">
            导航前往
          </Button>
        </div>
      </div>
    </div>
  );
}
