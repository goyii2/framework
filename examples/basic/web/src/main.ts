import './main.css'

// The whole frontend of the demo: prove the bundle ran by taking over the
// #vite-demo button (rendered by the templ pages) and counting clicks. In dev
// mode this file is served straight from the Vite dev server with HMR; in
// build mode it loads as the hashed asset the manifest points at.
const button = document.getElementById('vite-demo') as HTMLButtonElement | null
let clicks = 0
if (button) {
  button.textContent = 'Vite bundle loaded — click me'
  button.addEventListener('click', () => {
    clicks += 1
    button.textContent = `Vite counted ${clicks} click${clicks === 1 ? '' : 's'}`
  })
}
