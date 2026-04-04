import { useState, useCallback } from 'react'

/**
 * useUpload - Shared hook for managing file uploads with lock logic
 * 
 * @returns {Object} Upload state and control functions
 */
export function useUpload() {
  const [uploadStatus, setUploadStatus] = useState({
    isUploading: false,
    progress: {},
    failedFiles: []
  })

  const updateProgress = useCallback((uid, percent) => {
    setUploadStatus(prev => ({
      ...prev,
      progress: {
        ...prev.progress,
        [uid]: { percent, status: percent < 100 ? 'uploading' : 'done' }
      }
    }))
  }, [])

  const markAsFailed = useCallback((uid) => {
    setUploadStatus(prev => ({
      ...prev,
      failedFiles: [...prev.failedFiles, uid]
    }))
  }, [])

  const removeFailed = useCallback((uid) => {
    setUploadStatus(prev => ({
      ...prev,
      failedFiles: prev.failedFiles.filter(id => id !== uid)
    }))
  }, [])

  const startUpload = useCallback(() => {
    setUploadStatus(prev => ({
      ...prev,
      isUploading: true
    }))
  }, [])

  const finishUpload = useCallback(() => {
    setUploadStatus(prev => ({
      ...prev,
      isUploading: false
    }))
  }, [])

  const resetUpload = useCallback(() => {
    setUploadStatus({
      isUploading: false,
      progress: {},
      failedFiles: []
    })
  }, [])

  return {
    uploadStatus,
    updateProgress,
    markAsFailed,
    removeFailed,
    startUpload,
    finishUpload,
    resetUpload
  }
}