import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { Badge, Tag, Button, Modal, message, Form, Input } from 'antd'
import { ArrowLeft, Phone, Calendar, CheckCircle, CloseCircle, Clock } from 'lucide-react'
import { useUser } from '../context/UserContext'

function UserServiceCard({ order, onSubmitNew }) {
  const navigate = useNavigate()
  
  return (
    <div className="bg-white rounded-xl shadow-sm p-4">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="font-medium text-brand-text">{order.assetName}</h3>
          <Tag color={order.status === "处理中" ? "blue" : "orange"}>
            {order.status}
          </Tag>
        </div>
        
        <p className="text-gray-600 text-sm">故障: {order.fault}</p>
        
        {order.status === "待派单" && (
          <p className="text-gray-500 text-sm">备注: {order.site}</p>
        )}
        
        {order.status === "处理中" && (
          <div className="flex items-center gap-2">
            <span className="text-sm">服务人员: {order.technician}</span>
            <a href={`tel:${order.technicianPhone}`} className="text-brand-primary text-sm">
              📞 {order.technicianPhone}
            </a>
          </div>
        )}
        
        <p className="text-gray-400 text-xs">创建时间: {order.createdAt}</p>
      </div>
    </div>
  )
}

function TechnicianTicketCard({ ticket, onAccept, onComplete }) {
  const [completing, setCompleting] = useState(false)
  
  const handleComplete = () => {
    onComplete(ticket.id)
  }
  
  return (
    <div className="bg-white rounded-xl shadow-sm p-4 border border-gray-200">
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h3 className="font-medium text-brand-text">{ticket.assetName}</h3>
            <Tag color={ticket.status === "PROCESSING" ? "blue" : ticket.status === "PENDING" ? "orange" : "green"}>
              {ticket.status}
            </Tag>
          </div>
          <span className="text-xs text-gray-500">{ticket.createdAt}</span>
        </div>
        
        <div className="space-y-1">
          <p className="text-gray-600 text-sm"><span className="font-medium">客户:</span> {ticket.customerName}</p>
          <p className="text-gray-600 text-sm"><span className="font-medium">联系方式:</span> {ticket.customerPhone}</p>
          <p className="text-gray-600 text-sm"><span className="font-medium">地址:</span> {ticket.address}</p>
          <p className="text-gray-600 text-sm"><span className="font-medium">故障描述:</span> {ticket.fault}</p>
        </div>
        
        {ticket.status === "PENDING" && (
          <Button 
            type="primary"
            size="small"
            onClick={() => onAccept(ticket.id)}
            className="w-full"
          >
            <Clock size={14} className="mr-1 inline" />
            接单
          </Button>
        )}
        
        {ticket.status === "PROCESSING" && (
          <Button 
            type="primary"
            size="small"
            onClick={handleComplete}
            loading={completing}
            className="w-full bg-green-600 hover:bg-green-700 border-green-600"
          >
            <CheckCircle size={14} className="mr-1 inline" />
            完成维修
          </Button>
        )}
        
        {ticket.status === "COMPLETED" && (
          <div className="text-green-600 text-sm flex items-center gap-1">
            <CheckCircle size={14} />
            已完成
          </div>
        )}
      </div>
    </div>
  )
}

function RepairReportModal({ visible, onSubmit, onCancel }) {
  const [form] = Form.useForm()
  
  const handleSubmit = () => {
    form.validateFields().then(values => {
      onSubmit(values)
      form.resetFields()
    }).catch(error => {
      console.error('Validation failed:', error)
    })
  }
  
  return (
    <Modal
      title="维修报告"
      open={visible}
      onOk={handleSubmit}
      onCancel={onCancel}
      okText="提交报告"
      cancelText="取消"
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="report"
          label="维修报告"
          rules={[{ required: true, message: '请输入维修报告' }]}
        >
          <Input.TextArea 
            rows={4} 
            placeholder="请详细描述维修过程、更换的零件、维修结果等信息..."
          />
        </Form.Item>
        
        <Form.Item
          name="parts_used"
          label="使用的零件"
        >
          <Input.TextArea 
            rows={2} 
            placeholder="列出使用的零件（可选）"
          />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function TechnicianHall() {
  const [tickets, setTickets] = useState([])
  const [loading, setLoading] = useState(true)
  const [reportModalVisible, setReportModalVisible] = useState(false)
  const [currentTicketId, setCurrentTicketId] = useState(null)
  
  useEffect(() => {
    fetchTickets()
  }, [])
  
  const fetchTickets = async () => {
    try {
      setLoading(true)
      const data = await api.get('/technician/tickets')
      setTickets(data || [])
      setLoading(false)
    } catch (error) {
      console.error('Failed to fetch tickets:', error)
      message.error('加载工单失败')
      setLoading(false)
    }
  }
  
  const handleAcceptTicket = async (ticketId) => {
    try {
      await api.put(`/technician/tickets/${ticketId}/accept`)
      message.success('已接单')
      fetchTickets()
    } catch (error) {
      console.error('Failed to accept ticket:', error)
      message.error('接单失败')
    }
  }
  
  const handleCompleteTicket = (ticketId) => {
    setCurrentTicketId(ticketId)
    setReportModalVisible(true)
  }
  
  const handleSubmitReport = async (reportData) => {
    try {
      await api.post(`/technician/tickets/${currentTicketId}/complete`, reportData)
      message.success('维修报告已提交')
      setReportModalVisible(false)
      setCurrentTicketId(null)
      fetchTickets()
    } catch (error) {
      console.error('Failed to submit report:', error)
      message.error('提交报告失败')
    }
  }
  
  return (
    <>
      <div className="min-h-screen bg-brand-bg pb-20">
        {/* Header */}
        <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
          <h1 className="text-lg font-bold">维保工单大厅</h1>
        </div>
        
        {/* Tickets List */}
        <div className="p-4 space-y-4">
          {loading ? (
            <div className="text-center py-8 text-gray-500">加载中...</div>
          ) : tickets.length === 0 ? (
            <div className="text-center py-8 text-gray-500">暂无工单</div>
          ) : (
            tickets.map(ticket => (
              <TechnicianTicketCard 
                key={ticket.id} 
                ticket={ticket}
                onAccept={handleAcceptTicket}
                onComplete={handleCompleteTicket}
              />
            ))
          )}
        </div>
      </div>
      
      <RepairReportModal
        visible={reportModalVisible}
        onSubmit={handleSubmitReport}
        onCancel={() => {
          setReportModalVisible(false)
          setCurrentTicketId(null)
        }}
      />
    </>
  )
}

export default function MyService() {
  const navigate = useNavigate()
  const { isTechnician } = useUser()
  const [myServiceOrders, setMyServiceOrders] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!isTechnician()) {
      const fetchServiceOrders = async () => {
        try {
          setLoading(true)
          const data = await api.get('/user/service-orders')
          setMyServiceOrders(data || [])
          setLoading(false)
        } catch (error) {
          console.error('Failed to fetch service orders:', error)
          setLoading(false)
        }
      }
      
      fetchServiceOrders()
    }
  }, [])
  
  if (isTechnician()) {
    return <TechnicianHall />
  }
  
  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      {/* Header */}
      <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">我的维修</h1>
      </div>
      
      {/* Service Orders List */}
      <div className="p-4 space-y-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">加载中...</div>
        ) : (
          myServiceOrders.map(order => (
            <UserServiceCard key={order.id} order={order} />
          ))
        )}
      </div>
      
      {/* Bottom Navigation */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <span className="text-xl">🏠</span>
            <span className="text-xs mt-1">首页</span>
          </div>
          <div 
            className="flex flex-col items-center text-brand-primary cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <span className="text-xl">🔧</span>
            <span className="text-xs mt-1">维修</span>
          </div>
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/profile')}
          >
            <span className="text-xl">👤</span>
            <span className="text-xs mt-1">我的</span>
          </div>
        </div>
      </div>
    </div>
  )
}