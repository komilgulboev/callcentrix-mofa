import { defineConfig, transformWithEsbuild } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'
import autoprefixer from 'autoprefixer'

export default defineConfig(({ command }) => ({
  base: command === 'build' ? './' : '/',
  build: { outDir: 'build' },
  css: {
    postcss: { plugins: [autoprefixer()] },
  },
  plugins: [
    {
      name: 'treat-js-files-as-jsx',
      async transform(code, id) {
        if (!id.match(/src\/.*\.jsx?$/)) return null
        return transformWithEsbuild(code, id, { loader: 'jsx', jsx: 'automatic' })
      },
    },
    react(),
  ],
  optimizeDeps: {
    esbuildOptions: { loader: { '.js': 'jsx' } },
  },
  resolve: {
    alias: { src: path.resolve(__dirname, 'src') },
    extensions: ['.mjs', '.js', '.jsx', '.ts', '.tsx', '.json', '.scss'],
  },
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws':  'http://localhost:8080',
    },
  },
}))