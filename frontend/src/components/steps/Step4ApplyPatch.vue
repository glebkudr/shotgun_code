<template>
  <div class="p-6 flex flex-col h-full">
    <h2 class="text-xl font-semibold text-gray-800 mb-4">Step 4: Apply Patch</h2>
    
    <div v-if="isLoading" class="flex-grow flex justify-center items-center">
      <p class="text-gray-600">Loading split diffs...</p>
    </div>
    
    <div v-else-if="splitDiffs && splitDiffs.length > 0" class="flex-grow overflow-y-auto space-y-6">
      <p class="text-gray-600 mb-2 text-xs">
        The original diff has been split into {{ splitDiffs.length }} smaller diffs.
        Copy each part and apply it using your preferred tool. With an LLM, just tell it to <strong>apply the diff</strong>.
      </p>
      <div v-for="(diff, index) in splitDiffs" :key="index" :class="['border border-gray-300 rounded-md p-4', isCopied[index] ? 'bg-green-50' : 'bg-gray-50', 'shadow-sm']">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-medium text-gray-700">Split {{ index + 1 }} of {{ splitDiffs.length }}</h3>
          <div class="flex items-center space-x-2">
            <!-- SOON: add a feature to apply the diff automatically -->
            <!-- <button
              class="px-3 py-1 bg-gray-100 text-gray-300 text-xs font-semibold rounded-md focus:outline-none focus:ring-2 focus:ring-gray-400"
              disabled
            >
              Apply Diff
            </button> -->
            <button
              @click="copyDiffToClipboard(diff, index)"
              class="px-3 py-1 bg-gray-200 text-gray-700 text-xs font-semibold rounded-md hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-gray-400"
            >
              {{ copyButtonTexts[index] || 'Copy' }}
            </button>
          </div>
        </div>
        <div class="text-gray-600 text-xs mb-2">
           <!-- the lines metric will be orange if it's greater than props.splitLineLimit + 5%, red if it's greater than props.splitLineLimit + 20%, green if it's less than props.splitLineLimit + 5% -->
            <!-- calculate this in the vue script below, to simplify the code -->
          <div class="inline-block px-2 py-1 rounded-full text-xs" :class="getLineMetricClass(diff.split('\n').length)">
            {{ diff.split('\n').length }} lines
          </div>
          <div class="inline-block px-2 py-1 bg-blue-100 rounded-full text-xs ml-2">
            {{ (diff.match(/^diff --git/gm) || []).length }} file{{ (diff.match(/^diff --git/gm) || []).length === 1 ? '' : 's' }}
          </div>
          <div class="inline-block px-2 py-1 bg-blue-100 rounded-full text-xs ml-2">
            {{ (diff.match(/^@@ .* @@/gm) || []).length }} hunk{{ (diff.match(/^@@ .* @@/gm) || []).length === 1 ? '' : 's' }}
          </div>
        </div>
        <textarea
          :value="diff"
          rows="10"
          readonly
          class="w-full p-2 border border-gray-200 rounded-md bg-white font-mono text-xs"
          style="min-height: 150px;"
        ></textarea>
      </div>
    </div>
    
    <div v-else class="flex-grow flex justify-center items-center">
      <p class="text-gray-500">No split diffs to display. Go to Step 3 to split a diff.</p>
    </div>

    
    <div class="mt-6 flex space-x-4 flex-shrink-0 flex-row justify-between">
      <div>
        <h3 class="text-lg font-medium text-gray-700 mb-2">Apply Patch automatically <sup class="text-xs text-white bg-green-500 rounded-md px-1 py-1">COMING SOON</sup></h3>
        <p class="text-gray-600 italic text-xs">
          Here you will review and apply the patch. For now, it’s a placeholder. Click ‘Finish’ to simulate completion.
        </p>
      </div>
      <button
      @click="$emit('action', 'finishSplitting'), finishButtonText = 'Hooray! 🎉'"
      class="px-6 py-2 bg-blue-500 text-white font-semibold rounded-md hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
      :class="finishButtonText === 'Hooray! 🎉' ? 'bg-green-200 text-black hover:bg-green-200' : ''"
      >
        {{ finishButtonText }}
      </button>
    </div>
  </div>
</template>

<script setup>
const finishButtonText = ref('Finish');
import { ref, defineProps, watch } from 'vue';
// import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime'; // If needed for specific platforms

const props = defineProps({
  splitDiffs: {
    type: Array,
    default: () => []
  },
  isLoading: { // To indicate if MainLayout is fetching/processing splits
    type: Boolean,
    default: false
  },
  platform: {
    type: String,
    default: 'unknown'
  },
  splitLineLimit: { // Add the new prop
    type: Number,
    default: 500 // Provide a default value if the prop is not passed
  }
});

defineEmits(['action']);

const copyButtonTexts = ref({});
const isCopied = ref({}); // Tracks if a split has been successfully copied at least once

function getLineMetricClass(lineCount) {
  const limit = props.splitLineLimit;
  // clamp the thresholds to maximum 100 or 200 lines over the limit
  const orangeThreshold = Math.min(limit * 1.1, limit + 100);
  const redThreshold = Math.min(limit * 1.3, limit + 200);
  
  if (lineCount > redThreshold) {
    return 'bg-red-100';
  } else if (lineCount > orangeThreshold) {
    return 'bg-orange-100';
  } else {
    return 'bg-green-100';
  }
}

watch(() => props.splitDiffs, (newVal) => {
  // Reset copy button texts and copied states when diffs change
  const newTexts = {};
  const newCopiedStates = {};
  if (newVal) {
    newVal.forEach((_, index) => {
      newTexts[index] = 'Copy';
      newCopiedStates[index] = false; // Initialize as not copied
    });
  }
  copyButtonTexts.value = newTexts;
  isCopied.value = newCopiedStates;
}, { immediate: true, deep: true }); // Use deep: true if splitDiffs could be mutated internally, though usually props are replaced.


async function copyDiffToClipboard(diffContent, index) {
  if (!diffContent) return;
  try {
    await navigator.clipboard.writeText(diffContent);
    
    isCopied.value[index] = true; // Mark as successfully copied
    copyButtonTexts.value[index] = 'Copied! ✅';

    setTimeout(() => {
      copyButtonTexts.value[index] = 'Copy ✅'; // Persistent "copied" state text
    }, 2000);
  } catch (err) {
    console.error(`Failed to copy diff split ${index + 1}: `, err);
    
    // Temporarily show "Failed!"
    const originalText = isCopied.value[index] ? 'Copy ✅' : 'Copy';
    copyButtonTexts.value[index] = 'Failed!';

    setTimeout(() => {
      copyButtonTexts.value[index] = originalText; // Revert to previous state ("Copy" or "Copy ✅")
    }, 2000);
  }
}
</script> 