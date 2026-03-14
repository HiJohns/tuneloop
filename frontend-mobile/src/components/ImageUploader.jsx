import { useState, useRef } from 'react'
import { Upload, X, Image } from 'lucide-react'

export default function ImageUploader({ onUpload, maxImages = 5 }) {
  const [images, setImages] = useState([])
  const [uploading, setUploading] = useState(false)
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
      name: file.name
    }))
    
    setImages(prev => [...prev, ...newImages])
  }

  const handleUpload = async () => {
    if (images.length === 0) return
    
    setUploading(true)
    
    // Simulate API call
    setTimeout(() => {
      const mockUrl = "https://dummy.tuneloop.com/uploads/mock-image.jpg"
      setUploading(false)
      if (onUpload) {
        onUpload(mockUrl)
      }
      alert("上传成功！")
    }, 1500)
  }

  const removeImage = (index) => {
    setImages(prev => prev.filter((_, i) => i !== index))
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
            onClick={() => fileInputRef.current?.click()}
            className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center text-gray-400 hover:border-orange-500 hover:text-orange-500"
          >
            <Upload size={20} />
            <span className="text-xs mt-1">上传</span>
          </button>
        )}
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        multiple
        onChange={handleFileSelect}
        className="hidden"
      />

      {images.length > 0 && (
        <button
          onClick={handleUpload}
          disabled={uploading}
          className="w-full py-2.5 bg-orange-500 text-white rounded-lg font-medium disabled:opacity-50"
        >
          {uploading ? '上传中...' : '确认上传'}
        </button>
      )}

      <p className="text-gray-400 text-xs">
        最多上传 {maxImages} 张图片，支持 JPG、PNG 格式
      </p>
    </div>
  )
}