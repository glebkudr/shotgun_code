<template>
  <div class="p-4 h-full flex flex-col">
    <p class="text-gray-700 mb-4 text-center text-sm">
      Write the task for the LLM in the central column and copy the final prompt
    </p>

    <CustomRulesModal
      :is-visible="isPromptRulesModalVisible"
      :initial-rules="currentPromptRulesForModal"
      title="Edit Custom Prompt Rules"
      ruleType="prompt"
      @save="handleSavePromptRules"
      @cancel="handleCancelPromptRules"
    />

    <div class="flex-grow flex flex-row space-x-4 overflow-hidden">
      <div class="w-1/2 flex flex-col space-y-3 overflow-y-auto p-2 border border-gray-200 rounded-md bg-gray-50">
        <div>
          <label for="user-task-ai" class="block text-sm font-medium text-gray-700 mb-1">Your task for AI:</label>
          <textarea
            id="user-task-ai"
            v-model="localUserTask"
            rows="15"
            class="w-full p-2 border border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 text-sm"
            placeholder="Describe what the AI should do..."
          ></textarea>
        </div>

        <div>
          <label for="rules-content" class="block text-sm font-medium text-gray-700 mb-1 flex items-center">
            Custom rules:
            <button @click="openPromptRulesModal" title="Edit custom prompt rules" class="ml-2 p-0.5 hover:bg-gray-200 rounded text-xs">⚙️</button>
          </label>
          <textarea
            id="rules-content"
            :value="rulesContent"
            @input="e => emit('update:rulesContent', e.target.value)"
            rows="8"
            class="w-full p-2 border border-gray-300 rounded-md shadow-sm bg-gray-100 text-sm font-mono"
            placeholder="Rules for AI..."
          ></textarea>
        </div>

        <div>
          <label for="file-list-context" class="block text-sm font-medium text-gray-700 mb-1">Files to include:</label>
          <textarea
            id="file-list-context"
            :value="props.fileListContext"
            rows="20"
            readonly
            class="w-full p-2 border border-gray-300 rounded-md shadow-sm bg-gray-100 font-mono text-xs"
            placeholder="File list from Step 1 (Prepare Context) will appear here..."
            style="min-height: 150px;"
          ></textarea>
        </div>
      </div>

      <div class="w-1/2 flex flex-col overflow-y-auto p-2 border border-gray-200 rounded-md bg-white">
        <div class="flex justify-between items-center mb-2">
          <div class="flex items-center space-x-2">
            <h3 class="text-md font-medium text-gray-700">Prompt:</h3>
            <select
              v-model="selectedPromptTemplateKey"
              class="ml-2 p-1 border border-gray-300 rounded-md text-xs focus:ring-blue-500 focus:border-blue-500"
              :disabled="isLoadingFinalPrompt"
              title="Select prompt template"
            >
              <option v-for="(template, key) in promptTemplates" :key="key" :value="key">
                {{ template.name }}
              </option>
            </select>
          </div>
          <div class="flex items-center space-x-3">
            <span
              v-show="!isLoadingFinalPrompt"
              :class="['text-xs font-medium', charCountColorClass]"
              :title="tooltipText"
            >
              ~{{ approximateTokens }} tokens
            </span>
            <button
              @click="copyFinalPromptToClipboard"
              :disabled="!props.finalPrompt || isLoadingFinalPrompt"
              class="px-3 py-1 bg-blue-500 text-white text-xs font-semibold rounded-md hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 disabled:bg-gray-300"
            >
              {{ copyButtonText }}
            </button>
          </div>
        </div>

        <div v-if="isLoadingFinalPrompt" class="flex-grow flex justify-center items-center">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
          <p class="text-gray-500 ml-2">Updating prompt...</p>
        </div>

        <textarea
          v-else
          :value="props.finalPrompt"
          @input="e => emit('update:finalPrompt', e.target.value)"
          rows="20"
          class="w-full p-2 border border-gray-300 rounded-md shadow-sm font-mono text-xs flex-grow"
          placeholder="The final prompt will be generated here..."
          style="min-height: 300px;"
        ></textarea>
         <p class="text-xs text-gray-500 mt-1">
            The prompt updates automatically. Manual changes to this field may be overwritten when source data (task, rules, file list) is updated.
        </p>
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
  }
});

const emit = defineEmits(['update:finalPrompt', 'update:userTask', 'update:rulesContent']);

const promptTemplates = {
  dev: { name: 'Dev', content: devTemplateContentFromFile },
  architect: { name: 'Architect', content: architectTemplateContentFromFile },
  findBug: { name: 'Find Bug', content: findBugTemplateContentFromFile },
};

const selectedPromptTemplateKey = ref('dev'); // Default template

const isLoadingFinalPrompt = ref(false);
const copyButtonText = ref('Copy All');

let finalPromptDebounceTimer = null;
let userTaskInputDebounceTimer = null;

// Modal state for prompt rules
const isPromptRulesModalVisible = ref(false);
const currentPromptRulesForModal = ref('');

const isFirstMount = ref(true);

const localUserTask = ref(props.userTask);

// Character count and related computed properties
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
    return 'text-green-600';
  } else if (count <= 4000000) {
    return 'text-yellow-500'; // Using 500 for better visibility on white bg
  } else {
    return 'text-red-600';
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
  isLoadingFinalPrompt.value = true;
  await new Promise(resolve => setTimeout(resolve, 100));

  const currentTemplateContent = promptTemplates[selectedPromptTemplateKey.value].content;
  let populatedPrompt = currentTemplateContent;
  populatedPrompt = populatedPrompt.replace('{TASK}', props.userTask || "No task provided by the user.");
  populatedPrompt = populatedPrompt.replace('{RULES}', props.rulesContent);
  populatedPrompt = populatedPrompt.replace('{FILE_STRUCTURE}', props.fileListContext || "No file structure context provided.");

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
});

watch(localUserTask, (currentValue) => {
  clearTimeout(userTaskInputDebounceTimer);
  userTaskInputDebounceTimer = setTimeout(() => {
    if (currentValue !== props.userTask) {
      emit('update:userTask', currentValue);
    }
  }, 300);
});

watch([() => props.userTask, () => props.rulesContent, () => props.fileListContext, selectedPromptTemplateKey], () => {
  debouncedUpdateFinalPrompt();
}, { deep: true });

watch(selectedPromptTemplateKey, () => {
  LogInfoRuntime(`Prompt template changed to: ${promptTemplates[selectedPromptTemplateKey.value].name}. Updating final prompt.`);
  debouncedUpdateFinalPrompt();
});

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