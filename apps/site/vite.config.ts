import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    fs: {
      // The screenshots are read from docs/assets/screenshots at the repository
      // root rather than copied into static/. The production build resolves
      // them itself; `vite dev` needs to be told it may serve from up there.
      allow: ['..', '../..']
    }
  }
});
