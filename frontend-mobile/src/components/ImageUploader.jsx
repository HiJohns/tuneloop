import { useState, useRef } from 'react'
import { Upload, X, Image } from 'lucide-react'

// Helper to get token from localStorage
const getToken = () => {
  return localStorage.getItem('token')
}

export default function ImageUploader({ onUpload, maxImages = 5 }) {
  const [images, setImages] = useState([])
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef(null)
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

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
      capturedAt: new Date().toISOString() // Store capture time per ui.md §3.19
    }))
    
    setImages(prev => [...prev, ...newImages])
  }

  const handleUpload = async () => {
    if (images.length === 0) return
    
    setUploading(true)
    const successfullyUploaded = [] // Track successful uploads
    
    try {
      for (const img of images) {
        const formData = new FormData()
        formData.append('file', img.file)
        
        // Add authentication header to prevent 401
        const token = getToken()
        const resp = await fetch(`${baseUrl}/upload`, {
          method: 'POST',
          headers: {
            ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
          },
          body: formData,
        })
        
        const result = await resp.json()
        
        if (result.code === 20000 && result.data?.url) {
          successfullyUploaded.push({
            url: result.data.url,
            filename: img.name,
            timestamp: img.capturedAt || new Date().toISOString()
          })
        } else {
          // If any upload fails, stop and keep remaining images for retry
          console.error(`Upload failed for ${img.name}:`, result.message)
          break
        }
      }
      
      setUploading(false)
      
      // Only clear images if all were uploaded successfully
      if (successfullyUploaded.length === images.length) {
        setImages([])
      } else {
        // Remove successfully uploaded images, keep failed ones for retry
        setImages(prev => prev.slice(successfullyUploaded.length))
      }
      
      if (onUpload && successfullyUploaded.length > 0) {
        onUpload(successfullyUploaded)
      }
    } catch (err) {
      setUploading(false)
      alert(`上传失败: ${err.message}`)
    }
  }

  const removeImage = (index) => {
    // Revoke object URL to prevent memory leak
    const imgToRemove = images[index]
    if (imgToRemove?.preview) {
      URL.revokeObjectURL(imgToRemove.preview)
    }
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