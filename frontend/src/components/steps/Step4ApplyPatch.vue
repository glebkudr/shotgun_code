<template>
  <div class="p-6 flex flex-col h-full">
    <h2 class="text-xl font-semibold text-gray-800 mb-4">Step 4: Apply Patch Interactively</h2>
    <p class="text-gray-600 mb-4">
      The diff has been "executed". Here you would see an interactive patch editor
      allowing you to review and apply changes hunk by hunk.
    </p>

    <div class="flex-grow border border-gray-300 rounded-md p-4 bg-white overflow-auto">
      <h3 class="text-lg font-medium text-gray-700 mb-2">Interactive Patch Editor (Stub):</h3>
      <div class="space-y-4">
        <div v-for="i in 3" :key="i" class="border p-3 rounded-md bg-gray-50 shadow">
          <p class="font-mono text-sm mb-2">File: <code>src/example/File{{ i }}.txt</code></p>
          <div class="bg-red-100 text-red-700 p-2 rounded text-xs font-mono">- old example line {{ i }}</div>
          <div class="bg-green-100 text-green-700 p-2 rounded text-xs font-mono">+ new example line {{ i }}</div>
          <div class="mt-2">
            <label class="flex items-center cursor-pointer">
              <input type="checkbox" class="form-checkbox h-4 w-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500" checked />
              <span class="ml-2 text-sm text-gray-700">Apply this hunk</span>
            </label>
          </div>
        </div>
      </div>
      <p class="mt-4 text-sm text-gray-500 italic">A full interactive patch editor would be implemented here.</p>
    </div>
    
    <div class="mt-6 flex space-x-4 flex-shrink-0">
      <button
        @click="$emit('action', 'applySelectedPatches')"
        class="px-6 py-2 bg-teal-600 text-white font-semibold rounded-md hover:bg-teal-700 focus:outline-none focus:ring-2 focus:ring-teal-500 focus:ring-opacity-50"
      >
        Apply Selected
      </button>
      <button
        @click="$emit('action', 'applyAllPatches')"
        class="px-6 py-2 bg-blue-500 text-white font-semibold rounded-md hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
      >
        Apply All & Finish
      </button>
    </div>
  </div>
</template>

<script setup>
import { defineEmits } from 'vue';
defineEmits(['action']);
</script> 