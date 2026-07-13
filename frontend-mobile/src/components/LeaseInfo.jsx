import { View, Text } from '@tarojs/components'
import { Calendar, Clock } from 'lucide-react'

export default function LeaseInfo({ status, startDate, endDate, dailyRate, rentDays, actualDays, createdAt }) {
  const notStarted = ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_transit'].includes(status)
  const inProgress = ['in_lease', 'returning'].includes(status)
  const ended = ['returned', 'completed'].includes(status)

  return (
    <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
      <Text className="text-base font-black text-black mb-3">订单信息</Text>
      <View className="space-y-3">
        {notStarted && (
          <>
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">创建日期</Text>
                <Text className="text-sm font-black text-black">{createdAt || '-'}</Text>
              </View>
            </View>
            {rentDays > 0 && (
            <View className="flex items-start gap-3">
              <Clock size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计天数</Text>
                <Text className="text-sm font-black text-black">{rentDays} 天</Text>
              </View>
            </View>
            )}
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">日租金</Text>
                <Text className="text-sm font-black text-black">¥{Number(dailyRate || 0).toFixed(2)}</Text>
              </View>
            </View>
          </>
        )}
        {inProgress && (
          <>
            {startDate && (
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">起始日期</Text>
                <Text className="text-sm font-black text-black">{startDate}</Text>
              </View>
            </View>
            )}
            {rentDays > 0 && (
            <View className="flex items-start gap-3">
              <Clock size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计天数</Text>
                <Text className="text-sm font-black text-black">{rentDays} 天</Text>
              </View>
            </View>
            )}
            {actualDays > 0 && (
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">租赁天数</Text>
                <Text className="text-sm font-black text-black">{actualDays} 天</Text>
              </View>
            </View>
            )}
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">日租金</Text>
                <Text className="text-sm font-black text-black">¥{Number(dailyRate || 0).toFixed(2)}</Text>
              </View>
            </View>
          </>
        )}
        {ended && (
          <>
            {startDate && (
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">起始日期</Text>
                <Text className="text-sm font-black text-black">{startDate}</Text>
              </View>
            </View>
            )}
            {endDate && (
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">结束日期</Text>
                <Text className="text-sm font-black text-black">{endDate}</Text>
              </View>
            </View>
            )}
            {actualDays > 0 && (
            <View className="flex items-start gap-3">
              <Clock size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">租赁天数</Text>
                <Text className="text-sm font-black text-black">{actualDays} 天</Text>
              </View>
            </View>
            )}
            <View className="flex items-start gap-3">
              <Calendar size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">日租金</Text>
                <Text className="text-sm font-black text-black">¥{Number(dailyRate || 0).toFixed(2)}</Text>
              </View>
            </View>
          </>
        )}
      </View>
    </View>
  )
}
