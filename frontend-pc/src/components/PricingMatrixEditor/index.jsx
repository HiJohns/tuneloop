import { useState, useEffect } from 'react';
import { Table, InputNumber, Button, message, Card, Typography } from 'antd';
import { SaveOutlined, ReloadOutlined } from '@ant-design/icons';
import { api } from '../../services/api';

const { Title } = Typography;

const categories = [
  { key: 'piano', name: '钢琴' },
  { key: 'violin', name: '小提琴' },
  { key: 'guzheng', name: '古筝' },
  { key: 'guitar', name: '吉他' }
];

const levels = [
  { key: 'entry', name: '入门级' },
  { key: 'professional', name: '专业级' },
  { key: 'master', name: '大师级' }
];

export default function PricingMatrixEditor() {
  const [pricingData, setPricingData] = useState({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadPricingMatrix();
  }, []);

  const loadPricingMatrix = async () => {
    setLoading(true);
    try {
      const data = await api.get('/admin/pricing-matrix');
      setPricingData(data?.matrix || {});
    } catch (error) {
      message.error('加载定价矩阵失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  const handleCellChange = (category, level, field, value) => {
    setPricingData(prev => {
      const newData = { ...prev };
      if (!newData[category]) {
        newData[category] = {};
      }
      if (!newData[category][level]) {
        newData[category][level] = { monthly_rent: 0, deposit: 0 };
      }
      newData[category][level][field] = value;
      return newData;
    });
  };

  const savePricingMatrix = async () => {
    setSaving(true);
    try {
      await api.put('/admin/pricing-matrix', pricingData);
      message.success('定价矩阵保存成功');
    } catch (error) {
      message.error('保存失败: ' + error.message);
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    {
      title: '品类',
      dataIndex: 'categoryName',
      key: 'categoryName',
      width: 120,
      fixed: 'left',
    },
    {
      title: '级别',
      dataIndex: 'levelName',
      key: 'levelName',
      width: 100,
      fixed: 'left',
    },
    {
      title: '月租金 (¥)',
      dataIndex: 'monthly_rent',
      key: 'monthly_rent',
      render: (text, record) => (
        <InputNumber
          min={0}
          value={text}
          onChange={(value) => handleCellChange(record.category, record.level, 'monthly_rent', value)}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: '押金 (¥)',
      dataIndex: 'deposit',
      key: 'deposit',
      render: (text, record) => (
        <InputNumber
          min={0}
          value={text}
          onChange={(value) => handleCellChange(record.category, record.level, 'deposit', value)}
          style={{ width: '100%' }}
        />
      ),
    },
  ];

  const dataSource = [];
  categories.forEach(cat => {
    levels.forEach(level => {
      const cellData = pricingData[cat.key]?.[level.key] || { monthly_rent: 0, deposit: 0 };
      dataSource.push({
        key: `${cat.key}-${level.key}`,
        category: cat.key,
        categoryName: cat.name,
        level: level.key,
        levelName: level.name,
        ...cellData,
      });
    });
  });

  return (
    <Card>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <Title level={4}>定价矩阵编辑</Title>
        <div>
          <Button icon={<ReloadOutlined />} onClick={loadPricingMatrix} style={{ marginRight: 8 }}>
            刷新
          </Button>
          <Button type="primary" icon={<SaveOutlined />} onClick={savePricingMatrix} loading={saving}>
            保存全部
          </Button>
        </div>
      </div>
      
      <Table
        columns={columns}
        dataSource={dataSource}
        loading={loading}
        pagination={false}
        scroll={{ x: 800 }}
        bordered
      />
    </Card>
  );
}
