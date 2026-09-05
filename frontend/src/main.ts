import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { initTheme } from './lib/stores'

// Apply the stored theme before the first paint so dark mode never flashes
// white. initTheme is idempotent; App.svelte calls it again harmlessly.
initTheme()

const target = document.getElementById('app')
if (!target) throw new Error('#app mount point is missing from index.html')

const app = mount(App, { target })

export default app
