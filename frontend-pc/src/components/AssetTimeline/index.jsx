import { useEffect, useState } from 'react';
import { Timeline, Card, Typography, Tag } from 'antd';
import { ClockCircleOutlined, CheckCircleOutlined } from '@ant-design/icons';

const { Title } = Typography;

const eventColors = {
  '入库': 'blue',
  '调拨': 'orange',
  '租赁': 'green',
  '维保': 'purple',
  '归还': 'red',
};

export default function AssetTimeline({ assetId }) {
  const [timelineData, setTimelineData] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchTimeline();
  }, [assetId]);

  const fetchTimeline = async () => {
    try {
      const response = await fetch(`/api/common/assets/${assetId}/timeline`);
      const result = await response.json();
      
      if (result.code === 20000) {
        setTimelineData(result.data.timeline || []);
      }
    } catch (error) {
      console.error('Failed to fetch timeline:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <Title level={4} style={{ marginBottom: 24 }}>
        资产流转轨迹
      </Title>
      
      <Timeline
        loading={loading}
        pending={timelineData.length === 0 ? '暂无数据' : false}
        mode="left"
      >
        {timelineData.map((item, index) => (
          <Timeline.Item
            key={index}
            color={eventColors[item.event] || 'gray'}
            dot={<CheckCircleOutlined />}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <strong>{item.event}</strong>
                <div style={{ color: '#666', fontSize: '14px' }}>
                  {item.description}
                </div>
                <div style={{ color: '#999', fontSize: '12px', marginTop: '4px' }}>
                  位置: {item.location}
                </div>
              </div>
              <Tag icon={<ClockCircleOutlined />}>
                {item.date}
              </Tag>
            </div>
          </Timeline.Item>
        ))}
      </Timeline>
    </Card>
  );
}
