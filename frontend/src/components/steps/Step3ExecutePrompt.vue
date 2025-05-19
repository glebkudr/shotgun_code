<template>
  <div class="p-6 flex flex-col h-full">
    <h2 class="text-xl font-semibold text-gray-800 mb-4">Step 3: Execute Prompt</h2>
    <p class="text-gray-600 mb-4">
      <li>For now just go to Google AI studio, copy the prompt and paste it there with 2.5 pro model with 0.1 temperature. It will give you <b>the diff</b></li>
      <li>Then open any agentic code tool and ask 'apply diff' + copy-paste the diff. </li>
      <li>Diff application from the Shotgun itself will be available soon.</li>
    </p>
    
    <!--
    <button
      @click="$emit('action', 'executePrompt')"
      class="px-6 py-2 bg-green-600 text-white font-semibold rounded-md hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500 focus:ring-opacity-50 self-start mb-4"
    >
      Execute Prompt
    </button>

    <div class="flex-grow border border-gray-300 rounded-md p-4 bg-gray-800 text-gray-100 font-mono text-sm overflow-auto">
      <h3 class="text-lg font-medium text-gray-300 mb-2">Execution Console (Step Specific):</h3>
      <div v-for="(log, index) in localLogs" :key="index" class="whitespace-pre-wrap">
        <span :class="log.type === 'error' ? 'text-red-400' : (log.type === 'warn' ? 'text-yellow-400' : 'text-green-400')">[{{ log.timestamp }}]</span> {{ log.message }}
      </div>
      <p v-if="!localLogs.length" class="text-gray-400">Step-specific execution logs will appear here...</p>
    </div>
    -->
  </div>
</template>

<script setup>
import { ref, defineEmits } from 'vue';

defineEmits(['action']);

const localLogs = ref([]);

const addLog = (message, type = 'info') => {
  localLogs.value.push({ 
    message, type, timestamp: new Date().toLocaleTimeString() 
  });
};

defineExpose({ addLog });
</script> 