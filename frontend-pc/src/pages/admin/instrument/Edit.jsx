import { useParams, useNavigate } from 'react-router-dom'
import { Button } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'

export default function InstrumentEdit() {
  const { id } = useParams()
  const navigate = useNavigate()

  return (
    <div className="p-6">
      <div className="mb-4">
        <Button 
          icon={<ArrowLeftOutlined />} 
          onClick={() => navigate('/instruments/list')}
        >
          返回列表
        </Button>
      </div>
      
      <div className="bg-white p-6 rounded-lg shadow">
        <h1 className="text-2xl font-bold mb-6">
          {id ? '编辑乐器' : '添加乐器'}
        </h1>
        
        <div className="grid grid-cols-2 gap-6">
          <div>乐器表单字段 (ID: {id})</div>
          <div>更多表单字段</div>
        </div>
        
        <div className="mt-6 flex justify-end gap-3">
          <Button onClick={() => navigate('/instruments/list')}>
            取消
          </Button>
          <Button type="primary" onClick={() => navigate('/instruments/list')}>
            保存
          </Button>
        </div>
      </div>
    </div>
  )
}