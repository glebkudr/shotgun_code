<template>
  <Teleport to="body">
    <div v-if="isVisible" class="settings-overlay" @click.self="close">
      <div class="settings-panel">
        <div class="settings-header">
          <h2>Settings</h2>
          <button @click="close" class="close-button">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
        
        <div class="settings-body">
          <div class="settings-section">
            <h3>Debug Options</h3>
            <div class="setting-item">
              <label class="setting-label">
                <input 
                  type="checkbox" 
                  v-model="showDebugMessages" 
                  @change="updateShowDebugMessages"
                />
                <span>Show debug messages in notifications</span>
              </label>
              <div class="setting-description">
                When enabled, debug messages will appear in the notification system
              </div>
            </div>
          </div>
          
          <div class="settings-section">
            <h3>Notification Settings</h3>
            <div class="setting-item">
              <label class="setting-label">
                <input 
                  type="checkbox" 
                  v-model="showInfoMessages" 
                  @change="updateShowInfoMessages"
                />
                <span>Show info messages in notifications</span>
              </label>
              <div class="setting-description">
                When enabled, all info messages will appear in the notification system
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue';

const props = defineProps({
  isVisible: {
    type: Boolean,
    default: false
  }
});

const emit = defineEmits(['close']);

const showDebugMessages = ref(false);
const showInfoMessages = ref(false);

// Watch for visibility changes to handle body scrolling
watch(() => props.isVisible, (newValue) => {
  if (newValue) {
    // Prevent background scrolling when modal is open
    document.body.style.overflow = 'hidden';
    // Force focus trap in the modal
    nextTick(() => {
      const firstInput = document.querySelector('.settings-panel input');
      if (firstInput) firstInput.focus();
    });
  } else {
    // Restore scrolling when modal is closed
    document.body.style.overflow = '';
  }
}, { immediate: true });

onMounted(() => {
  // Load settings from localStorage
  showDebugMessages.value = localStorage.getItem('shotgun-show-debug-messages') === 'true';
  showInfoMessages.value = localStorage.getItem('shotgun-show-info-messages') === 'true';
  
  // Add escape key listener
  window.addEventListener('keydown', handleKeyDown);
});

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown);
  // Ensure we restore scrolling when component is unmounted
  document.body.style.overflow = '';
});

function handleKeyDown(event) {
  if (event.key === 'Escape' && props.isVisible) {
    close();
  }
}

function updateShowDebugMessages() {
  localStorage.setItem('shotgun-show-debug-messages', showDebugMessages.value);
}

function updateShowInfoMessages() {
  localStorage.setItem('shotgun-show-info-messages', showInfoMessages.value);
}

function close() {
  emit('close');
}
</script>
