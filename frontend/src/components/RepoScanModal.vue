<template>
  <div v-if="isVisible" class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50 flex justify-center items-center" @click.self="handleCancel">
    <div class="relative mx-auto p-5 border w-full max-w-2xl shadow-lg rounded-md bg-white dark:bg-slate-800">
      <div class="mt-3 text-center">
        <h3 class="text-lg leading-6 font-medium text-gray-900">Edit Repo Scan</h3>
        <div class="mt-2 px-7 py-3">
          <textarea 
            v-model="editableScan"
            rows="15"
            class="w-full p-2 border border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 text-sm font-mono bg-white dark:bg-slate-700"
            placeholder="Paste your repo scan here..."
          ></textarea>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 text-left">
            Repo scan is attached to your context extraction prompt to better understand the repository structure and extract the right context. Any text format is supported here. Markdown, JSON, CSV, etc.
            You may create it on <a href="#" @click.prevent="openLink" class="text-blue-600 dark:text-blue-400 hover:underline">shotgunpro.dev</a>
          </p>
        </div>
        <div class="items-center px-4 py-3">
          <button
            @click="handleSave"
            class="px-4 py-2 bg-blue-500 text-white text-base font-medium rounded-md w-auto hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 mr-2 dark:bg-blue-500 dark:hover:bg-blue-700"
          >
            Save
          </button>
          <button
            @click="handleCancel"
            class="px-4 py-2 bg-gray-200 text-gray-800 text-base font-medium rounded-md w-auto hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-400 dark:bg-slate-700 dark:text-gray-100 dark:hover:bg-slate-600"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, defineProps, defineEmits } from 'vue';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';

const props = defineProps({
  isVisible: {
    type: Boolean,
    required: true,
  },
  initialScan: {
    type: String,
    default: '',
  }
});

const emit = defineEmits(['save', 'cancel']);

const editableScan = ref('');

watch(() => props.initialScan, (newVal) => {
  editableScan.value = newVal;
}, { immediate: true });

watch(() => props.isVisible, (newVal) => {
  if (newVal) {
    editableScan.value = props.initialScan;
  }
});

function handleSave() {
  emit('save', editableScan.value);
}

function handleCancel() {
  emit('cancel');
}

function openLink() {
  BrowserOpenURL('https://shotgunpro.dev');
}
</script>

<style scoped>
</style>
