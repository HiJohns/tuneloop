import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  define: {
    'process.env': {},
    global: 'globalThis',
    ENABLE_INNER_HTML: false,
    ENABLE_ADJACENT_HTML: false,
    ENABLE_MUTATION_OBSERVER: false,
    ENABLE_CLONE_NODE: false,
    ENABLE_CONTAINS: false,
    ENABLE_SIZE_APIS: false,
    ENABLE_SIZE_APIS: false,
    ENABLE_TEMPLATE_CONTENT: false,
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
