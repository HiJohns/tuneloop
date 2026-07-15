import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Button, Input, ScrollView } from '@tarojs/components'
import { api } from '../services/api'

export default function PointsPrePurchase() {
  const navigate = useNavigate()
  const [amount, setAmount] = useState(300)
  const [customAmount, setCustomAmount] = useState('')
  const [paying, setPaying] = useState(false)

  const selectedAmount = customAmount || amount

  const handlePay = () => {
    if (!selectedAmount || selectedAmount <= 0) return
    navigate(`/payment?type=points&amount=${parseFloat(selectedAmount)}`, { replace: true })
  }

  return (
    <ScrollView scrollY className="h-screen bg-gradient-to-b from-blue-50 to-white">
      <View className="px-5 pt-12 pb-8">
        <View className="mb-2"><Text className="text-2xl font-bold text-center block">预购点数</Text></View>
        <View className="mb-8"><Text className="text-gray-500 text-center text-sm block">预购点数可抵扣租金，更优惠</Text></View>

        <View className="space-y-3 mb-8">
          {[{ amt: 100, desc: '适合短期租赁' }, { amt: 300, desc: '推荐 · 赠送 30 点' }, { amt: 500, desc: '适合长期租赁 · 赠送 80 点' }].map(p => (
            <View key={p.amt}
              className={`border-2 rounded-xl p-4 active:opacity-80 ${selectedAmount === p.amt ? 'border-blue-500 bg-blue-50' : 'border-gray-200'}`}
              onClick={() => { setAmount(p.amt); setCustomAmount('') }}>
              <View className="flex flex-row justify-between items-center">
                <View>
                  <Text className="text-lg font-bold text-black">¥{p.amt}</Text>
                  <Text className="text-xs text-gray-400 ml-1">{p.desc}</Text>
                </View>
                <View className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${selectedAmount === p.amt ? 'border-blue-500' : 'border-gray-300'}`}>
                  {selectedAmount === p.amt && <View className="w-2.5 h-2.5 rounded-full bg-blue-500" />}
                </View>
              </View>
            </View>
          ))}
        </View>

        <View className="mb-8">
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="自定义金额"
            type="digit"
            value={customAmount}
            onChange={e => { setCustomAmount(e.target.value); setAmount(0) }} />
        </View>

        <Button className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium mb-3"
          disabled={paying || !selectedAmount} onClick={handlePay}>
          {paying ? '处理中...' : '支付'}
        </Button>

        <Button className="w-full text-gray-400 text-sm py-3"
          onClick={() => navigate('/')}>跳过</Button>
      </View>
    </ScrollView>
  )
}
