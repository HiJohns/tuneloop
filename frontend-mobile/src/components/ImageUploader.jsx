import { useState, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Image, Text } from '@tarojs/components'
import { dialog, env } from '../platform'

// #2122 修复：原实现用原生 HTML（div/img/button）→ weapp 整块不渲染、无添加按钮
//（#归还页 同类坑）。改为 Taro 组件（View/Image/Text），跨端可用；接口不变
//（onChange(files)，file = weapp 临时路径 / H5 File）。
export default function ImageUploader({ onChange, maxImages = 5 }) {
  const [images, setImages] = useState([])
  const fileInputRef = useRef(null)

  const emit = (updated) => {
    setImages(updated)
    if (onChange) onChange(updated.map(i => i.file))
  }

  const handleFileSelect = async (e) => {
    const files = Array.from(e.target.files || [])
    if (images.length + files.length > maxImages) {
      dialog.alert(`最多上传 ${maxImages} 张图片`)
      return
    }
    const newImages = files.map(file => ({
      file,
      preview: URL.createObjectURL(file),
      name: file.name,
      capturedAt: new Date().toISOString(),
    }))
    emit([...images, ...newImages])
  }

  const handleWeappPick = async () => {
    try {
      const res = await Taro.chooseImage({ count: maxImages - images.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      const paths = res.tempFilePaths || []
      if (images.length + paths.length > maxImages) {
        Taro.showToast({ title: `最多上传 ${maxImages} 张图片`, icon: 'none' })
        return
      }
      const newImages = paths.map((path, i) => ({
        file: path,
        preview: path,
        name: `weapp_${Date.now()}_${i}.jpg`,
        capturedAt: new Date().toISOString(),
      }))
      emit([...images, ...newImages])
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const removeImage = (index) => {
    const imgToRemove = images[index]
    if (imgToRemove?.preview && !env.isMiniProgram) {
      URL.revokeObjectURL(imgToRemove.preview)
    }
    emit(images.filter((_, i) => i !== index))
  }

  const onPickClick = () => {
    if (env.isMiniProgram) {
      handleWeappPick()
    } else {
      fileInputRef.current?.click()
    }
  }

  return (
    <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <View style={{ display: 'flex', flexDirection: 'row', flexWrap: 'wrap', gap: 8 }}>
        {images.map((img, index) => (
          <View key={index} style={{ position: 'relative', width: 80, height: 80 }}>
            <Image src={img.preview} mode="aspectFill"
              style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5' }} />
            <View
              onClick={() => removeImage(index)}
              style={{ position: 'absolute', top: -6, right: -6, width: 20, height: 20, borderRadius: '50%', backgroundColor: '#ef4444', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
            >
              <Text style={{ color: '#FFFFFF', fontSize: 12, lineHeight: '20px' }}>×</Text>
            </View>
          </View>
        ))}

        {images.length < maxImages && (
          <View
            onClick={onPickClick}
            style={{ width: 80, height: 80, borderWidth: 2, borderStyle: 'dashed', borderColor: '#d4d4d8', borderRadius: 8, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}
          >
            <Text style={{ fontSize: 22, color: '#a1a1aa', lineHeight: '26px' }}>＋</Text>
            <Text style={{ fontSize: 11, color: '#a1a1aa', marginTop: 2 }}>添加照片</Text>
          </View>
        )}
      </View>

      {!env.isMiniProgram && (
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          onChange={handleFileSelect}
          className="hidden"
        />
      )}

      <Text style={{ fontSize: 11, color: '#a1a1aa' }}>
        最多上传 {maxImages} 张图片，支持 JPG、PNG 格式
      </Text>
    </View>
  )
}
