import { fileURLToPath } from 'node:url';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig, type Connect, type Plugin } from 'vite';

function routePages(preview = false): Connect.NextHandleFunction {
  return (request, _response, next) => {
    const url = new URL(request.url ?? '/', 'http://localhost');
    const wantsJSON = url.searchParams.get('format') === 'json'
      || request.headers.accept?.includes('application/json');
    if (request.method === 'GET' || request.method === 'HEAD') {
      if (preview && /^\/(?:_internal\/)?ui\//.test(url.pathname)) {
        request.url = request.url?.replace(/^\/(?:_internal\/)?ui\//, '/');
      } else if (!wantsJSON) {
        if (url.pathname === '/') {
          request.url = `/client/index.html${url.search}`;
        } else if (/^\/_internal\/(?:overview\/?|tun(?:\/.*)?)?$/.test(url.pathname)) {
          request.url = `/operator/index.html${url.search}`;
        }
      }
    }
    next();
  };
}

function pages(): Plugin {
  return {
    name: 'specter-pages',
    configureServer(server) {
      server.middlewares.use(routePages());
    },
    configurePreviewServer(server) {
      server.middlewares.use(routePages(true));
    },
  };
}

export default defineConfig({
  appType: 'mpa',
  base: './',
  plugins: [pages(), svelte(), tailwindcss()],
  experimental: {
    renderBuiltUrl(filename, { hostId, hostType }) {
      if (hostType === 'html') {
        return `${hostId === 'operator/index.html' ? '/_internal/ui/' : '/ui/'}${filename}`;
      }
      return { relative: true };
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    modulePreload: { polyfill: false },
    rolldownOptions: {
      input: {
        client: fileURLToPath(new URL('./client/index.html', import.meta.url)),
        operator: fileURLToPath(new URL('./operator/index.html', import.meta.url)),
      },
    },
  },
  server: {
    proxy: {
      '/api': process.env.SPECTER_CLIENT_ORIGIN ?? 'http://127.0.0.1:1180',
      '/_internal': process.env.SPECTER_OPERATOR_ORIGIN ?? 'http://127.0.0.1:11180',
    },
  },
});
