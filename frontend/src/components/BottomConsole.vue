<template>
  <div 
    :style="{ height: height + 'px' }" 
    class="console-container overflow-y-auto flex flex-col-reverse select-text"
    ref="consoleRootRef"
  >
    <div ref="consoleContentRef" class="flex-grow">
      <div v-for="(log, index) in logMessages" :key="index" 
           :class="['whitespace-pre-wrap break-words', getLogColor(log.type)]">
        <span class="font-medium">[{{ log.timestamp }}]</span> 
        <span v-if="log.type !== 'info'" class="font-semibold">[{{ log.type.toUpperCase() }}] </span>
        {{ log.message }}
      </div>
      <div v-if="logMessages.length === 0" class="text-hint">
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
    default: 150
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
      consoleRootRef.value.scrollTop = 0;
    }
  });
}, { deep: true });

</script>

<!-- Styles moved to main.css --> 