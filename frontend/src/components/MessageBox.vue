<template>
  <transition name="message-fade">
    <div v-if="currentMessage" class="message-box">
      <div :class="['message-item', getMessageTypeClass(currentMessage.type)]">
        <div class="message-content">
          <div class="message-text">{{ currentMessage.message }}</div>
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue';

const props = defineProps({
  messages: {
    type: Array,
    default: () => []
  },
  autoDismissTime: {
    type: Number,
    default: 3000
  }
});

const currentMessage = ref(null);
let dismissTimer = null;

function processMessages() {
  if (props.messages.length === 0) {
    currentMessage.value = null;
    return;
  }
  
  const latestMessage = props.messages[props.messages.length - 1];
  
  const showDebug = localStorage.getItem('shotgun-show-debug-messages') === 'true';
  const showInfo = localStorage.getItem('shotgun-show-info-messages') === 'true';

  if (latestMessage.type === 'error' || 
      latestMessage.type === 'warn' || 
      latestMessage.type === 'success' || 
      (latestMessage.type === 'debug' && showDebug) || 
      (latestMessage.type === 'info' && showInfo)) {
    
    currentMessage.value = latestMessage;
    
    if (dismissTimer) {
      clearTimeout(dismissTimer);
    }
    
    dismissTimer = setTimeout(() => {
      currentMessage.value = null;
    }, props.autoDismissTime);
  }
}

function getMessageTypeClass(type) {
  switch (type) {
    case 'error': return 'message-error';
    case 'warn': return 'message-warning';
    case 'success': return 'message-success';
    case 'debug': return 'message-debug';
    case 'info':
    default:
      return 'message-info';
  }
}

onUnmounted(() => {
  if (dismissTimer) {
    clearTimeout(dismissTimer);
  }
});

watch(() => props.messages, () => {
  processMessages();
}, { deep: true, immediate: true });


</script>
