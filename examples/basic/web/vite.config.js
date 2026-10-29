import { defineConfig } from 'vite'

// The minimal frontend of the goyii basic example. The build writes the
// manifest next to the hashed assets; the Go application reads it at startup
// (vite.Vite, Manifest "web/dist/.vite/manifest.json") and turns the
// "src/main.ts" entry into <link>/<script> tags for the page head.
//
//   npm install     # once, build-time only — the Go server never needs Node
//   npm run build   # writes web/dist (manifest + hashed assets), committed
//   npm run dev     # dev server on :5173 with HMR; the Go app proxies /assets
export default defineConfig({
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
    manifest: true,
    rollupOptions: {
      input: 'src/main.ts',
    },
  },
})
