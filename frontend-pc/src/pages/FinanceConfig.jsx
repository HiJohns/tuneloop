import { useState, useEffect } from 'react'
import { Card, InputNumber, Button, message, Table, Tag, Tooltip, Input, Spin, Empty } from 'antd'
import { api } from '../services/api'

const levelColors = {
  "入门级": "default",
  "专业级": "blue",
  "大师级": "gold"
}

const defaultEmptyConfig = {
  levels: {}
}

export default function FinanceConfig() {
  const [config, setConfig] = useState(defaultEmptyConfig)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    loadFinanceConfig()
  }, [])

  const loadFinanceConfig = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await api.get('/admin/finance-config')
      if (response && response.levels) {
        setConfig({ levels: response.levels })
      } else {
        throw new Error('Invalid config data')
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleChange = (level, field, value) => {
    setConfig({
      ...config,
      levels: {
        ...config.levels,
        [level]: {
          ...config.levels[level],
          [field]: value
        }
      }
    })
  }

  const handleSave = async () => {
    try {
      await api.put('/admin/finance-config', config)
      message.success('配置已更新，即时生效')
    } catch (error) {
      message.error('保存失败: ' + error.message)
    }
  }

  const columns = [
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      render: (level) => <Tag color={levelColors[level]}>{level}</Tag>
    },
    {
      title: '月租金 (元)',
      dataIndex: 'rent',
      key: 'rent',
      align: 'right',
      render: (rent, record) => (
        <InputNumber
          value={rent}
          min={0}
          max={99999}
          onChange={(value) => handleChange(record.level, 'rent', value)}
          style={{ width: 120 }}
          formatter={value => `¥ ${value}`.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}
          parser={value => value.replace(/¥\s?|(,*)/g, '')}
        />
      )
    },
    {
      title: '押金 (元)',
      dataIndex: 'deposit',
      key: 'deposit',
      align: 'right',
      render: (deposit, record) => (
        <InputNumber
          value={deposit}
          min={0}
          max={999999}
          onChange={(value) => handleChange(record.level, 'deposit', value)}
          style={{ width: 120 }}
          formatter={value => `¥ ${value}`.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}
          parser={value => value.replace(/¥\s?|(,*)/g, '')}
        />
      )
    },
    {
      title: (
        <span>
          续租折扣
          <Tooltip title="95% 表示续租时租金按原价 9.5 折计算">
            <span className="ml-1 cursor-help text-gray-400">ℹ️</span>
          </Tooltip>
        </span>
      ),
      dataIndex: 'renewalDiscount',
      key: 'renewalDiscount',
      render: (discount, record) => (
        <InputNumber
          value={discount}
          min={0}
          max={1}
          step={0.05}
          onChange={(value) => {
            if (value > 1) {
              message.warning('折扣不能超过100%')
              return
            }
            handleChange(record.level, 'renewalDiscount', value)
          }}
          style={{ width: 100 }}
          formatter={value => `${(value * 100).toFixed(0)}%`}
          parser={value => value.replace('%', '') / 100}
        />
      )
    },
     {
       title: '包含服务包内容',
       dataIndex: 'maintenance',
       key: 'maintenance',
       render: (maintenance, record) => (
         <Input
           value={maintenance}
           size="small"
           onChange={(e) => handleChange(record.level, 'maintenance', e.target.value)}
           placeholder="输入服务包内容"
           style={{ width: 200 }}
         />
       )
     }
  ]

  const dataSource = Object.entries(config.levels).map(([level, values]) => ({
    key: level,
    level,
    ...values
  }))

  if (loading) {
    return (
      <div className="text-center py-16">
        <Spin size="large" />
        <div className="mt-4 text-gray-500">数据正在同步中...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-16">
        <Empty description="配置加载失败" />
        <Button type="primary" onClick={loadFinanceConfig} className="mt-4">
          重试
        </Button>
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">财务配置</h2>
      
      <Card title="乐器级别定价参数" className="mb-4">
        <p className="mb-4 text-gray-600">
          在此页面维护不同级别乐器的租金、押金和续租折扣规则。体现了"管理员管人不管事"——只管规则，不管具体订单。
        </p>
        
        <Table 
          columns={columns} 
          dataSource={dataSource} 
          pagination={{ total: dataSource.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
        />

        <Button type="primary" className="mt-4" onClick={handleSave}>
          保存配置
        </Button>
      </Card>

      <Card title="配置说明">
        <ul className="list-disc pl-5 space-y-2 text-gray-600">
          <li><strong>月租金:</strong> 客户租用乐器每月的费用</li>
          <li><strong>押金:</strong> 客户租用时需要缴纳的保证金，退租时退还</li>
          <li><strong>续租折扣:</strong> 客户续租时享受的折扣比例 (1 = 100% = 无折扣)</li>
        </ul>
      </Card>
    </div>
  )
}
