<template>
  <div class="section-wrapper">
    <div class="content-container">
      <h2 class="text-title mb-4">Step 4: Apply Patch</h2>
      
      <div v-if="isLoading" class="flex-grow flex justify-center items-center">
        <p class="text-secondary">Loading split diffs...</p>
      </div>
      
      <div v-else-if="splitDiffs && splitDiffs.length > 0" class="section-container">
        <p class="text-hint mb-2">
          The original diff has been split into {{ splitDiffs.length }} smaller diffs.
          Copy each part and apply it using your preferred tool. With an LLM, just tell it to <strong>apply the diff</strong>.
        </p>
        <div v-for="(diff, index) in splitDiffs" :key="index" :class="isCopied[index] ? 'bg-elevated' : 'bg-secondary'">
          <div class="flex justify-between items-center">
            <h3 class="text-subtitle">Split {{ index + 1 }} of {{ splitDiffs.length }}</h3>
            <div class="flex items-center space-x-2">
              <!-- SOON: add a feature to apply the diff automatically -->
              <!-- <button
                class="btn-secondary" 
                disabled
              >
                Apply Diff
              </button> -->
              <button
                @click="copyDiffToClipboard(diff, index)"
                class="btn-secondary"
              >
                {{ copyButtonTexts[index] || 'Copy' }}
              </button>
            </div>
          </div>
          <div class="text-hint mb-2">
             <!-- the lines metric will be orange if it's greater than props.splitLineLimit + 5%, red if it's greater than props.splitLineLimit + 20%, green if it's less than props.splitLineLimit + 5% -->
              <!-- calculate this in the vue script below, to simplify the code -->
            <div class="inline-block px-2 py-1 rounded-full text-xs" :class="getLineMetricClass(diff.split('\n').length)">
              {{ diff.split('\n').length }} lines
            </div>
            <div class="inline-block px-2 py-1 bg-secondary rounded-full text-xs ml-2">
              {{ (diff.match(/^diff --git/gm) || []).length }} file{{ (diff.match(/^diff --git/gm) || []).length === 1 ? '' : 's' }}
            </div>
            <div class="inline-block px-2 py-1 bg-secondary rounded-full text-xs ml-2">
              {{ (diff.match(/^@@ .* @@/gm) || []).length }} hunk{{ (diff.match(/^@@ .* @@/gm) || []).length === 1 ? '' : 's' }}
            </div>
          </div>
          <textarea
            :value="diff"
            rows="10"
            readonly
            class="input-textarea text-code"
          ></textarea>
        </div>
      </div>
      
      <div v-else class="flex-grow flex justify-center items-center">
        <p class="text-secondary">No split diffs to display. Go to Step 3 to split a diff.</p>
      </div>

      
      <div class="mt-6 flex space-x-4 flex-shrink-0 flex-row justify-between">
        <div>
          <h3 class="text-subtitle mb-2">Apply Patch automatically <sup class="text-xs text-white bg-green-500 rounded-md px-1 py-1">COMING SOON</sup></h3>
          <p class="text-hint italic">
            Here you will review and apply the patch. For now, it's a placeholder. Click 'Finish' to simulate completion.
          </p>
        </div>
        <div>
          <button
            @click="$emit('action', 'finishSplitting'), finishButtonText = 'Hooray! 🎉'"
            class="btn-primary"
            :class="finishButtonText === 'Hooray! 🎉' ? 'bg-elevated text-primary hover:bg-elevated' : ''"
          >
            {{ finishButtonText }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, defineProps, defineEmits, watch } from 'vue';

const emit = defineEmits(['action']);
const finishButtonText = ref('Finish');

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
    
    const originalText = isCopied.value[index] ? 'Copy ✅' : 'Copy';
    copyButtonTexts.value[index] = 'Failed!';

    setTimeout(() => {
      copyButtonTexts.value[index] = originalText; // Revert to previous state ("Copy" or "Copy ✅")
    }, 2000);
  }
}

function checkCompletion() {
  if (props.splitDiffs && props.splitDiffs.length > 0) {
    const atLeastOneCopied = Object.values(isCopied.value).some(copied => copied === true);
    return atLeastOneCopied;
  }
  return false;
}

defineExpose({
  checkCompletion
});
</script> 