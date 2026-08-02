export default defineNuxtConfig({
  compatibilityDate: '2026-01-01',
  ssr: true,
  devtools: { enabled: true },

  css: ['~/assets/css/main.css'],

  app: {
    head: {
      title: 'RG Blitz',
      htmlAttrs: { lang: 'de' },
      viewport: 'width=device-width, initial-scale=1, viewport-fit=cover',
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=Instrument+Sans:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap',
        },
      ],
    },
  },

  runtimeConfig: {
    public: {
      // Base URL of the Go backend. Read server-side (SSR fetch) and exposed
      // to the browser (client-side polling after hydration) — see docs/06-api-spec.md.
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
    },
  },

  typescript: {
    strict: true,
    typeCheck: false,
  },
})
