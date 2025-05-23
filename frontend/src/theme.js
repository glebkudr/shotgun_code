import { ref, watch } from 'vue';

export const useThemeStore = () => {
  const getInitialTheme = () => {
    const savedTheme = localStorage.getItem('theme');
    const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    
    return savedTheme || (systemPrefersDark ? 'dark' : 'light');
  };

  const theme = ref(getInitialTheme());
  
  const toggleTheme = () => {
    theme.value = theme.value === 'light' ? 'dark' : 'light';
  };

  const setTheme = (newTheme) => {
    if (newTheme === 'dark' || newTheme === 'light') {
      theme.value = newTheme;
    }
  };

  watch(theme, (newTheme) => {
    localStorage.setItem('theme', newTheme);
    
    if (newTheme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, { immediate: true });
  
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  mediaQuery.addEventListener('change', (e) => {
    if (!localStorage.getItem('theme')) {
      setTheme(e.matches ? 'dark' : 'light');
    }
  });

  return {
    theme,
    toggleTheme,
    setTheme,
    isDark: () => theme.value === 'dark'
  };
};

const themeStore = useThemeStore();
export default themeStore;
