import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

createApp(App).mount('#app')

// Never cache authenticated feeds or media in the standalone app.
if (import.meta.env.PROD && window.isSecureContext && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js', { updateViaCache: 'none' }).catch(error => {
      console.warn('PWA registration unavailable', error)
    })
  }, { once: true })
}
