import { useState, useRef } from 'react'
import Taro from '@tarojs/taro'
import { Upload, X } from 'lucide-react'
import { env } from '../platform'

export default function ImageUploader({ onChange, maxImages = 5 }) {
  const [images, setImages] = useState([])
  const fileInputRef = useRef(null)

  const handleFileSelect = async (e) => {
    const files = Array.from(e.target.files)
    if (images.length + files.length > maxImages) {
      alert(`最多上传 ${maxImages} 张图片`)
      return
    }

    const newImages = files.map(file => ({
      file,
      preview: URL.createObjectURL(file),
      name: file.name,
      capturedAt: new Date().toISOString()
    }))
    
    const updated = [...images, ...newImages]
    setImages(updated)
    if (onChange) onChange(updated.map(i => i.file))
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
        capturedAt: new Date().toISOString()
      }))
      const updated = [...images, ...newImages]
      setImages(updated)
      if (onChange) onChange(updated.map(i => i.file))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const removeImage = (index) => {
    const imgToRemove = images[index]
    if (imgToRemove?.preview && !env.isMiniProgram) {
      URL.revokeObjectURL(imgToRemove.preview)
    }
    const updated = images.filter((_, i) => i !== index)
    setImages(updated)
    if (onChange) onChange(updated.map(i => i.file))
  }

  const onPickClick = () => {
    if (env.isMiniProgram) {
      handleWeappPick()
    } else {
      fileInputRef.current?.click()
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {images.map((img, index) => (
          <div key={index} className="relative">
            <img 
              src={img.preview} 
              alt={img.name}
              className="w-20 h-20 object-cover rounded-lg"
            />
            <button
              onClick={() => removeImage(index)}
              className="absolute -top-2 -right-2 bg-red-500 text-white rounded-full p-1"
            >
              <X size={12} />
            </button>
          </div>
        ))}
        
        {images.length < maxImages && (
          <button
            onClick={onPickClick}
            className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center text-gray-400 hover:border-orange-500 hover:text-orange-500"
          >
            <Upload size={20} />
            <span className="text-xs mt-1">添加照片</span>
          </button>
        )}
      </div>

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

      <p className="text-gray-400 text-xs">
        最多上传 {maxImages} 张图片，支持 JPG、PNG 格式
      </p>
    </div>
  )
}
