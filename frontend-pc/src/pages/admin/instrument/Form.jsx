import { useState, useEffect, useCallback } from 'react'
import { Modal, Form, Input, Select, Upload, Switch, message, Button, InputNumber, Row, Col, Divider, Space, Card } from 'antd'
import { UploadOutlined, PlusOutlined, DeleteOutlined, DragOutlined } from '@ant-design/icons'
import { arrayMove } from '@dnd-kit/sortable';
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

const { Option } = Select
const { TextArea } = Input

// Sortable image item component
const SortableImageItem = ({ file, onRemove }) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: file.uid });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className="relative inline-block m-2"
    >
      <img src={file.url} alt={file.name} className="w-24 h-24 object-cover rounded" />
      <div className="absolute top-0 right-0 flex">
        <Button
          size="small"
          icon={<DragOutlined />}
          {...attributes}
          {...listeners}
          className="cursor-move"
        />
        <Button
          size="small"
          icon={<DeleteOutlined />}
          onClick={() => onRemove(file.uid)}
          danger
        />
      </div>
    </div>
  );
};

export default function InstrumentForm({ visible, onCancel, onSubmit, initialData = null, categories = [] }) {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [fileList, setFileList] = useState([])
  const [specs, setSpecs] = useState([])
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'
  
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  useEffect(() => {
    if (visible) {
      if (initialData) {
        form.setFieldsValue(initialData)
        // Set file list if images exist
        if (initialData.images && initialData.images.length > 0) {
          setFileList(initialData.images.map((url, index) => ({
            uid: `initial-${index}`,
            name: `image${index}`,
            status: 'done',
            url: url
          })))
        }
        // Set specs
        setSpecs(initialData.specs || [])
      } else {
        form.resetFields()
        setFileList([])
        setSpecs([{ id: Date.now(), name: '', daily_rent: 0, weekly_rent: 0, monthly_rent: 0, deposit: 0, stock: 0 }])
      }
    }
  }, [visible, initialData])

  const beforeUpload = (file) => {
    // Generate preview URL using FileReader
    const reader = new FileReader()
    reader.onload = (e) => {
      setFileList(prev => {
        const fileIndex = prev.findIndex(f => f.uid === file.uid)
        if (fileIndex >= 0) {
          const newList = [...prev]
          newList[fileIndex] = { ...newList[fileIndex], url: e.target.result }
          return newList
        }
        return prev
      })
    }
    reader.readAsDataURL(file)
    return true // Allow upload to proceed
  }

  const handleUploadChange = ({ fileList: newFileList }) => {
    setFileList(newFileList.map(file => {
      if (file.response && file.response.code === 20000) {
        return { ...file, url: file.response.data.url }
      }
      return file
    }).filter(file => file.status !== 'error'))
  }

  const handleDragEnd = (event) => {
    const { active, over } = event;
    if (active.id !== over.id) {
      setFileList((items) => {
        const oldIndex = items.findIndex((item) => item.uid === active.id);
        const newIndex = items.findIndex((item) => item.uid === over.id);
        return arrayMove(items, oldIndex, newIndex);
      });
    }
  }

  const removeImage = (uid) => {
    setFileList(fileList.filter(file => file.uid !== uid))
  }

  // Specs management
  const addSpec = () => {
    const newSpec = {
      id: Date.now(),
      name: '',
      daily_rent: 0,
      weekly_rent: 0,
      monthly_rent: 0,
      deposit: 0,
      stock: 0
    }
    setSpecs([...specs, newSpec])
  }

  const removeSpec = (id) => {
    if (specs.length <= 1) {
      message.warning('至少需要保留一个规格')
      return
    }
    setSpecs(specs.filter(spec => spec.id !== id))
  }

  const updateSpec = (id, field, value) => {
    setSpecs(specs.map(spec => 
      spec.id === id ? { ...spec, [field]: value } : spec
    ))
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setLoading(true)
      
      // Extract image URLs from fileList
      const images = fileList
        .filter(file => file.status === 'done' && file.url)
        .map(file => file.url)
      
      // Prepare specs data
      const processedSpecs = specs.map(spec => ({
        name: spec.name,
        daily_rent: spec.daily_rent || 0,
        weekly_rent: spec.weekly_rent || 0,
        monthly_rent: spec.monthly_rent || 0,
        deposit: spec.deposit || 0,
        stock: spec.stock || 0
      })).filter(spec => spec.name && spec.monthly_rent > 0)
      
      // Prepare form data
      const formData = {
        name: values.name,
        brand: values.brand,
        model: values.model,
        category_id: values.category_id,
        level: values.level,
        description: values.description,
        images: images,
        video: values.video || '',
        status: values.status || 'active',
        pricing_tiers: values.pricing_tiers || [],
        specs: processedSpecs
      }
      
      // Submit to API
      const url = initialData ? `${API_BASE_URL}/instruments/${initialData.id}` : `${API_BASE_URL}/instruments`
      const method = initialData ? 'PUT' : 'POST'
      
      const response = await fetch(url, {
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData)
      })
      
      if (!response.ok) throw new Error('提交失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success(initialData ? '更新成功' : '创建成功')
        onSubmit(result.data)
        form.resetFields()
        setFileList([])
        setSpecs([{ id: Date.now(), name: '', daily_rent: 0, weekly_rent: 0, monthly_rent: 0, deposit: 0, stock: 0 }])
      } else {
        throw new Error(result.message || '提交失败')
      }
    } catch (error) {
      if (error.errorFields) {
        // Form validation error
        console.error('Validation failed:', error)
      } else {
        message.error(error.message || '提交失败')
      }
    } finally {
      setLoading(false)
    }
  }

  const title = initialData ? '编辑乐器' : '新增乐器'

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      width={800}
      bodyStyle={{ 
        maxHeight: '70vh', 
        overflowY: 'auto', 
        overflowX: 'hidden' 
      }}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          status: 'active',
          stock: 0,
          level: 'beginner'
        }}
      >
        <Divider orientation="left">基本信息</Divider>
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="name"
              label="乐器名称"
              rules={[
                { required: true, message: '请输入乐器名称' },
                { min: 2, message: '乐器名称至少需要2个字符' },
                { max: 100, message: '乐器名称不能超过100个字符' }
              ]}
            >
              <Input placeholder="请输入乐器名称，如：雅马哈立式钢琴 U1" />
            </Form.Item>
          </Col>
          
          <Col span={12}>
            <Form.Item
              name="category_id"
              label="分类"
              rules={[{ required: true, message: '请选择分类' }]}
            >
              <Select placeholder="请选择分类">
                {categories.map(cat => (
                  <Option key={cat.id} value={cat.id}>{cat.name}</Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={8}>
            <Form.Item
              name="brand"
              label="品牌"
              rules={[{ required: true, message: '请输入品牌' }]}
            >
              <Input placeholder="如：Yamaha" />
            </Form.Item>
          </Col>
          
          <Col span={8}>
            <Form.Item
              name="model"
              label="型号"
              rules={[{ required: true, message: '请输入型号' }]}
            >
              <Input placeholder="如：U1" />
            </Form.Item>
          </Col>
          
          <Col span={8}>
            <Form.Item
              name="level"
              label="级别"
              rules={[{ required: true, message: '请选择级别' }]}
            >
              <Select placeholder="请选择级别">
                <Option value="beginner">入门级</Option>
                <Option value="intermediate">中级</Option>
                <Option value="advanced">高级</Option>
                <Option value="professional">专业级</Option>
              </Select>
            </Form.Item>
          </Col>
        </Row>

        <Form.Item
          name="description"
          label="描述"
          rules={[{ max: 500, message: '描述不能超过500个字符' }]}
        >
          <TextArea rows={3} placeholder="请输入乐器描述" />
        </Form.Item>

        <Divider orientation="left">图片和视频</Divider>
        
        <Form.Item
          name="images"
          label="图片"
          extra="拖拽可调整图片顺序，建议尺寸 800x600"
        >
          <Upload
            listType="picture-card"
            fileList={fileList}
            onChange={handleUploadChange}
            beforeUpload={beforeUpload}
            action={`${API_BASE_URL}/upload`}
            multiple
            accept="image/*"
            showUploadList={false}
          >
            <div>
              <UploadOutlined />
              <div style={{ marginTop: 8 }}>点击或拖拽上传</div>
            </div>
          </Upload>
          
          {/* Drag and drop sortable image list */}
          {fileList.length > 0 && (
            <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
              <SortableContext items={fileList.map(f => f.uid)} strategy={verticalListSortingStrategy}>
                <div className="mt-4">
                  {fileList.map((file) => (
                    <SortableImageItem key={file.uid} file={file} onRemove={removeImage} />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          )}
        </Form.Item>

        <Form.Item
          name="video"
          label="视频URL"
        >
          <Input placeholder="请输入视频URL（可选）" />
        </Form.Item>

        <Divider orientation="left">价格配置</Divider>
        
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item
              name="daily_rate"
              label="日租金 (¥)"
              rules={[{ required: true, message: '请输入日租金' }]}
            >
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
          </Col>
          
          <Col span={6}>
            <Form.Item
              name="weekly_rate"
              label="周租金 (¥)"
            >
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
          </Col>
          
          <Col span={6}>
            <Form.Item
              name="monthly_rate"
              label="月租金 (¥)"
              rules={[{ required: true, message: '请输入月租金' }]}
            >
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
          </Col>
          
          <Col span={6}>
            <Form.Item
              name="deposit"
              label="押金 (¥)"
              rules={[{ required: true, message: '请输入押金' }]}
            >
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">规格配置</Divider>
        
        <div className="mb-4">
          {specs.map((spec, index) => (
            <Card key={spec.id} size="small" className="mb-3">
              <Row gutter={16} align="middle">
                <Col span={4}>
                  <Input
                    placeholder="规格名称"
                    value={spec.name}
                    onChange={(e) => updateSpec(spec.id, 'name', e.target.value)}
                  />
                </Col>
                <Col span={3}>
                  <InputNumber
                    placeholder="日租金"
                    value={spec.daily_rent}
                    onChange={(value) => updateSpec(spec.id, 'daily_rent', value)}
                    style={{ width: '100%' }}
                    min={0}
                  />
                </Col>
                <Col span={3}>
                  <InputNumber
                    placeholder="周租金"
                    value={spec.weekly_rent}
                    onChange={(value) => updateSpec(spec.id, 'weekly_rent', value)}
                    style={{ width: '100%' }}
                    min={0}
                  />
                </Col>
                <Col span={3}>
                  <InputNumber
                    placeholder="月租金"
                    value={spec.monthly_rent}
                    onChange={(value) => updateSpec(spec.id, 'monthly_rent', value)}
                    style={{ width: '100%' }}
                    min={0}
                  />
                </Col>
                <Col span={3}>
                  <InputNumber
                    placeholder="押金"
                    value={spec.deposit}
                    onChange={(value) => updateSpec(spec.id, 'deposit', value)}
                    style={{ width: '100%' }}
                    min={0}
                  />
                </Col>
                <Col span={3}>
                  <InputNumber
                    placeholder="库存"
                    value={spec.stock}
                    onChange={(value) => updateSpec(spec.id, 'stock', value)}
                    style={{ width: '100%' }}
                    min={0}
                  />
                </Col>
                <Col span={2}>
                  <Button
                    icon={<DeleteOutlined />}
                    onClick={() => removeSpec(spec.id)}
                    danger
                    disabled={specs.length <= 1}
                  />
                </Col>
              </Row>
            </Card>
          ))}
          <Button icon={<PlusOutlined />} onClick={addSpec} type="dashed">
            添加规格
          </Button>
        </div>

        <Divider orientation="left">价格阶梯配置（可选）</Divider>
        
        <Form.List name="pricing_tiers">
          {(fields, { add, remove }) => (
            <>
              {fields.map(({ key, name, ...restField }) => (
                <Card key={key} size="small" className="mb-3">
                  <Row gutter={16}>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'min_days']}
                        rules={[{ required: true, message: '请输入最小天数' }]}
                      >
                        <InputNumber placeholder="最小天数" min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'max_days']}
                      >
                        <InputNumber placeholder="最大天数" min={1} style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item
                        {...restField}
                        name={[name, 'discount']}
                        rules={[{ required: true, message: '请输入折扣率' }]}
                      >
                        <InputNumber
                          placeholder="折扣率"
                          min={0.1}
                          max={1}
                          step={0.05}
                          style={{ width: '100%' }}
                          addonAfter="%"
                        />
                      </Form.Item>
                    </Col>
                    <Col span={4}>
                      <Button icon={<DeleteOutlined />} onClick={() => remove(name)} danger />
                    </Col>
                  </Row>
                </Card>
              ))}
              <Button icon={<PlusOutlined />} onClick={() => add()} type="dashed">
                添加价格阶梯
              </Button>
            </>
          )}
        </Form.List>

        <Divider orientation="left">库存和状态</Divider>
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="stock"
              label="库存数量"
              rules={[{ required: true, message: '请输入库存数量' }]}
            >
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
          </Col>
          
          <Col span={12}>
            <Form.Item
              name="status"
              label="状态"
              rules={[{ required: true, message: '请选择状态' }]}
            >
              <Select placeholder="请选择状态">
                <Option value="active">可租</Option>
                <Option value="inactive">下架</Option>
                <Option value="maintenance">维修中</Option>
              </Select>
            </Form.Item>
          </Col>
        </Row>
      </Form>
    </Modal>
  )
}