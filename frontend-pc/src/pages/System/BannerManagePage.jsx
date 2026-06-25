import { useState, useEffect } from 'react';
import { Table, Card, Button, Space, Modal, Form, Input, InputNumber, Select, Upload, Image, Popconfirm, message } from 'antd';
import { PlusOutlined, UploadOutlined, EditOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { bannerApi } from '../../services/api';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api';

const statusOptions = [
  { value: 'active', label: '启用' },
  { value: 'inactive', label: '禁用' },
];

export default function BannerManagePage() {
  const [banners, setBanners] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingBanner, setEditingBanner] = useState(null);
  const [form] = Form.useForm();

  const fetchBanners = async () => {
    setLoading(true);
    try {
      const res = await bannerApi.list();
      if (res.code === 20000) {
        setBanners(res.data?.list || []);
      } else {
        message.error('获取轮播图列表失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
    setLoading(false);
  };

  useEffect(() => { fetchBanners(); }, []);

  const handleCreate = () => {
    setEditingBanner(null);
    form.resetFields();
    form.setFieldsValue({ sort_order: 0, status: 'active' });
    setModalOpen(true);
  };

  const handleEdit = (record) => {
    setEditingBanner(record);
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const handleDelete = async (id) => {
    try {
      const res = await bannerApi.delete(id);
      if (res.code === 20000) {
        message.success('删除成功');
        fetchBanners();
      } else {
        message.error('删除失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
  };

  const handleMoveUp = (index) => {
    if (index === 0) return
    const sorted = [...banners]
    const prev = sorted[index - 1]
    const curr = sorted[index]
    const temp = prev.sort_order
    prev.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      bannerApi.update(prev.id, { sort_order: prev.sort_order }),
      bannerApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchBanners()).catch(() => message.error('排序更新失败'))
  }

  const handleMoveDown = (index) => {
    if (index === banners.length - 1) return
    const sorted = [...banners]
    const next = sorted[index + 1]
    const curr = sorted[index]
    const temp = next.sort_order
    next.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      bannerApi.update(next.id, { sort_order: next.sort_order }),
      bannerApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchBanners()).catch(() => message.error('排序更新失败'))
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingBanner) {
        const res = await bannerApi.update(editingBanner.id, values);
        if (res.code === 20000) {
          message.success('更新成功');
        } else {
          message.error('更新失败');
          return;
        }
      } else {
        const res = await bannerApi.create(values);
        if (res.code === 20000 || res.code === 20100) {
          message.success('创建成功');
        } else {
          message.error('创建失败');
          return;
        }
      }
      setModalOpen(false);
      fetchBanners();
    } catch (e) {
      if (e.errorFields) return;
      message.error('操作失败');
    }
  };

  const columns = [
    {
      title: '缩略图',
      dataIndex: 'image_url',
      key: 'image_url',
      width: 100,
      render: (url) => (
        url ? <Image src={url} width={60} height={60} style={{ objectFit: 'cover', borderRadius: 4 }} /> : '-'
      ),
    },
    { title: '标题', dataIndex: 'title', key: 'title' },
    { title: '跳转链接', dataIndex: 'link_url', key: 'link_url', ellipsis: true },
    { title: '排序', dataIndex: 'sort_order', key: 'sort_order', width: 80 },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => (
        <span style={{ color: status === 'active' ? '#52c41a' : '#ff4d4f' }}>
          {status === 'active' ? '启用' : '禁用'}
        </span>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_, record, index) => (
        <Space>
          <Button type="link" size="small" icon={<ArrowUpOutlined />} disabled={index === 0} onClick={() => handleMoveUp(index)} />
          <Button type="link" size="small" icon={<ArrowDownOutlined />} disabled={index === banners.length - 1} onClick={() => handleMoveDown(index)} />
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)}>编辑</Button>
          <Popconfirm title="确定删除该轮播图？" onConfirm={() => handleDelete(record.id)} okText="确定" cancelText="取消">
            <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card title="轮播图管理" extra={<Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>新增轮播图</Button>}>
      <Table
        dataSource={banners}
        columns={columns}
        rowKey="id"
        loading={loading}
        pagination={false}
      />
      <Modal
        title={editingBanner ? '编辑轮播图' : '新增轮播图'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        okText="保存"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item label="图片" extra="支持上传或输入图片URL">
            <Upload
              action={`${API_BASE_URL}/upload`}
              name="file"
              showUploadList={false}
              onChange={(info) => {
                if (info.file.status === 'done') {
                  const resp = info.file.response;
                  if (resp?.data?.url) {
                    form.setFieldsValue({ image_url: resp.data.url });
                    message.success('上传成功');
                  }
                } else if (info.file.status === 'error') {
                  message.error('上传失败');
                }
              }}
            >
              <Button icon={<UploadOutlined />}>上传图片</Button>
            </Upload>
          </Form.Item>
          <Form.Item name="image_url" label="图片URL" rules={[{ required: true, message: '请上传或输入图片URL' }]}>
            <Input placeholder="图片URL" />
          </Form.Item>
          <Form.Item name="link_url" label="跳转链接">
            <Input placeholder="可选：点击轮播图跳转的链接" />
          </Form.Item>
          <Form.Item name="title" label="标题">
            <Input placeholder="可选：轮播图标题" />
          </Form.Item>
          <Form.Item name="sort_order" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select options={statusOptions} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
