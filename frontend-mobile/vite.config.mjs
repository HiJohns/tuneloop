import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  define: {
    'process.env': {},
    global: 'globalThis',
  },
  base: '/',
  resolve: {
    alias: {
      '@tarojs/components': path.resolve(__dirname, 'src/taro-shim.js'),
    },
  },
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5553,
    host: '0.0.0.0',
    allowedHosts: ['opencode.linxdeep.com', 'localhost'],
    proxy: {
      '/api': {
        target: 'http://localhost:5556',
        changeOrigin: true,
        secure: false,
      },
      '/auth': {
        target: 'http://localhost:5556',
        changeOrigin: true,
        secure: false,
      },
      '/uploads': {
        target: 'http://localhost:5556',
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
