import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: parseInt(process.env.VITE_DEV_PORT || '5554'),
    host: '0.0.0.0',
    allowedHosts: ['opencode.linxdeep.com', 'localhost'],
    proxy: {
      '/api': {
        target: 'http://localhost:5556',
        changeOrigin: true,
        secure: false
      }
    }
  }
})
