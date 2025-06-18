import {createApp} from 'vue'
import App from './App.vue'
import './assets/main.css'

// Initialize dark mode class on the HTML element based on stored preference or system preference
const initDarkMode = () => {
  // Get saved theme preference from localStorage
  const savedTheme = localStorage.getItem('theme')
  // Check if system prefers dark mode
  const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  
  // Apply dark mode class if either saved preference is 'dark' or system prefers dark (and no saved preference)
  if (savedTheme === 'dark' || (!savedTheme && systemPrefersDark)) {
    document.documentElement.classList.add('dark')
  }
}

// Initialize dark mode before mounting the app
initDarkMode()

createApp(App).mount('#app')
