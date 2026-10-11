import { defineConfig } from 'vite';

// Vite 配置：outDir 与 .gitignore 中的 dist/ 保持一致。
// 后端已开启 CORS，开发时前端可直接访问 http://localhost:6872；
// 如需规避跨域或统一同源路径，可启用下方代理并改用相对路径 /api。
export default defineConfig({
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    // proxy: {
    //   '/api': 'http://localhost:6872',
    // },
  },
});
