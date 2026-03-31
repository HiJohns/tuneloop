import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: parseInt(process.env.VITE_DEV_PORT || '5554'),
    host: '0.0.0.0',
    allowedHosts: ['opencode.linxdeep.com', 'localhost'],
    proxy: {
      '/uploads': {
        target: 'http://localhost:5557',
        changeOrigin: true,
        secure: false
      },
      '/api': {
        target: 'http://localhost:5557',  // API 走 PC 后端
        changeOrigin: true,
        secure: false
      },
      '/auth': {
        target: 'http://localhost:5557',  // 认证走 PC 后端
        changeOrigin: true,
        secure: false
      }
    }
  }
})
