<template>
  <div class="content-container p-6">
    <h2 class="text-title mb-3">Step 3: Execute Prompt & Split Diff</h2>
    <p class="text-secondary mb-3">
      For now, please go to an external LLM provider like Google AI Studio or an equivalent.
      Copy the full project context generated in Step 1 and the prompt you composed in Step 2.
      Paste them into the LLM and obtain the resulting diff output.
    </p>
    <p class="text-secondary mb-4">
      Then, paste the full <code class="code-inline">gitDiff</code> output (the LLM's response) below.
      You can also specify the approximate number of lines per split, or leave it as the total number of lines if you don't want to split the diff.
    </p>

    <div class="info-box mb-4">
      <h4 class="info-title">Why Split the Diff?</h4>
      <p class="text-secondary">
        Sometimes, the generated diff is a large file that can be difficult to apply with some LLMs or review tools. 
        Splitting it into smaller parts makes it easier to manage and reduces the risk of errors.
      </p>
    </div>

    <div class="section-container mb-4">
      <div class="section-header">Git Diff Output:</div>
      <textarea
        id="shotgun-git-diff-input"
        v-model="localShotgunGitDiffInput"
        rows="15"
        class="input-textarea font-mono"
        placeholder="Paste the git diff output here, e.g., diff --git a/file.txt b/file.txt..."
      ></textarea>
    </div>

    <div class="section-container mb-4">
      <div class="section-header">Approx. Lines per Split:</div>
      <p class="text-hint mb-3">
        ⓘ This will attempt to split the diff into the specified number of lines, while keeping the original structure and the hunks.
        The exact number of lines per split is not guaranteed, but the diff will be split into as many parts as possible.
        <br>
        Leave this unchanged if you don't want to split the diff.
      </p>
      <div class="flex items-center gap-4 mb-3">
        <input
          type="number"
          id="split-line-limit"
          v-model.number="localSplitLineLimit"
          min="50"
          step="50"
          class="input-number w-32"
        />
        <span class="text-hint">
          Total lines: {{ shotgunGitDiffInputLines }} 
          <button 
            @click="resetSplitLineLimit" 
            class="text-link"
            title="Reset to total number of lines"
          >
            (reset)
          </button>
        </span>
      </div>
    </div>

    <div class="mt-6">
      <button
        @click="handleSplitDiff"
        :disabled="!localShotgunGitDiffInput.trim() || localSplitLineLimit <= 0"
        class="btn-primary w-full sm:w-auto"
      >
        {{ localSplitLineLimit === shotgunGitDiffInputLines ? 'Proceed to Apply' : 'Split Diff & Proceed to Apply' }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, defineEmits, watch, computed, onMounted, onBeforeUnmount } from 'vue';
import { LogInfo as LogInfoRuntime, LogError as LogErrorRuntime } from '../../../wailsjs/runtime/runtime';

const emit = defineEmits(['action', 'update:shotgunGitDiff', 'update:splitLineLimit']);

const props = defineProps({
  initialGitDiff: {
    type: String,
    default: ''
  },
  initialSplitLineLimit: {
    type: Number,
    default: 0
  }
});


const localShotgunGitDiffInput = ref(props.initialGitDiff);

const localSplitLineLimit = ref(props.initialSplitLineLimit > 0 ? props.initialSplitLineLimit : 500);

onMounted(() => {
    
  localShotgunGitDiffInput.value = props.initialGitDiff;

    
  if (props.initialSplitLineLimit > 0) {
    localSplitLineLimit.value = props.initialSplitLineLimit;
  } else if (localSplitLineLimit.value <= 0) {
    localSplitLineLimit.value = 500;
  }
});

const shotgunGitDiffInputLines = computed(() => {
  return localShotgunGitDiffInput.value ? localShotgunGitDiffInput.value.split('\n').length : 0;
});

watch(() => props.initialGitDiff, (newVal, oldVal) => {
        if (newVal !== localShotgunGitDiffInput.value) {
                localShotgunGitDiffInput.value = newVal;
            }
});

watch(() => props.initialSplitLineLimit, (newVal, oldVal) => {
        if (newVal > 0 && newVal !== localSplitLineLimit.value) {
        localSplitLineLimit.value = newVal;
    } else if (newVal <= 0 && localSplitLineLimit.value !== 500 && props.initialGitDiff === '') {
        localSplitLineLimit.value = 500;
    }
});

let diffInputDebounceTimer = null;
watch(localShotgunGitDiffInput, (newVal, oldVal) => {
    
    clearTimeout(diffInputDebounceTimer);
    
    diffInputDebounceTimer = setTimeout(() => {
                if (newVal !== props.initialGitDiff) {
                        emit('update:shotgunGitDiff', newVal);
        } else {
                    }
        if (newVal && newVal.trim() !== '') {
            const lines = newVal.split('\n').length;
            const currentLimit = localSplitLineLimit.value;

            if (currentLimit === 500 || (currentLimit !== lines && currentLimit === (newVal.substring(0, newVal.length - (newVal.split('\n').pop().length +1)).split('\n').length))) {
                if (lines > 0 && lines !== currentLimit) {
                    localSplitLineLimit.value = lines;
                }
            } else if (lines === 0 && currentLimit !== 500){
                 localSplitLineLimit.value = 500;
            }
        } else if ((!newVal || newVal.trim() === '') && localSplitLineLimit.value !== 500) {
            localSplitLineLimit.value = 500;
        }
    }, 300);
});

let limitDebounceTimer = null;
watch(localSplitLineLimit, (newVal) => {
    clearTimeout(limitDebounceTimer);
    limitDebounceTimer = setTimeout(() => {
        if (newVal > 0 && newVal !== props.initialSplitLineLimit) { 
            emit('update:splitLineLimit', newVal);
        } else if (newVal <= 0 && props.initialSplitLineLimit > 0) {
        }
    }, 300);
});

onBeforeUnmount(() => {
    // Clear any pending debounced updates
  clearTimeout(diffInputDebounceTimer);
  clearTimeout(limitDebounceTimer);
  
  // Immediately emit the current value of localShotgunGitDiffInput if it's different from the prop
    if (localShotgunGitDiffInput.value !== props.initialGitDiff) {
        emit('update:shotgunGitDiff', localShotgunGitDiffInput.value);
  } else {
       }

  // Immediately emit the current value of localSplitLineLimit if it's valid and different from the prop
    if (localSplitLineLimit.value > 0 && localSplitLineLimit.value !== props.initialSplitLineLimit) {
        emit('update:splitLineLimit', localSplitLineLimit.value);
  } else {
      }
});

function handleSplitDiff() {
  if (!localShotgunGitDiffInput.value.trim() || localSplitLineLimit.value <= 0) {
    return;
  }
  emit('action', 'executePromptAndSplitDiff', {
    gitDiff: localShotgunGitDiffInput.value,
    lineLimit: localSplitLineLimit.value
  });
}

const resetSplitLineLimit = () => {
  if (shotgunGitDiffInputLines.value > 0) {
    localSplitLineLimit.value = shotgunGitDiffInputLines.value;
  } else {
    localSplitLineLimit.value = 500;
  }
}

// Simplified to just return validation status since step completion is now handled by CentralPanel
function checkCompletion() {
  return localShotgunGitDiffInput.value && localShotgunGitDiffInput.value.trim() !== '';
}

defineExpose({
  checkCompletion
});
</script> 