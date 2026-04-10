import { useState, useEffect, useCallback, useRef } from 'react'
import { Modal, Form, Input, Select, TreeSelect, Upload, Switch, message, Button, InputNumber, Row, Col, Divider, Space, Card, Progress } from 'antd'
import { UploadOutlined, PlusOutlined, DeleteOutlined, DragOutlined, ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined } from '@ant-design/icons'
import { arrayMove } from '@dnd-kit/sortable';
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { api, sitesApi } from '../../../services/api'

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

export default function InstrumentForm({ open: controlledOpen, onCancel, onSubmit, initialData = null, categories = [] }) {
  // If no open prop provided, assume page mode and auto-open modal
  const open = controlledOpen !== undefined ? controlledOpen : true
  // Page mode when onCancel is not provided (route usage), modal mode when onCancel is provided (List.jsx)
  const isPageMode = !onCancel
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [fileList, setFileList] = useState([])
  const [uploadStatus, setUploadStatus] = useState({
    isUploading: false,
    progress: {},
    failedFiles: []
  })
  const [categoryTree, setCategoryTree] = useState([])
  const [siteTree, setSiteTree] = useState([])
  const [properties, setProperties] = useState([])
  const [categoryLoading, setCategoryLoading] = useState(false)
  const [propertiesLoading, setPropertiesLoading] = useState(false)
  const [snChecking, setSnChecking] = useState(false)
  const [snDuplicate, setSnDuplicate] = useState(false)
  const snCheckTimer = useRef(null)
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
      } else {
        form.resetFields()
        setFileList([])
      }
    }
  }, [open, initialData])

  useEffect(() => {
    if (open) {
      fetchCategoryTree()
      fetchSiteTree()
      fetchProperties()
    }
  }, [open])

  // Initial data load on mount
  useEffect(() => {
    console.log('[DEBUG] Component mounted, open:', open, 'isPageMode:', isPageMode)
    if (open || isPageMode) {
      console.log('[DEBUG] Initial data load triggered')
      fetchCategoryTree()
      fetchSiteTree()
      fetchProperties()
    }
  }, [])

  useEffect(() => {
    console.log('[DEBUG] categoryTree state updated:', categoryTree)
  }, [categoryTree])

  useEffect(() => {
    console.log('[DEBUG] properties state updated:', properties)
  }, [properties])

  const fetchCategoryTree = async () => {
    try {
      console.log('[DEBUG] Fetching categories...')
      setCategoryLoading(true)
      const result = await api.get('/categories')
      console.log('[DEBUG] Categories API response:', result)
      
      // result is already the processed data array from api.js
      const data = Array.isArray(result) ? result : []
      console.log('[DEBUG] Categories data before mapping:', data)
      
      const tree = data.map(cat => {
        console.log('[DEBUG] Processing category:', cat)
        return {
          key: cat.id,
          title: cat.name,
          value: cat.id,
          children: cat.sub_categories?.map(child => ({
            key: child.id,
            title: child.name,
            value: child.id,
          }))
        }
      })
      console.log('[DEBUG] Final category tree:', tree)
      setCategoryTree(tree)
    } catch (err) {
      console.error('Failed to fetch categories:', err)
    } finally {
      setCategoryLoading(false)
    }
  }

  const fetchSiteTree = async () => {
    try {
      const result = await sitesApi.getTree()
      const data = result?.data?.list || []
      const tree = data.map(site => ({
        key: site.id,
        title: site.name,
        value: site.id,
        children: site.children?.map(child => ({
          key: child.id,
          title: child.name,
          value: child.id,
        }))
      }))
      setSiteTree(tree)
    } catch (err) {
      console.error('Failed to fetch sites:', err)
    }
  }

  const fetchProperties = async () => {
    try {
      console.log('[DEBUG] Fetching properties...')
      setPropertiesLoading(true)
      const result = await api.get('/properties')
      console.log('[DEBUG] Properties API response:', result)
      
      // Check if result is an array (direct response) or has data property (wrapped response)
      if (Array.isArray(result)) {
        console.log('[DEBUG] Result is an array, using directly:', result)
        setProperties(result)
        console.log('[DEBUG] Properties state updated, length:', result.length)
      } else if (result.code === 20000) {
        console.log('[DEBUG] Properties data:', result.data)
        setProperties(result.data || [])
        console.log('[DEBUG] Properties state updated, length:', result.data?.length || 0)
      }
    } catch (err) {
      console.error('Failed to fetch properties:', err)
    } finally {
      setPropertiesLoading(false)
    }
  }

  const handleSnChange = (value) => {
    if (snCheckTimer.current) {
      clearTimeout(snCheckTimer.current)
    }
    if (!value) {
      setSnDuplicate(false)
      return
    }
    setSnChecking(true)
    snCheckTimer.current = setTimeout(async () => {
      try {
        const result = await api.get(`/instruments/check?sn=${encodeURIComponent(value)}`)
        if (result.code === 20000 && result.data?.exists) {
          setSnDuplicate(true)
          const conflictInfo = result.data.info || {}
          message.warning(`识别码已存在: ${conflictInfo.site || ''} ${conflictInfo.category || ''}`)
        } else {
          setSnDuplicate(false)
        }
      } catch (err) {
        console.error('SN check failed:', err)
      } finally {
        setSnChecking(false)
      }
    }, 3000)
  }

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
    setUploadStatus(prev => ({
      ...prev,
      failedFiles: prev.failedFiles.filter(file => file.uid !== uid)
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
      
      
      // Prepare form data - remove name, level, specifications; add sn, site_id, properties
      const formData = {
        sn: values.sn,
        category_id: values.category_id,
        site_id: values.site_id,
        description: values.description,
        images: images,
        video: values.video || '',
        status: initialData ? (values.status || 'active') : 'active',
      }
      
      // Add dynamic properties
      const instrumentProps = {}
      properties.forEach(prop => {
        const propValue = values[`prop_${prop.id}`]
        if (propValue) {
          instrumentProps[prop.name] = propValue
        }
      })
      if (Object.keys(instrumentProps).length > 0) {
        formData.properties = instrumentProps
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

  // For modal mode (List.jsx), render as Modal. For page mode (routes), render as flat page
  const renderFormContent = () => (
    <Form
      form={form}
      layout="vertical"
      style={{ marginRight: '16px' }}
      initialValues={{
        status: 'active'
      }}
    >
      <Divider orientation="left">基本信息</Divider>
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="sn"
              label="识别码"
              rules={[
                { required: true, message: '请输入识别码' },
                { validator: () => snDuplicate ? Promise.reject('识别码已存在') : Promise.resolve() }
              ]}
              validateTrigger="onBlur"
            >
              <Input 
                placeholder="请输入唯一识别码" 
                suffix={snChecking && <LoadingOutlined />}
                onChange={e => handleSnChange(e.target.value)}
              />
            </Form.Item>
          </Col>
          
          <Col span={12}>
              <Form.Item
                name="category_id"
                label="乐器分类"
                rules={[{ required: true, message: '请选择分类' }]}
              >
                <TreeSelect
                  treeData={categoryTree}
                  placeholder="请选择分类"
                  treeDefaultExpandAll
                  fieldNames={{ title: 'title', value: 'value', children: 'children' }}
                  loading={categoryLoading}
                />
              </Form.Item>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="site_id"
              label="归属网点"
              rules={[{ required: true, message: '请选择网点' }]}
            >
              <TreeSelect
                treeData={siteTree}
                placeholder="请选择归属网点"
                treeDefaultExpandAll
              />
            </Form.Item>
          </Col>

        </Row>
        {console.log('[DEBUG RENDER Form] Properties at render time:', properties, 'Length:', properties?.length)}
        {properties.length > 0 && (
          <>
            <Divider orientation="left">动态属性</Divider>
            <Row gutter={16}>
              {properties.map(prop => (
                <Col span={12} key={prop.id}>
                  <Form.Item
                    name={`prop_${prop.id}`}
                    label={prop.name}
                    required={prop.is_required}
                  >
                    <Select 
                      placeholder={`请选择或输入${prop.name}`}
                      allowClear
                      mode="combobox"
                      onChange={(value, option) => {
                        if (option?.children && !option.key) {
                          console.log(`New value "${value}" for ${prop.name} - should be marked as pending`)
                        }
                      }}
                    >
                      {prop.options?.map(opt => (
                        <Option key={opt.value} value={opt.value}>{opt.value}</Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
              ))}
            </Row>
          </>
        )}
        {properties.length === 0 && propertiesLoading && (
          <div>加载动态属性中...</div>
        )}
        {properties.length === 0 && !propertiesLoading && (
          <>
            <Divider orientation="left">动态属性</Divider>
            <div style={{ padding: '16px', textAlign: 'center', color: '#999' }}>
              <p>暂无动态属性配置</p>
              <Button 
                type="link" 
                onClick={() => window.location.href = '/instruments/properties'}
              >
                前往属性管理页面 →
              </Button>
            </div>
          </>
        )}

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
      </Form>
  )

  return isPageMode ? (
    <div style={{ padding: '24px' }}>
      <Card title={title}>
        {renderFormContent()}
      </Card>
    </div>
  ) : (
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
      {renderFormContent()}
    </Modal>
  )
}