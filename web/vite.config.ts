import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// dev 免 CORS:前端 5173,代理 /api + /healthz → os server(:8787)。
// 生产 = go:embed 单二进制同源,无需代理。
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8787', changeOrigin: true },
      '/healthz': { target: 'http://127.0.0.1:8787', changeOrigin: true },
    },
  },
});
