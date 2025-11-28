import {createApp} from 'vue'
import App from './App.vue'
import './assets/main.css' // Basic styles

// Theme handling: supports 'light' | 'dark' | 'system'
function applyTheme(mode) {
	try {
		if (mode === 'dark') {
			document.documentElement.classList.add('dark');
		} else if (mode === 'light') {
			document.documentElement.classList.remove('dark');
		} else {
			// system
			const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
			if (prefersDark) {
				document.documentElement.classList.add('dark');
			} else {
				document.documentElement.classList.remove('dark');
			}
		}
	} catch (e) {
		// ignore
	}
}

function getSavedTheme() {
	try {
		return localStorage.getItem('shotgun_theme');
	} catch (e) {
		return null;
	}
}

function setSavedTheme(value) {
	try {
		if (value === null || typeof value === 'undefined') {
			localStorage.removeItem('shotgun_theme');
		} else {
			localStorage.setItem('shotgun_theme', value);
		}
	} catch (e) {
		// ignore
	}
}

// Initialize theme from saved preference or system
const saved = getSavedTheme();
if (saved === 'dark' || saved === 'light' || saved === 'system') {
	applyTheme(saved);
} else {
	// default to system if nothing saved
	applyTheme('system');
}

// Listen to system preference changes and re-apply only when user chose 'system'
if (window.matchMedia) {
	const mq = window.matchMedia('(prefers-color-scheme: dark)');
	mq.addEventListener?.('change', (e) => {
		const current = getSavedTheme() || 'system';
		if (current === 'system') {
			applyTheme('system');
		}
	});
}

// Expose helpers for UI to set theme mode programmatically
window.setShotgunTheme = function (mode) {
	try {
		if (!mode || (mode !== 'light' && mode !== 'dark' && mode !== 'system')) {
			return false;
		}
		setSavedTheme(mode);
		applyTheme(mode);
		return true;
	} catch (e) {
		return false;
	}
}

// Backwards-compatible toggle (keeps previous behaviour: toggles between light/dark and persists)
window.toggleShotgunTheme = function () {
	try {
		const current = getSavedTheme();
		if (current === 'system' || !current) {
			// If system or unset, infer current from DOM
			const isDark = document.documentElement.classList.contains('dark');
			const newMode = isDark ? 'light' : 'dark';
			setSavedTheme(newMode);
			applyTheme(newMode);
			return newMode === 'dark';
		}
		const newMode = current === 'dark' ? 'light' : 'dark';
		setSavedTheme(newMode);
		applyTheme(newMode);
		return newMode === 'dark';
	} catch (e) {
		return null;
	}
}

createApp(App).mount('#app')
