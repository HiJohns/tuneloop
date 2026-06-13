import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { api, sitesApi, maintenanceApi } from '../services/api'
import ImageUploader from '../components/ImageUploader'
import SiteSelector from '../components/SiteSelector'
import { ArrowLeft, Clock, Calendar } from 'lucide-react'
import { dialog } from '../platform'

export default function Booking() {
  const navigate = useNavigate()
  const [maintenancePackages, setMaintenancePackages] = useState([])
  const [nearbySites, setNearbySites] = useState([])
  const [selectedPackage, setSelectedPackage] = useState(null)
  const [selectedSite, setSelectedSite] = useState(null)
  const [selectedDate, setSelectedDate] = useState("")
  const [selectedTime, setSelectedTime] = useState("")
  const [loading, setLoading] = useState(true)
  
  const timeSlots = [
    { value: "morning", label: "上午 (9:00-12:00)" },
    { value: "afternoon", label: "下午 (14:00-18:00)" }
  ]

  useEffect(() => {
    const fetchBookingData = async () => {
      try {
        setLoading(true)
        
        const packagesRes = await api.get('/config/maintenance-packages')
        setMaintenancePackages(packagesRes || [])
        
        const position = await new Promise((resolve) => {
          navigator.geolocation.getCurrentPosition(
            (pos) => resolve(pos.coords),
            () => resolve({ latitude: 43.8118, longitude: -79.4231 }),
            { timeout: 5000 }
          )
        })
        
        const sitesRes = await sitesApi.nearby({
          lat: position.latitude,
          lng: position.longitude
        })
        setNearbySites(sitesRes || [])
        
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch booking data:', error)
        setLoading(false)
      }
    }
    
    fetchBookingData()
  }, [])

  const getToday = () => {
    const today = new Date()
    return today.toISOString().split('T')[0]
  }

  const handleSubmit = async () => {
    if (!selectedPackage || !selectedSite || !selectedDate || !selectedTime) {
      dialog.alert("请填写完整信息")
      return
    }
    
    try {
      await maintenanceApi.submit({
        service_id: selectedPackage.id,
        site_id: selectedSite.id,
        date: selectedDate,
        time: selectedTime
      })
      dialog.alert("预约成功！")
      navigate('/')
    } catch (error) {
      console.error('Failed to submit maintenance:', error)
      dialog.alert("预约失败，请重试")
    }
  }

  return (
    <View className="min-h-screen bg-gray-50 pb-20">
      {/* Header */}
      <View className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold">维修预约</Text>
      </View>

      <View className="p-4 space-y-4">
        {loading && <View className="text-center py-8 text-gray-500">加载中...</View>}
        
        {/* Package Selection */}
        <View className="bg-white rounded-lg p-4">
          <Text className="font-medium text-gray-800 mb-3">选择服务</Text>
          <View className="space-y-2">
            {maintenancePackages.map(pkg => (
              <View
                key={pkg.id}
                onClick={() => setSelectedPackage(pkg)}
                className={`p-4 rounded-lg border-2 cursor-pointer transition-colors ${
                  selectedPackage?.id === pkg.id
                    ? 'border-orange-500 bg-orange-50'
                    : 'border-gray-200'
                }`}
              >
                <View className="flex items-start gap-3">
                  <Text className="text-2xl">{pkg.icon}</Text>
                  <View className="flex-1">
                    <View className="flex justify-between items-center">
                      <Text className="font-medium">{pkg.name}</Text>
                      <Text className="text-orange-500 font-bold">¥{pkg.price}</Text>
                    </View>
                    <Text className="text-gray-500 text-sm mt-1">{pkg.description}</Text>
                    <View className="flex items-center gap-1 text-gray-400 text-xs mt-2">
                      <Clock size={12} />
                      <Text>{pkg.duration}</Text>
                    </View>
                  </View>
                </View>
              </View>
            ))}
          </View>
        </View>

        {/* Image Upload */}
        <View className="bg-white rounded-lg p-4">
          <Text className="font-medium text-gray-800 mb-3">故障描述（可选）</Text>
          <Text className="text-gray-500 text-sm mb-3">上传故障图片或视频，帮助我们更好地了解情况</Text>
          <ImageUploader onUpload={(url) => console.log("Uploaded:", url)} />
        </View>

        {/* Site Selection */}
        <View className="bg-white rounded-lg p-4">
          <Text className="font-medium text-gray-800 mb-3">选择网点</Text>
          <SiteSelector 
            sites={nearbySites} 
            selectedSite={selectedSite}
            onSelect={setSelectedSite}
          />
        </View>

        {/* Date & Time */}
        <View className="bg-white rounded-lg p-4">
          <Text className="font-medium text-gray-800 mb-3">预约时间</Text>
          <View className="space-y-3">
            <View>
              <label className="text-gray-500 text-sm mb-1 block">选择日期</label>
              <input
                type="date"
                min={getToday()}
                value={selectedDate}
                onChange={(e) => setSelectedDate(e.target.value)}
                className="w-full p-3 border rounded-lg"
              />
            </View>
            <View>
              <label className="text-gray-500 text-sm mb-1 block">选择时段</label>
              <View className="flex gap-2">
                {timeSlots.map(slot => (
                  <Button
                    key={slot.value}
                    onClick={() => setSelectedTime(slot.value)}
                    className={`flex-1 py-2.5 rounded-lg text-sm font-medium border-2 transition-colors ${
                      selectedTime === slot.value
                        ? 'border-orange-500 bg-orange-50 text-orange-600'
                        : 'border-gray-200 text-gray-600'
                    }`}
                  >
                    {slot.label}
                  </Button>
                ))}
              </View>
            </View>
          </View>
        </View>
      </View>

      {/* Bottom Action */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <Button 
          onClick={handleSubmit}
          className="w-full bg-orange-500 text-white py-3 rounded-lg font-medium"
        >
          提交预约
        </Button>
      </View>
    </View>
  )
}