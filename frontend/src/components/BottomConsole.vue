<template>
  <div 
    :style="{ height: height + 'px' }" 
    class="bg-gray-800 text-white p-3 text-xs overflow-y-auto flex flex-col-reverse select-text"
    ref="consoleRootRef"
  >
    <div ref="consoleContentRef" class="flex-grow">
      <div v-for="(log, index) in logMessages" :key="index" 
           :class="['whitespace-pre-wrap break-words', getLogColor(log.type)]">
        <span class="font-medium">[{{ log.timestamp }}]</span> 
        <span v-if="log.type !== 'info'" class="font-semibold">[{{ log.type.toUpperCase() }}] </span>
        {{ log.message }}
      </div>
      <div v-if="logMessages.length === 0" class="text-gray-500">
        Console is empty.
      </div>
    </div>
  </div>
</template>

<script setup>
import { defineProps, ref, watch, nextTick } from 'vue';

const props = defineProps({
  logMessages: {
    type: Array,
    default: () => []
  },
  height: {
    type: Number,
    default: 150 // Default height if not provided
  }
});

const consoleRootRef = ref(null);
const consoleContentRef = ref(null);

function getLogColor(type) {
  switch (type) {
    case 'error': return 'text-red-400';
    case 'warn': return 'text-yellow-400';
    case 'success': return 'text-green-400';
    case 'info':
    default:
      return 'text-gray-300';
  }
}

watch(() => props.logMessages, () => {
  nextTick(() => {
    if (consoleRootRef.value) {
      // Scroll to the bottom (which is top due to flex-col-reverse)
      consoleRootRef.value.scrollTop = 0;
    }
  });
}, { deep: true });

</script>

<style scoped>
.select-text {
  user-select: text;
}

div {
  scrollbar-width: thin; /* For Firefox */
  scrollbar-color: #555 #333; /* For Firefox - thumb and track */
}

/* For Chrome, Edge, and Safari */
div::-webkit-scrollbar {
  width: 8px;
}

div::-webkit-scrollbar-track {
  background: #333; /* Darker track */
}

div::-webkit-scrollbar-thumb {
  background-color: #555; /* Lighter thumb */
  border-radius: 4px;
  border: 2px solid #333; /* Match track for padding */
}
</style> 