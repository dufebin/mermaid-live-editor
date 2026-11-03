import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { FileSystemIconLoader } from 'unplugin-icons/loaders';
import Icons from 'unplugin-icons/vite';
import { defineConfig, loadEnv } from 'vite';
import devtoolsJson from 'vite-plugin-devtools-json';

/**
 * HMR creates state inconsistencies, so we always reload the page.
 * @type {import('vite').PluginOption} PluginOption
 */
const alwaysFullReload = {
  name: 'always-full-reload',
  handleHotUpdate({ server }) {
    server.ws.send({ type: 'full-reload' });
    return [];
  }
};

export default defineConfig(({ mode }) => {
  const serverEnv = loadEnv(mode, process.cwd(), '');
  const aiProxy = serverEnv.AI_PROXY_TARGET
    ? {
        '/ai': {
          changeOrigin: true,
          headers: { Authorization: `Bearer ${serverEnv.AI_PROXY_KEY ?? ''}` },
          rewrite: (path) => path.replace(/^\/ai/, ''),
          target: serverEnv.AI_PROXY_TARGET
        }
      }
    : undefined;

  const apiProxy = {
    '/api': {
      changeOrigin: true,
      target: serverEnv.API_PROXY_TARGET ?? 'http://127.0.0.1:18790'
    }
  };

  return {
    plugins: [
      tailwindcss(),
      sveltekit(),
      Icons({
        compiler: 'svelte',
        customCollections: {
          custom: FileSystemIconLoader('./static/icons')
        }
      }),
      alwaysFullReload,
      devtoolsJson()
    ],
    envPrefix: 'MERMAID_',
    server: { host: true, port: 3000, proxy: { ...(aiProxy ?? {}), ...apiProxy } },
    preview: { host: true, port: 3000, proxy: { ...(aiProxy ?? {}), ...apiProxy } },
    // Vitest otherwise resolves Svelte's server build, where $effect is a no-op.
    resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
    test: {
      environment: 'jsdom',
      // in-source testing
      includeSource: ['src/**/*.{js,ts,svelte}'],
      // Ignore E2E tests
      exclude: [
        'tests/**/*',
        '**/node_modules/**',
        '**/dist/**',
        '**/.{idea,git,cache,output,temp}/**',
        '**/{karma,rollup,webpack,vite,vitest,jest,ava,babel,nyc,cypress,tsup,build,eslint,prettier}.config.*'
      ],
      setupFiles: ['./src/tests/setup.ts'],
      coverage: {
        exclude: ['src/mocks', '.svelte-kit', 'src/**/*.test.ts'],
        reporter: ['text', 'json', 'html', 'lcov']
      }
    }
  };
});
