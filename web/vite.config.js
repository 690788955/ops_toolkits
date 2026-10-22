import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/',
  plugins: [react()],
  build: {
    outDir: '../internal/server/web',
    emptyOutDir: true
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  test: {
    // 各测试文件用 `// @vitest-environment jsdom` 自行声明环境；
    // 这里只负责注入全局 setup，消除 React act() 告警噪声。
    setupFiles: ['./src/test/setup.js']
  }
})
