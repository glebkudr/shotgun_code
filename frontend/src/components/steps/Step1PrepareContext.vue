<template>
  <div class="content-container max-w-4xl mx-auto p-4 h-full flex flex-col">
    <!-- Loading State: Progress Bar -->
    <div v-if="isLoadingContext" class="flex-grow flex justify-center items-center p-4">
      <div class="text-center w-full max-w-sm">
        <p class="text-secondary mb-4">Generating project context...</p>
        <div class="progress-container mb-2">
          <div class="progress-bar" :style="{ width: progressBarWidth }"></div>
        </div>
        <p class="text-hint">
          {{ generationProgress.current }} / {{ generationProgress.total > 0 ? generationProgress.total : 'calculating...' }} items
        </p>
      </div>
    </div>

    <!-- Content Area when project is selected -->
    <div v-else-if="projectRoot" class="flex-grow flex flex-col h-full">
      <h2 class="text-title mb-4">Step 1: Prepare Context</h2>

      <div v-if="generatedContext && !generatedContext.startsWith('Error:')" class="section-container flex-grow flex flex-col">
        <div class="section-header">
          <h3 class="section-label">Generated Project Context:</h3>
          <button
            @click="copyGeneratedContextToClipboard"
            class="btn-secondary text-xs"
          >
            {{ copyButtonText }}
          </button>
        </div>
        <textarea
          :value="generatedContext"
          readonly
          class="input-textarea flex-grow"
          placeholder="Context will appear here. If empty, ensure files are selected and not all excluded."
        ></textarea>
      </div>
      
      <!-- Error generating context -->
      <div v-else-if="generatedContext && generatedContext.startsWith('Error:')" class="section-container bg-error flex-grow flex flex-col">
        <div class="section-header">
          <h4 class="section-label text-error">Error Generating Context:</h4>
        </div>
        <pre class="error-details">{{ generatedContext.substring(6).trim() }}</pre>
      </div>
      
      <!-- Waiting for context generation -->
      <div v-else class="section-container flex-grow flex flex-col">
        <p class="text-hint text-center p-8">
          Project context will be generated automatically. If empty after generation, ensure files are selected and not all excluded.
        </p>
      </div>
    </div>

    <!-- Initial message when no project is selected -->
    <p v-else class="text-hint flex-grow flex justify-center items-center text-center">
      Select a project folder to begin.
    </p>
  </div>
</template>

<script setup>
import { defineProps, ref, computed } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';

const props = defineProps({
  generatedContext: {
    type: String,
    default: ''
  },
  projectRoot: {
    type: String,
    default: ''
  },
  isLoadingContext: { // New prop
    type: Boolean,
    default: false
  },
  generationProgress: { // New prop for progress data
    type: Object,
    default: () => ({ current: 0, total: 0 })
  },
  platform: { // To know if we are on macOS
    type: String,
    default: 'unknown'
  }
});

const progressBarWidth = computed(() => {
  if (props.generationProgress && props.generationProgress.total > 0) {
    const percentage = (props.generationProgress.current / props.generationProgress.total) * 100;
    return `${Math.min(100, Math.max(0, percentage))}%`;
  }
  return '0%';
});
const copyButtonText = ref('Copy All');

async function copyGeneratedContextToClipboard() {
  if (!props.generatedContext) return;
  try {
    await navigator.clipboard.writeText(props.generatedContext);
    //if (props.platform === 'darwin') {
    //  await WailsClipboardSetText(props.generatedContext);
    //} else {
    //  await navigator.clipboard.writeText(props.generatedContext);
    //}
    copyButtonText.value = 'Copied!';
    setTimeout(() => {
      copyButtonText.value = 'Copy All';
    }, 2000);
  } catch (err) {
    console.error('Failed to copy context: ', err);
    if (props.platform === 'darwin' && err) {
      console.error('darvin ClipboardSetText failed for context:', err);
    }
    copyButtonText.value = 'Failed!';
    setTimeout(() => {
      copyButtonText.value = 'Copy All';
    }, 2000);
  }
}
</script>