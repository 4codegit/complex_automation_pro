import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      // Same-origin API + WebSocket in dev: everything goes through this
      // proxy, exactly like the embedded bundle behind capd.
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
