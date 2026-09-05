import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import Preview from './lib/views/_Preview.svelte'
import { initTheme } from './lib/stores'

// Apply the stored theme before the first paint so dark mode never flashes
// white. initTheme is idempotent; App.svelte calls it again harmlessly.
initTheme()

const target = document.getElementById('app')
if (!target) throw new Error('#app mount point is missing from index.html')

// Dev-only harness: `npm run dev` then /?preview=1&view=study renders a single
// view without the shell. The guard keeps production and the Wails build on the
// normal App path.
const isPreview = typeof location !== 'undefined' && location.search.includes('preview=')

const app = isPreview ? mount(Preview, { target }) : mount(App, { target })

export default app
