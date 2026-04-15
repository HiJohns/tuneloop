import { useEffect, useState } from 'react'
import { Card, Select, List, Button, Modal, Form, Input, Switch, message, Spin, Empty, Space, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, AppstoreOutlined, MenuOutlined } from '@ant-design/icons'
import { DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { api, categoriesApi } from '../../../services/api'

const { Option } = Select

function SortableItem({ category, onEdit, onDelete }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: category.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: 'grab',
  }

  return (
    <div ref={setNodeRef} style={style} className="flex items-center justify-between p-3 bg-white border-b hover:bg-gray-50">
      <div className="flex items-center gap-2 flex-1">
        <MenuOutlined {...attributes} {...listeners} className="cursor-grab text-gray-400" />
        <span className="font-medium">{category.name}</span>
        {category.icon && <span>{category.icon}</span>}
        {!category.visible && <span className="text-xs text-gray-400">(hidden)</span>}
      </div>
      <Space size="small">
        <Button size="small" icon={<EditOutlined />} onClick={() => onEdit(category)} />
        <Popconfirm
          title="Delete this category?"
          onConfirm={() => onDelete(category.id)}
          okText="Yes"
          cancelText="No"
        >
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      </Space>
    </div>
  )
}

export default function CategoryList() {
  const [level1Categories, setLevel1Categories] = useState([])
  const [selectedParentId, setSelectedParentId] = useState(null)
  const [subCategories, setSubCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [savingSort, setSavingSort] = useState(false)
  const [editingCategory, setEditingCategory] = useState(null)
  const [modalVisible, setModalVisible] = useState(false)
  const [formMode, setFormMode] = useState('create')
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  useEffect(() => {
    fetchCategories()
  }, [])

  useEffect(() => {
    if (selectedParentId) {
      const parent = level1Categories.find(c => c.id === selectedParentId)
      if (parent && parent.sub_categories) {
        const sorted = [...parent.sub_categories].sort((a, b) => (a.sort || 0) - (b.sort || 0))
        setSubCategories(sorted)
      } else {
        setSubCategories([])
      }
    } else {
      setSubCategories([])
    }
  }, [selectedParentId, level1Categories])

  const fetchCategories = async () => {
    try {
      setLoading(true)
      const result = await api.get('/categories')
      const data = result?.data?.list || []
      setLevel1Categories(data)
      if (data.length > 0 && !selectedParentId) {
        setSelectedParentId(data[0].id)
      }
    } catch (err) {
      message.error('Failed to load categories: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDragEnd = async (event) => {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const oldIndex = subCategories.findIndex(c => c.id === active.id)
    const newIndex = subCategories.findIndex(c => c.id === over.id)

    const newSubCategories = [...subCategories]
    const [removed] = newSubCategories.splice(oldIndex, 1)
    newSubCategories.splice(newIndex, 0, removed)
    setSubCategories(newSubCategories)

    const sortUpdates = newSubCategories.map((cat, index) => ({
      id: cat.id,
      sort: index + 1,
    }))

    setSavingSort(true)
    try {
      await api.put('/categories/sort', { items: sortUpdates })
      message.success('Sort order updated')
      await fetchCategories()
    } catch (err) {
      message.error('Failed to update sort: ' + err.message)
      const parent = level1Categories.find(c => c.id === selectedParentId)
      if (parent && parent.sub_categories) {
        setSubCategories([...parent.sub_categories].sort((a, b) => (a.sort || 0) - (b.sort || 0)))
      }
    } finally {
      setSavingSort(false)
    }
  }

  const handleCreateTopLevel = () => {
    setEditingCategory(null)
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ visible: true })
    setModalVisible(true)
  }

  const handleCreateSubCategory = () => {
    if (!selectedParentId) {
      message.warning('Please select a parent category first')
      return
    }
    setEditingCategory({ parent_id: selectedParentId })
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ visible: true })
    setModalVisible(true)
  }

  const handleEdit = (category) => {
    setEditingCategory({ ...category })
    setFormMode('edit')
    form.resetFields()
    form.setFieldsValue({
      ...category,
      visible: category.visible !== false,
    })
    setModalVisible(true)
  }

  const handleDelete = async (categoryId) => {
    try {
      await categoriesApi.delete(categoryId)
      message.success('Deleted successfully')
      await fetchCategories()
    } catch (err) {
      message.error('Failed to delete: ' + err.message)
    }
  }

  const handleFormSubmit = async () => {
    setSaving(true)
    try {
      const values = await form.validateFields()

      let finalParentId = null
      if (formMode === 'create' && editingCategory?.parent_id) {
        finalParentId = editingCategory.parent_id
      } else if (formMode === 'edit') {
        finalParentId = editingCategory.parent_id || null
      }

      const formData = {
        name: values.name,
        icon: values.icon || '',
        visible: values.visible !== false,
        parent_id: finalParentId,
      }

      if (formMode === 'edit' && editingCategory?.id) {
        await categoriesApi.update(editingCategory.id, formData)
        message.success('Updated successfully')
      } else {
        await categoriesApi.create(formData)
        message.success('Created successfully')
      }

      setModalVisible(false)
      await fetchCategories()
    } catch (error) {
      if (!error.errorFields) {
        message.error(error.message || 'Failed to submit')
      }
    } finally {
      setSaving(false)
    }
  }

  const getParentCategoryName = () => {
    const parent = level1Categories.find(c => c.id === selectedParentId)
    return parent?.name || ''
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="p-4">
      <div className="flex gap-4">
        <Card title="Category Management" className="w-1/3" extra={<Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>Create Top Level</Button>}>
          {level1Categories.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-64 text-gray-400">
              <AppstoreOutlined style={{ fontSize: 64, marginBottom: 16 }} />
              <p className="text-lg mb-4">No categories</p>
              <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>Create Top Level Category</Button>
            </div>
          ) : (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Select Parent Category (Level 1)</label>
                <Select
                  value={selectedParentId}
                  onChange={setSelectedParentId}
                  className="w-full"
                  placeholder="Select a parent category"
                >
                  {level1Categories.map(cat => (
                    <Option key={cat.id} value={cat.id}>{cat.icon} {cat.name}</Option>
                  ))}
                </Select>
              </div>
              <Button block icon={<PlusOutlined />} onClick={handleCreateSubCategory} disabled={!selectedParentId}>Create Sub Category</Button>
            </div>
          )}
        </Card>

        <Card className="w-2/3" title={selectedParentId ? `Sub Categories of "${getParentCategoryName()}"` : 'Sub Categories'} extra={savingSort && <Spin size="small" />}>
          {!selectedParentId ? (
            <Empty description="Please select a parent category from the left" />
          ) : subCategories.length === 0 ? (
            <Empty description="No sub categories, click 'Create Sub Category' to add one" />
          ) : (
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
              <SortableContext items={subCategories.map(c => c.id)} strategy={verticalListSortingStrategy}>
                <div className="border rounded">
                  {subCategories.map(category => (
                    <SortableItem key={category.id} category={category} onEdit={handleEdit} onDelete={handleDelete} />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          )}
        </Card>
      </div>

      <Modal
        title={formMode === 'edit' ? 'Edit Category' : 'Create Category'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleFormSubmit}
        confirmLoading={saving}
        okText="Submit"
        cancelText="Cancel"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Category Name" rules={[{ required: true, message: 'Please enter category name' }, { min: 2, message: 'At least 2 characters' }, { max: 50, message: 'Max 50 characters' }]}>
            <Input placeholder="Enter category name" />
          </Form.Item>

          {formMode === 'create' && editingCategory?.parent_id && (
            <Form.Item label="Parent Category">
              <div className="p-2 bg-gray-50 rounded">{getParentCategoryName()}</div>
            </Form.Item>
          )}

          <Form.Item name="icon" label="Icon" extra="Enter emoji or icon URL">
            <Input placeholder="e.g. 🎹" />
          </Form.Item>

          {formMode === 'edit' && (
            <Form.Item name="visible" label="Visibility" valuePropName="checked">
              <Switch checkedChildren="Visible" unCheckedChildren="Hidden" />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  )
}
