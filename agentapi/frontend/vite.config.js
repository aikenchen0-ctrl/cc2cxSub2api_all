import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';
import checker from 'vite-plugin-checker';
import { resolve } from 'path';
export default defineConfig(function (_a) {
    var mode = _a.mode;
    var env = loadEnv(mode, process.cwd(), '');
    var backend = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080';
    var port = Number(env.VITE_DEV_PORT || 3000);
    return {
        plugins: [vue(), checker({ vueTsc: true })],
        resolve: {
            alias: {
                '@': resolve(__dirname, 'src'),
            },
        },
        build: {
            outDir: '../backend/web',
            emptyOutDir: true,
        },
        server: {
            host: '0.0.0.0',
            port: port,
            proxy: {
                '/api': { target: backend, changeOrigin: true },
                '/v1': { target: backend, changeOrigin: true },
                '/healthz': { target: backend, changeOrigin: true },
            },
        },
    };
});
