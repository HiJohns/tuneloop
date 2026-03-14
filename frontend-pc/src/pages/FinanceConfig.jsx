import { useState } from 'react'
import { Card, InputNumber, Button, message, Input } from 'antd'
import { financeConfig } from '../data/mockData'

export default function FinanceConfig() {
  const [config, setConfig] = useState(financeConfig)

  const handleRatioChange = (level, value) => {
    setConfig({
      ...config,
      rentDepositRatio: {
        ...config.rentDepositRatio,
        [level]: value
      }
    })
  }

  const handleDiscountChange = (months, value) => {
    setConfig({
      ...config,
      renewalDiscount: {
        ...config.renewalDiscount,
        [months]: value
      }
    })
  }

  const handleSave = () => {
    message.success('财务配置已保存')
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">财务配置</h2>
      
      <Card title="租押比设置" className="mb-4">
        <div className="space-y-4">
          {Object.entries(config.rentDepositRatio).map(([level, ratio]) => (
            <div key={level} className="flex items-center justify-between">
              <span>{level}</span>
              <InputNumber
                value={ratio}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) => handleRatioChange(level, value)}
                style={{ width: 120 }}
              />
            </div>
          ))}
        </div>
      </Card>

      <Card title="续租折扣设置">
        <div className="space-y-4">
          {Object.entries(config.renewalDiscount).map(([months, discount]) => (
            <div key={months} className="flex items-center justify-between">
              <span>{months}</span>
              <InputNumber
                value={discount}
                min={0}
                max={1}
                step={0.05}
                onChange={(value) => handleDiscountChange(months, value)}
                formatter={value => `${(value * 100).toFixed(0)}%`}
                parser={value => value.replace('%', '') / 100}
                style={{ width: 120 }}
              />
            </div>
          ))}
        </div>
        <Button type="primary" className="mt-4" onClick={handleSave}>
          保存配置
        </Button>
      </Card>
    </div>
  )
}
