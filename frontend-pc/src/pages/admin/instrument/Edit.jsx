import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Form, Input, Select, Upload, message, Space } from 'antd'
import { ArrowLeftOutlined, UploadOutlined } from '@ant-design/icons'
import { api } from '../../services/api'

const { Option } = Select

export default function InstrumentEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [categories, setCategories] = useState([])
  const [loading, setLoading] = useState(true)

  // 浏览器退出拦截
  useEffect(() => {
    const handleBeforeUnload = (e) => {
      if (hasUnsavedChanges) {
        e.preventDefault()
        e.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [hasUnsavedChanges])

  // 加载分类数据
  useEffect(() => {
    const loadCategories = async () => {
      try {
        const response = await api.get('/categories')
        if (response.code === 20000) {
          setCategories(response.data || [])
        }
      } catch (error) {
        console.error('Failed to load categories:', error)
      }
    }
    loadCategories()
  }, [])

  // 加载乐器数据
  useEffect(() => {
    const loadInstrument = async () => {
      try {
        setLoading(true)
        const response = await api.get(`/instruments/${id}`)
        if (response.code === 20000) {
          form.setFieldsValue(response.data)
        }
      } catch (error) {
        console.error('Failed to load instrument:', error)
        message.error('加载乐器数据失败')
      } finally {
        setLoading(false)
      }
    }
    
    if (id && id !== 'add') {
      loadInstrument()
    } else {
      setLoading(false)
    }
  }, [id, form])

  const handleSave = async () => {
    try {
      const values = await form.validateFields()
      
      // 准备提交数据
      const submitData = {
        ...values,
        images: values.images?.fileList?.map(file => file.url || file.response?.url).filter(Boolean) || []
      }
      
      let response
      if (id && id !== 'add') {
        response = await api.put(`/instruments/${id}`, submitData)
      } else {
        response = await api.post('/instruments', submitData)
      }
      
      if (response.code === 20000) {
        message.success(id && id !== 'add' ? '更新成功' : '创建成功')
        navigate('/instruments/list')
      } else {
        message.error(response.message || '保存失败')
      }
    } catch (error) {
      console.error('Failed to save instrument:', error)
      message.error('保存失败')
    }
  }

  const handleCancel = () => {
    if (hasUnsavedChanges) {
      message.warning('您有未保存的修改，请先保存或确认离开')
    }
    navigate('/instruments/list')
  }

  return (
    <div className="p-6">
      <div className="mb-4">
        <Button 
          icon={<ArrowLeftOutlined />} 
          onClick={handleCancel}
        >
          返回列表
        </Button>
      </div>
      
      <div className="max-w-6xl mx-auto">
        <div className="bg-white p-6 rounded-lg shadow">
          <h1 className="text-2xl font-bold mb-6">
            {id && id !== 'add' ? '编辑乐器' : '添加乐器'}
          </h1>
          
          <Form 
            form={form} 
            layout="vertical" 
            onValuesChange={() => setHasUnsavedChanges(true)}
            disabled={loading}
          >
            {/* 两行规格布局 */}
            <div className="grid grid-cols-2 gap-6">
              <Form.Item 
                name="name" 
                label="乐器名称" 
                rules={[{ required: true, message: '请输入乐器名称' }]}
              >
                <Input placeholder="请输入乐器名称" />
              </Form.Item>
              
              <Form.Item 
                name="brand" 
                label="品牌" 
                rules={[{ required: true, message: '请输入品牌' }]}
              >
                <Input placeholder="请输入品牌" />
              </Form.Item>
              
              <Form.Item 
                name="model" 
                label="型号" 
                rules={[{ required: true, message: '请输入型号' }]}
              >
                <Input placeholder="请输入型号" />
              </Form.Item>
              
              <Form.Item 
                name="category_id" 
                label="分类"
                rules={[{ required: true, message: '请选择分类' }]}
              >
                <Select placeholder="请选择分类" loading={categories.length === 0}>
                  {categories.map(cat => (
                    <Option key={cat.id} value={cat.id}>{cat.name}</Option>
                  ))}
                </Select>
              </Form.Item>
              
              <Form.Item 
                name="status" 
                label="状态"
                rules={[{ required: true, message: '请选择状态' }]}
              >
                <Select placeholder="请选择状态">
                  <Option value="active">活跃</Option>
                  <Option value="inactive">非活跃</Option>
                </Select>
              </Form.Item>
              
              <Form.Item 
                name="stock" 
                label="库存数量"
                rules={[{ required: true, message: '请输入库存数量' }]}
              >
                <Input type="number" placeholder="请输入库存数量" />
              </Form.Item>
            </div>
            
            <Form.Item 
              name="description" 
              label="描述"
              className="mt-4"
            >
              <Input.TextArea rows={4} placeholder="请输入乐器描述" />
            </Form.Item>
            
            <Form.Item 
              name="images" 
              label="乐器图片"
              className="mt-4"
            >
              <Upload
                multiple
                accept="image/*"
                listType="picture-card"
                customRequest={({ file, onSuccess, onError }) => {
                  const formData = new FormData()
                  formData.append('file', file)
                  
                  api.post('/upload', formData)
                    .then(response => {
                      if (response.code === 20000 && response.url) {
                        onSuccess({ url: response.url })
                      } else {
                        onError(new Error('上传失败'))
                      }
                    })
                    .catch(onError)
                }}
              >
                <div>
                  <UploadOutlined />
                  <div className="mt-2">上传图片</div>
                </div>
              </Upload>
            </Form.Item>
            
            <div className="mt-6 flex justify-end gap-3">
              <Button onClick={handleCancel}>
                取消
              </Button>
              <Button type="primary" onClick={handleSave} loading={loading}>
                保存
              </Button>
            </div>
          </Form>
        </div>
      </div>
    </div>
  )
}
