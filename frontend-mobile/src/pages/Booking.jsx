import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, sitesApi, maintenanceApi } from '../services/api'
import ImageUploader from '../components/ImageUploader'
import SiteSelector from '../components/SiteSelector'
import { ArrowLeft, Clock, Calendar } from 'lucide-react'

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
      alert("请填写完整信息")
      return
    }
    
    try {
      await maintenanceApi.submit({
        service_id: selectedPackage.id,
        site_id: selectedSite.id,
        date: selectedDate,
        time: selectedTime
      })
      alert("预约成功！")
      navigate('/')
    } catch (error) {
      console.error('Failed to submit maintenance:', error)
      alert("预约失败，请重试")
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      {/* Header */}
      <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">维修预约</h1>
      </div>

      <div className="p-4 space-y-4">
        {loading && <div className="text-center py-8 text-gray-500">加载中...</div>}
        
        {/* Package Selection */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">选择服务</h2>
          <div className="space-y-2">
            {maintenancePackages.map(pkg => (
              <div
                key={pkg.id}
                onClick={() => setSelectedPackage(pkg)}
                className={`p-4 rounded-lg border-2 cursor-pointer transition-colors ${
                  selectedPackage?.id === pkg.id
                    ? 'border-orange-500 bg-orange-50'
                    : 'border-gray-200'
                }`}
              >
                <div className="flex items-start gap-3">
                  <span className="text-2xl">{pkg.icon}</span>
                  <div className="flex-1">
                    <div className="flex justify-between items-center">
                      <h3 className="font-medium">{pkg.name}</h3>
                      <span className="text-orange-500 font-bold">¥{pkg.price}</span>
                    </div>
                    <p className="text-gray-500 text-sm mt-1">{pkg.description}</p>
                    <div className="flex items-center gap-1 text-gray-400 text-xs mt-2">
                      <Clock size={12} />
                      <span>{pkg.duration}</span>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Image Upload */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">故障描述（可选）</h2>
          <p className="text-gray-500 text-sm mb-3">上传故障图片或视频，帮助我们更好地了解情况</p>
          <ImageUploader onUpload={(url) => console.log("Uploaded:", url)} />
        </div>

        {/* Site Selection */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">选择网点</h2>
          <SiteSelector 
            sites={nearbySites} 
            selectedSite={selectedSite}
            onSelect={setSelectedSite}
          />
        </div>

        {/* Date & Time */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">预约时间</h2>
          <div className="space-y-3">
            <div>
              <label className="text-gray-500 text-sm mb-1 block">选择日期</label>
              <input
                type="date"
                min={getToday()}
                value={selectedDate}
                onChange={(e) => setSelectedDate(e.target.value)}
                className="w-full p-3 border rounded-lg"
              />
            </div>
            <div>
              <label className="text-gray-500 text-sm mb-1 block">选择时段</label>
              <div className="flex gap-2">
                {timeSlots.map(slot => (
                  <button
                    key={slot.value}
                    onClick={() => setSelectedTime(slot.value)}
                    className={`flex-1 py-2.5 rounded-lg text-sm font-medium border-2 transition-colors ${
                      selectedTime === slot.value
                        ? 'border-orange-500 bg-orange-50 text-orange-600'
                        : 'border-gray-200 text-gray-600'
                    }`}
                  >
                    {slot.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Bottom Action */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <button 
          onClick={handleSubmit}
          className="w-full bg-orange-500 text-white py-3 rounded-lg font-medium"
        >
          提交预约
        </button>
      </div>
    </div>
  )
}