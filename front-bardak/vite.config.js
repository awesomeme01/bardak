import {defineConfig} from 'vite';
import {svelte} from '@sveltejs/vite-plugin-svelte';

// Бэкенд слушает 8088: порт 8080 на машине занят Docker Desktop.
const BACKEND = process.env.BARDAK_BACKEND ?? 'http://localhost:8088';

export default defineConfig({
    // Svelte выбран на M3 по ADR-036, но переезжаем раньше: состояние стола приходит
    // по WS целыми снимками, и раскладывать его в DOM руками — главный источник
    // рассогласования картинки и правды сервера.
    plugins: [svelte()],
    server: {
        port: 5173,
        // Проксируем REST и WebSocket на бэкенд, чтобы фронт жил на одном origin
        // и не упирался в CORS. Этот порт уже прописан в bardak.ws.allowed-origins.
        proxy: {
            '/api': {
                target: BACKEND,
                changeOrigin: true,
            },
            '/ws': {
                target: BACKEND,
                ws: true,
                changeOrigin: true,
            },
            // Картинки карт и тем стола лежат на бэкенде, а не в репозитории фронта.
            '/assets': {
                target: BACKEND,
                changeOrigin: true,
            },
        },
    },
    build: {
        outDir: 'dist',
        sourcemap: true,
        // Бандл уезжает в dist/app/, а НЕ в dist/assets/: путь /assets/** занят
        // картинками карт, которые отдаёт бэкенд. Иначе в проде они перекрыли бы друг друга.
        assetsDir: 'app',
    },
});
