// PhotoUploader — 后台照片上传控件（PC 共用，#2070）
// 抽自「维修师档案」既有实现，供创建师徒表单等复用（禁止复制第二份）。
// Props:
//   photoUrl: 现有照片 URL 或本地预览 URL（选择即预览；编辑场景由父组件传入现有图 → 回显）
//   onSelect(file): 选中文件回调（父组件负责本地预览 URL 与后续上传）
//   width/height: 预览尺寸（默认 72）
//   label: 按钮文案（默认「上传照片」）
import { Upload, Button, Image, Space } from 'antd'
import { UploadOutlined } from '@ant-design/icons'

export default function PhotoUploader({ photoUrl = '', onSelect, width = 72, height = 72, label = '上传照片' }) {
  return (
    <Space>
      {photoUrl ? (
        <Image src={photoUrl} width={width} height={height} style={{ objectFit: 'cover', borderRadius: 6 }} />
      ) : null}
      <Upload
        beforeUpload={(file) => {
          if (onSelect) onSelect(file)
          return false // 阻止自动上传：由父组件决定上传时机
        }}
        showUploadList={false}
        accept="image/*"
      >
        <Button icon={<UploadOutlined />}>{label}</Button>
      </Upload>
    </Space>
  )
}
