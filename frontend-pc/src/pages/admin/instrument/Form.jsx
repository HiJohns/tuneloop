import { useState, useEffect, useCallback } from 'react'
import { Modal, Form, Input, Select, Upload, Switch, message, Button, InputNumber, Row, Col, Divider, Space, Card, Progress } from 'antd'
import { UploadOutlined, PlusOutlined, DeleteOutlined, DragOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { arrayMove } from '@dnd-kit/sortable';
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

const { Option } = Select
const { TextArea } = Input

// Sortable image item component
const SortableImageItem = ({ file, onRemove, uploadStatus, onRetry }) => {
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

  const progress = uploadStatus.progress[file.uid];
  const isFailed = uploadStatus.failedFiles.includes(file.uid);

  return (
    <div
      ref={setNodeRef}
      style={style}
      className="relative inline-block m-2"
    >
      <img src={file.url} alt={file.name} className="w-24 h-24 object-cover rounded" />
      
      {/* Progress bar for uploading files */}
      {progress && progress.percent < 100 && (
        <div className="absolute bottom-0 left-0 right-0 bg-black bg-opacity-50 rounded-b">
          <Progress 
            percent={progress.percent} 
            showInfo={false}
            strokeColor="#52c41a"
          />
        </div>
      )}
      
      {/* Status overlay */}
      {isFailed && (
        <div className="absolute inset-0 bg-red-500 bg-opacity-50 flex items-center justify-center rounded">
          <CloseCircleOutlined style={{ color: 'white', fontSize: 24 }} />
        </div>
      )}
      {progress && progress.percent === 100 && (
        <div className="absolute inset-0 bg-green-500 bg-opacity-50 flex items-center justify-center rounded">
          <CheckCircleOutlined style={{ color: 'white', fontSize: 24 }} />
        </div>
      )}
      
      <div className="absolute top-0 right-0 flex">
        <Button
          size="small"
          icon={<DragOutlined />}
          {...attributes}
          {...listeners}
          className="cursor-move"
        />
        {isFailed ? (
          <Button
            size="small"
            icon={<ReloadOutlined />}
            onClick={() => onRetry(file)}
            type="primary"
          />
        ) : (
          <Button
            size="small"
            icon={<DeleteOutlined />}
            onClick={() => onRemove(file.uid)}
            danger
          />
        )}
      </div>
    </div>
  );
};

export default function InstrumentForm({ open, onCancel, onSubmit, initialData = null, categories = [] }) {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [fileList, setFileList] = useState([])
  const [specs, setSpecs] = useState([])
  const [uploadStatus, setUploadStatus] = useState({
    isUploading: false,
    progress: {},
    failedFiles: []
  })
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'
  
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  useEffect(() => {
    if (open) {
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
  }, [open, initialData])

  const beforeUpload = (file) => {
    console.log('[DEBUG] beforeUpload called for file:', file.name)
    // Generate preview URL using FileReader
    const reader = new FileReader()
    reader.onload = (e) => {
      console.log('[DEBUG] FileReader onload, setting preview URL')
      setFileList(prev => {
        const fileIndex = prev.findIndex(f => f.uid === file.uid)
        if (fileIndex >= 0) {
          const newList = [...prev]
          newList[fileIndex] = { ...newList[fileIndex], url: e.target.result }
          console.log('[DEBUG] Updated fileList with preview URL:', newList[fileIndex])
          return newList
        }
        return prev
      })
    }
    reader.readAsDataURL(file)
    return false // Prevent automatic upload, handle manually
  }

  const uploadFileWithProgress = (file, onProgress) => {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      
      // Progress tracking
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          const percent = Math.round((event.loaded / event.total) * 100)
          onProgress(percent)
        }
      }
      
      xhr.onload = () => {
        if (xhr.status === 200) {
          const result = JSON.parse(xhr.responseText)
          resolve(result.data?.url || result.url)
        } else {
          reject(new Error('Upload failed'))
        }
      }
      
      xhr.onerror = () => reject(new Error('Network error'))
      
      xhr.open('POST', `${API_BASE_URL}/upload`)
      const formData = new FormData()
      formData.append('file', file.originFileObj || file)
      xhr.send(formData)
    })
  }

  const handleUploadChange = ({ fileList: newFileList }) => {
    console.log('[DEBUG] handleUploadChange called with fileList:', newFileList)
    
    const processedList = newFileList.map(file => {
      // Handle different response formats from upload API
      if (file.response) {
        console.log('[DEBUG] File response:', file.name, file.response)
        
        // Format 1: { code: 20000, data: { url: "..." } }
        if (file.response.code === 20000 && file.response.data?.url) {
          console.log('[DEBUG] Format 1 detected, URL:', file.response.data.url)
          return { ...file, url: file.response.data.url }
        }
        // Format 2: { success: true, url: "..." }
        else if (file.response.success && file.response.url) {
          console.log('[DEBUG] Format 2 detected, URL:', file.response.url)
          return { ...file, url: file.response.url }
        }
        // Format 3: Direct url in response
        else if (file.response.url) {
          console.log('[DEBUG] Format 3 detected, URL:', file.response.url)
          return { ...file, url: file.response.url }
        }
      }
      return file
    }).filter(file => file.status !== 'error')
    
    console.log('[DEBUG] Processed fileList:', processedList)
    setFileList(processedList)
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

  const calculateRentals = (dailyRent) => {
    if (!dailyRent || dailyRent <= 0) {
      return { weekly_rent: 0, monthly_rent: 0, deposit: 0 }
    }
    return {
      weekly_rent: Math.round(dailyRent * 6),
      monthly_rent: Math.round(dailyRent * 25),
      deposit: Math.round(dailyRent * 20)
    }
  }

  const updateSpec = (id, field, value) => {
    setSpecs(specs.map(spec => {
      if (spec.id === id) {
        const updated = { ...spec, [field]: value }
        if (field === 'daily_rent') {
          const calculated = calculateRentals(value)
          updated.weekly_rent = calculated.weekly_rent
          updated.monthly_rent = calculated.monthly_rent
          updated.deposit = calculated.deposit
        }
        return updated
      }
      return spec
    }))
  }

  const syncPricesToAll = () => {
    if (specs.length <= 1) return
    const firstSpec = specs[0]
    const ratio = {
      weekly: firstSpec.daily_rent ? firstSpec.weekly_rent / firstSpec.daily_rent : 6,
      monthly: firstSpec.daily_rent ? firstSpec.monthly_rent / firstSpec.daily_rent : 25,
      deposit: firstSpec.daily_rent ? firstSpec.deposit / firstSpec.daily_rent : 20
    }
    setSpecs(specs.map((spec, idx) => {
      if (idx === 0) return spec
      return {
        ...spec,
        weekly_rent: Math.round(spec.daily_rent * ratio.weekly),
        monthly_rent: Math.round(spec.daily_rent * ratio.monthly),
        deposit: Math.round(spec.daily_rent * ratio.deposit)
      }
    }))
  }

  const uploadPendingFiles = async () => {
    const pendingFiles = fileList.filter(file => file.status !== 'done')
    const previouslyUploadedImages = fileList
      .filter(file => file.status === 'done' && file.url)
      .map(file => file.url)
    
    if (pendingFiles.length === 0) {
      return { success: true, uploadedImages: previouslyUploadedImages }
    }
    
    const newlyUploadedImages = []
    const failedFiles = []
    
    // Initialize progress tracking for all pending files
    setUploadStatus(prev => ({
      ...prev,
      isUploading: true,
      progress: pendingFiles.reduce((acc, file) => ({
        ...acc,
        [file.uid]: { percent: 0, status: 'uploading' }
      }), {}),
      failedFiles: []
    }))
    
    const uploadPromises = pendingFiles.map(async (file) => {
      try {
        const onProgress = (percent) => {
          setUploadStatus(prev => ({
            ...prev,
            progress: {
              ...prev.progress,
              [file.uid]: { percent, status: 'uploading' }
            }
          }))
        }
        
        const uploadedUrl = await uploadFileWithProgress(file, onProgress)
        newlyUploadedImages.push(uploadedUrl)
        
        // Update fileList with uploaded status
        setFileList(prev => prev.map(f => {
          if (f.uid === file.uid) {
            return { ...f, status: 'done', url: uploadedUrl }
          }
          return f
        }))
        
        // Mark as completed
        setUploadStatus(prev => ({
          ...prev,
          progress: {
            ...prev.progress,
            [file.uid]: { percent: 100, status: 'done' }
          }
        }))
        
        return { success: true, file }
      } catch (error) {
        console.error('Upload failed for', file.name, ':', error)
        failedFiles.push(file)
        
        setUploadStatus(prev => ({
          ...prev,
          progress: {
            ...prev.progress,
            [file.uid]: { percent: 0, status: 'error' }
          },
          failedFiles: [...prev.failedFiles, file]
        }))
        
        return { success: false, file, error }
      }
    })
    
    const results = await Promise.all(uploadPromises)
    const allSuccess = results.every(r => r.success)
    
    setUploadStatus(prev => ({ ...prev, isUploading: false }))
    
    if (!allSuccess) {
      message.error('部分图片上传失败，请重试')
      return { success: false, uploadedImages: [], failedFiles }
    }
    
    return { 
      success: true, 
      uploadedImages: [...previouslyUploadedImages, ...newlyUploadedImages] 
    }
  }

  const retryUpload = async (file) => {
    const onProgress = (percent) => {
      setUploadStatus(prev => ({
        ...prev,
        progress: {
          ...prev.progress,
          [file.uid]: { percent, status: 'uploading' }
        }
      }))
    }
    
    try {
      const uploadedUrl = await uploadFileWithProgress(file, onProgress)
      
      setFileList(prev => prev.map(f => {
        if (f.uid === file.uid) {
          return { ...f, status: 'done', url: uploadedUrl }
        }
        return f
      }))
      
      setUploadStatus(prev => ({
        ...prev,
        progress: {
          ...prev.progress,
          [file.uid]: { percent: 100, status: 'done' }
        },
        failedFiles: prev.failedFiles.filter(f => f.uid !== file.uid)
      }))
      
      message.success(`${file.name} 上传成功`)
    } catch (error) {
      message.error(`${file.name} 重试失败`)
    }
  }

  const handleSubmit = async () => {
    console.log('[DEBUG] ==== handleSubmit START ====')
    
    try {
      const values = await form.validateFields()
      
      // Check if any files are uploading or failed
      if (uploadStatus.isUploading) {
        message.warning('请等待图像上传完成')
        return
      }
      
      if (uploadStatus.failedFiles.length > 0) {
        message.error('请先处理上传失败的图片')
        return
      }
      
      setLoading(true)
      
      console.log('[DEBUG] Form values:', values)
      
      // Upload pending files first and get the uploaded image URLs
      let images = []
      if (fileList.length > 0) {
        const uploadResult = await uploadPendingFiles()
        console.log('[DEBUG] Upload result:', uploadResult)
        if (!uploadResult.success) {
          console.error('[DEBUG] Upload failed, aborting submit')
          setLoading(false)
          return
        }
        images = uploadResult.uploadedImages || []
      }
      
      console.log('[DEBUG] Final images array:', images)
      
      // Prepare specs data
      const processedSpecs = specs.map(spec => ({
        name: spec.name,
        daily_rent: spec.daily_rent || 0,
        weekly_rent: spec.weekly_rent || 0,
        monthly_rent: spec.monthly_rent || 0,
        deposit: spec.deposit || 0,
        stock: spec.stock || 0
      })).filter(spec => spec.name && spec.monthly_rent > 0)
      
      // Auto-calculate total stock from specs
      const totalStock = processedSpecs.reduce((sum, spec) => sum + (spec.stock || 0), 0)
      
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
        status: initialData ? (values.status || 'active') : 'active',
        stock: totalStock,
        pricing_tiers: values.pricing_tiers || [],
        specs: processedSpecs
      }
      
      console.log('[DEBUG] ==== PREPARING TO SEND POST /api/instruments ====')
      console.log('[DEBUG] Request body (formData):', JSON.stringify(formData, null, 2))
      console.log('[DEBUG] ==== LAUNCHING REQUEST ====')
      
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
      
      console.log('[DEBUG] Response status:', response.status)
      
      if (!response.ok) throw new Error('提交失败')
      
      const result = await response.json()
      if (result.code === 20000 || result.code === 20100) {
        message.success(initialData ? '更新成功' : '创建成功')
        onSubmit(result.data)
        form.resetFields()
        setFileList([])
        setSpecs([{ id: Date.now(), name: '', daily_rent: 0, weekly_rent: 0, monthly_rent: 0, deposit: 0, stock: 0 }])
        setUploadStatus({ isUploading: false, progress: {}, failedFiles: [] })
      } else {
        throw new Error(result.message || '提交失败')
      }
    } catch (error) {
      if (error.errorFields) {
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
      open={open}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading || uploadStatus.isUploading}
      okButtonProps={{ 
        disabled: uploadStatus.failedFiles.length > 0,
        loading: uploadStatus.isUploading 
      }}
      width={800}
      styles={{ 
        body: {
          maxHeight: '70vh', 
          overflowY: 'auto', 
          overflowX: 'hidden',
          paddingLeft: '16px'
        }
      }}
    >
      <Form
        form={form}
        layout="vertical"
        style={{ marginRight: '16px' }}
        initialValues={{
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
          label="图片"
          extra="拖拽可调整图片顺序，建议尺寸 800x600"
        >
          <div>
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
                       <SortableImageItem 
                         key={file.uid} 
                         file={file} 
                         onRemove={removeImage}
                         uploadStatus={uploadStatus}
                         onRetry={retryUpload}
                       />
                     ))}
                  </div>
                </SortableContext>
              </DndContext>
            )}
          </div>
        </Form.Item>

        <Form.Item
          name="video"
          label="视频URL"
        >
          <Input placeholder="请输入视频URL（可选）" />
        </Form.Item>

        <Divider orientation="left">规格配置</Divider>
        
        <div className="mb-2 flex justify-end">
          <Button onClick={syncPricesToAll} disabled={specs.length <= 1} size="small">
            同步价格比例到所有规格
          </Button>
        </div>
        
        <div className="mb-4">
          {specs.map((spec, index) => (
            <Card key={spec.id} size="small" className="mb-3" style={{ border: '1px solid #f0f0f0', backgroundColor: '#fafafa' }}>
              {/* 第一行 */}
              <Row gutter={16}>
                <Col span={6}>
                  <Form.Item label="规格名称" required>
                    <Input
                      placeholder="规格名称"
                      value={spec.name}
                      onChange={(e) => updateSpec(spec.id, 'name', e.target.value)}
                    />
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item label="日租金 (¥)" required>
                    <InputNumber
                      placeholder="日租金"
                      value={spec.daily_rent}
                      onChange={(value) => updateSpec(spec.id, 'daily_rent', value)}
                      style={{ width: '100%' }}
                      min={0}
                    />
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item label="初始库存 (件)" required>
                    <InputNumber
                      placeholder="库存"
                      value={spec.stock}
                      onChange={(value) => updateSpec(spec.id, 'stock', value)}
                      style={{ width: '100%' }}
                      min={0}
                    />
                  </Form.Item>
                </Col>
                <Col span={6} style={{ display: 'flex', alignItems: 'flex-end' }}>
                  <Button
                    icon={<DeleteOutlined />}
                    onClick={() => removeSpec(spec.id)}
                    danger
                    disabled={specs.length <= 1}
                  />
                </Col>
              </Row>
              {/* 第二行 */}
              <Row gutter={16}>
                <Col span={6}>
                  <Form.Item label="周租金 (¥)">
                    <InputNumber
                      placeholder="周租金"
                      value={spec.weekly_rent}
                      onChange={(value) => updateSpec(spec.id, 'weekly_rent', value)}
                      style={{ width: '100%' }}
                      min={0}
                    />
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item label="月租金 (¥)">
                    <InputNumber
                      placeholder="月租金"
                      value={spec.monthly_rent}
                      onChange={(value) => updateSpec(spec.id, 'monthly_rent', value)}
                      style={{ width: '100%' }}
                      min={0}
                    />
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item label="押金 (¥)" required>
                    <InputNumber
                      placeholder="押金"
                      value={spec.deposit}
                      onChange={(value) => updateSpec(spec.id, 'deposit', value)}
                      style={{ width: '100%' }}
                      min={0}
                    />
                  </Form.Item>
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
      </Form>
    </Modal>
  )
}