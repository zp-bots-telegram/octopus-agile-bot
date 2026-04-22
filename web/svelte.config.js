import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	compilerOptions: {
		runes: ({ filename }) =>
			filename.split(/[/\\]/).includes('node_modules') ? undefined : true
	},
	kit: {
		// Build a Single-Page App. fallback: every unknown path returns index.html so
		// the client-side router handles routing. No prerender (all pages need an
		// authenticated session).
		adapter: adapter({ fallback: 'index.html' }),
		prerender: { entries: [] }
	}
};

export default config;
