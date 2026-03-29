import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5553,
    host: '0.0.0.0',
    allowedHosts: ['opencode.linxdeep.com', 'localhost']
  }
})
