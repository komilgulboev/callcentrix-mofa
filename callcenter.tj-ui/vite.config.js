import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'
import { execSync } from 'node:child_process'

function gitCommit() {
  try {
    return execSync('git rev-parse --short HEAD').toString().trim()
  } catch {
    return 'unknown'
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  define: {
    __BUILD_TIME__:   JSON.stringify(new Date().toISOString()),
    __BUILD_COMMIT__: JSON.stringify(gitCommit()),
  },
  plugins: [react({ include: ['**/*.{jsx,tsx,js,ts}'] })],
  resolve: {
    alias: {
      src: fileURLToPath(new URL('./src', import.meta.url)),
    },
    extensions: ['.mjs', '.js', '.ts', '.jsx', '.tsx', '.json', '.scss'],
  },
  server: {
    port: 3000,
    proxy: {
      // Forward API calls to backend in dev
      // '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
