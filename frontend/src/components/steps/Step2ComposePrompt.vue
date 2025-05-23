<template>
  <div class="content-container max-w-4xl mx-auto p-4">
    <p class="text-secondary text-center mb-4">
      Write the task for the LLM and review the generated prompt
    </p>

    <CustomRulesModal
      :is-visible="isPromptRulesModalVisible"
      :initial-rules="currentPromptRulesForModal"
      title="Edit Custom Prompt Rules"
      ruleType="prompt"
      @save="handleSavePromptRules"
      @cancel="handleCancelPromptRules"
    />

    <div class="flex-grow flex flex-col space-y-4">
      <!-- User Task Section -->
      <div class="section-container">
        <div class="section-header">
          <label for="user-task-ai" class="section-label">Your task for AI:</label>
          <button 
            @click="openPromptRulesModal" 
            title="Edit custom rules" 
            class="btn-secondary text-xs"
          >
            <span class="mr-1">⚙️</span> Rules
          </button>
        </div>
        <textarea
          id="user-task-ai"
          v-model="localUserTask"
          rows="6"
          class="input-textarea resize-none mb-2"
          placeholder="Describe what the AI should do..."
        ></textarea>
      </div>

      <!-- Prompt Section -->
      <div class="section-container">
        <div class="section-header">
          <h3 class="text-subtitle">Prompt:</h3>
          <button
            @click="copyFinalPromptToClipboard"
            :disabled="!props.finalPrompt || isLoadingFinalPrompt"
            class="btn-primary text-xs"
          >
            {{ copyButtonText }}
          </button>
        </div>

        <div v-if="isLoadingFinalPrompt" class="loading-state">
          <div class="spinner"></div>
          <p class="loading-text">Updating prompt...</p>
        </div>

        <textarea
          v-else
          :value="props.finalPrompt"
          @input="e => emit('update:finalPrompt', e.target.value)"
          rows="15"
          class="input-textarea flex-grow"
          placeholder="The final prompt will be generated here..."
        ></textarea>
        
        <div class="prompt-footer">
          <p class="text-hint">
            The prompt updates automatically. Manual changes may be overwritten when source data is updated.
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, computed } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';
import { GetCustomPromptRules, SetCustomPromptRules } from '../../../wailsjs/go/main/App';
import { LogInfo as LogInfoRuntime, LogError as LogErrorRuntime } from '../../../wailsjs/runtime/runtime';
import CustomRulesModal from '../CustomRulesModal.vue';

import devTemplateContentFromFile from '../../../../design/prompts/prompt_makeDiffGitFormat.md?raw';
import architectTemplateContentFromFile from '../../../../design/prompts/prompt_makePlan.md?raw';
import findBugTemplateContentFromFile from '../../../../design/prompts/prompt_analyzeBug.md?raw';
import projectManagerTemplateContentFromFile from '../../../../design/prompts/prompt_projectManager.md?raw';

const props = defineProps({
  fileListContext: {
    type: String,
    default: ''
  },
  platform: { // To know if we are on macOS
    type: String,
    default: 'unknown'
  },
  userTask: {
    type: String,
    default: ''
  },
  rulesContent: {
    type: String,
    default: ''
  },
  finalPrompt: {
    type: String,
    default: ''
  },
  selectedTemplate: {
    type: String,
    default: 'dev'
  }
});

const templateContents = {
  dev: devTemplateContentFromFile,
  architect: architectTemplateContentFromFile,
  findBug: findBugTemplateContentFromFile,
  projectManager: projectManagerTemplateContentFromFile
};

watch(() => props.selectedTemplate, () => {
  updateFinalPrompt();
});

const emit = defineEmits(['update:finalPrompt', 'update:userTask', 'update:rulesContent']);

const isLoadingFinalPrompt = ref(false);
const copyButtonText = ref('Copy All');

let finalPromptDebounceTimer = null;
let userTaskInputDebounceTimer = null;

const isPromptRulesModalVisible = ref(false);
const currentPromptRulesForModal = ref('');

const isFirstMount = ref(true);

const localUserTask = ref(props.userTask);

const charCount = computed(() => {
  return (props.finalPrompt || '').length;
});

const approximateTokens = computed(() => {
  const tokens = Math.round(charCount.value / 3);
  return tokens.toString().replace(/\B(?=(\d{3})+(?!\d))/g, " ");
});

const charCountColorClass = computed(() => {
  const count = charCount.value;
  if (count < 1000000) {
    return 'text-success';
  } else if (count <= 4000000) {
    return 'text-warning';
  } else {
    return 'text-error';
  }
});

const tooltipText = computed(() => {
  if (isLoadingFinalPrompt.value) return 'Calculating...';
  
  const count = charCount.value;
  const tokens = Math.round(count / 3);
  return `Your text contains ${count} symbols which is roughly equivalent to ${tokens} tokens`;
});

const DEFAULT_RULES = `no additional rules`;

onMounted(async () => {
  try {
    localUserTask.value = props.userTask;
    // Load rules from the backend only on the first mount
    if (isFirstMount.value) {
      const fetchedRules = await GetCustomPromptRules();
      if (!props.rulesContent) {
        emit('update:rulesContent', fetchedRules);
      }
      isFirstMount.value = false;
    }
  } catch (error) {
    console.error("Failed to load custom prompt rules:", error);
    LogErrorRuntime(`Failed to load custom prompt rules: ${error.message || error}`);
    if (isFirstMount.value && !props.rulesContent) {
      emit('update:rulesContent', DEFAULT_RULES);
    }
    isFirstMount.value = false;
  }

  if (!props.finalPrompt && (props.fileListContext || props.userTask)) {
    debouncedUpdateFinalPrompt();
  }
});

async function updateFinalPrompt() {
  if (isFirstMount.value) {
    isFirstMount.value = false;
    return;
  }

  isLoadingFinalPrompt.value = true;
  await new Promise(resolve => setTimeout(resolve, 100));

  // Get the current template content based on the selected template
  const currentTemplateContent = templateContents[props.selectedTemplate] || templateContents.dev;
  let populatedPrompt = currentTemplateContent;
  
  // Replace placeholders with actual content
  populatedPrompt = populatedPrompt.replace('{TASK}', props.userTask || "No task provided by the user.");
  populatedPrompt = populatedPrompt.replace('{RULES}', props.rulesContent);
  populatedPrompt = populatedPrompt.replace('{FILE_LIST}', props.fileListContext);

  emit('update:finalPrompt', populatedPrompt);
  isLoadingFinalPrompt.value = false;
}

function debouncedUpdateFinalPrompt() {
  clearTimeout(finalPromptDebounceTimer);
  finalPromptDebounceTimer = setTimeout(() => {
    updateFinalPrompt();
  }, 750);
}

watch(() => props.userTask, (newValue) => {
  if (newValue !== localUserTask.value) {
    localUserTask.value = newValue;
  }
  debouncedUpdateFinalPrompt();
});

watch(() => props.selectedTemplate, () => {
  debouncedUpdateFinalPrompt();
});

watch(localUserTask, (currentValue) => {
  clearTimeout(userTaskInputDebounceTimer);
  userTaskInputDebounceTimer = setTimeout(() => {
    if (currentValue !== props.userTask) {
      emit('update:userTask', currentValue);
    }
  }, 300);
});

watch([() => props.userTask, () => props.rulesContent, () => props.fileListContext], () => {
  debouncedUpdateFinalPrompt();
}, { deep: true });

async function copyFinalPromptToClipboard() {
  if (!props.finalPrompt) return;
  try {
    await navigator.clipboard.writeText(props.finalPrompt);
    copyButtonText.value = 'Copied!';
    setTimeout(() => {
      copyButtonText.value = 'Copy All';
    }, 2000);
  } catch (err) {
    console.error('Failed to copy final prompt: ', err);
    if (props.platform === 'darwin' && err) {
      console.error('darvin ClipboardSetText failed for final prompt:', err);
    }
    copyButtonText.value = 'Failed!';
    setTimeout(() => {
      copyButtonText.value = 'Copy All';
    }, 2000);
  }
}

async function openPromptRulesModal() {
  try {
    currentPromptRulesForModal.value = await GetCustomPromptRules();
    isPromptRulesModalVisible.value = true;
  } catch (error) {
    console.error("Error fetching prompt rules for modal:", error);
    LogErrorRuntime(`Error fetching prompt rules for modal: ${error.message || error}`);
    currentPromptRulesForModal.value = props.rulesContent || DEFAULT_RULES;
    isPromptRulesModalVisible.value = true;
  }
}

async function handleSavePromptRules(newRules) {
  try {
    await SetCustomPromptRules(newRules);
    emit('update:rulesContent', newRules);
    isPromptRulesModalVisible.value = false;
    LogInfoRuntime('Custom prompt rules saved successfully.');
  } catch (error) {
    console.error("Error saving prompt rules:", error);
    LogErrorRuntime(`Error saving prompt rules: ${error.message || error}`);
  }
}

function handleCancelPromptRules() {
  isPromptRulesModalVisible.value = false;
}

defineExpose({});
</script>