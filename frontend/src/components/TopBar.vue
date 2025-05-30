<template>
  <div class="top-bar">
    <div class="app-name">
      <div class="flex items-center">
        <div class="sidebar-toggle-container mr-3" @click="toggleLeftSidebar" :title="getSidebarToggleTitle">
          <div class="sidebar-toggle-icon" :class="`state-${leftSidebarState}`">
            <div class="sidebar-toggle-bars">
              <span></span>
              <span></span>
              <span></span>
            </div>
            <div class="sidebar-toggle-arrow">
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="15 18 9 12 15 6"></polyline>
              </svg>
            </div>
          </div>
        </div>
        <h1>Shotgun Code</h1>
      </div>
    </div>
    <div class="controls">
      <button class="settings-button" @click="toggleSettings">
        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="3"></circle>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
        </svg>
      </button>
      <ThemeToggle />
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue';
import ThemeToggle from './ThemeToggle.vue';

const props = defineProps({
  leftSidebarState: {
    type: Number,
    default: 2 
  }
});

const emit = defineEmits(['toggle-settings', 'toggle-left-sidebar']);

const showSettings = ref(false);

function toggleSettings() {
  emit('toggle-settings');
}

function toggleLeftSidebar() {
  emit('toggle-left-sidebar');
}

const getSidebarToggleTitle = computed(() => {
  if (props.leftSidebarState === 0) {
    return 'Show sidebar';
  } else if (props.leftSidebarState === 1) {
    return 'Expand sidebar';
  } else {
    return 'Collapse sidebar';
  }
});
</script>
