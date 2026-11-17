import { resolve } from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';

const frontend = resolve(import.meta.dirname, '..');

// The real app, with the two modules that talk to the Wails backend swapped for the fakes here.
const fakes = new Map([
  [resolve(frontend, 'src/api/index.ts'), resolve(import.meta.dirname, 'api.ts')],
  [resolve(frontend, 'src/api/events.ts'), resolve(import.meta.dirname, 'events.ts')],
]);

const fakeBackend: Plugin = {
  name: 'portunus-fake-backend',
  enforce: 'pre',
  async resolveId(source, importer, options) {
    const resolved = await this.resolve(source, importer, { ...options, skipSelf: true });
    return resolved ? (fakes.get(resolved.id) ?? null) : null;
  },
};

export default defineConfig({
  root: import.meta.dirname,
  base: './',
  publicDir: resolve(frontend, 'public'),
  plugins: [fakeBackend, react()],
  build: { outDir: resolve(frontend, 'demo-dist'), emptyOutDir: true },
});
