import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // The public satellite origin is localhost:5174. Docker overrides this
    // to 5173 because compose maps host 5174 to the container UI port.
    port: Number(process.env.VITE_PORT || 5174),
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8787',
        changeOrigin: true,
        timeout: 0,
        proxyTimeout: 0,
      },
    },
  },
})
