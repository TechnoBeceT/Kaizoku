// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  ssr: false,
  nitro: { preset: 'bun' },
  compatibilityDate: '2025-07-15',
  devtools: { enabled: true },

  modules: [
    '@nuxt/ui',
    '@nuxt/eslint',
    '@nuxt/image',
  ],

  css: ['~/assets/css/main.css', 'flag-icons/css/flag-icons.min.css'],

  runtimeConfig: {
    public: {
      backendUrl: '',
    },
  },

  vite: {
    server: {
      proxy: {
        '/api': { target: 'http://localhost:9833', changeOrigin: true },
        '/progress': { target: 'http://localhost:9833', ws: true },
      },
    },
  },
})
