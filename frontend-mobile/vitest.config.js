import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

const root = process.cwd()

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@tarojs/components': path.resolve(root, 'src/taro-shim.js'),
      '@tarojs/taro': path.resolve(root, 'scripts/page-smoke/taro-stub.js'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['scripts/page-smoke/setup.js'],
    include: ['scripts/page-smoke/**/*.test.jsx'],
    css: false,
    testTimeout: 20000,
    hookTimeout: 20000,
  },
})
