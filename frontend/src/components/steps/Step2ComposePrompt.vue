<template>
  <div class="p-4 h-full flex flex-col relative">
    
    <CustomRulesModal
      :is-visible="isPromptRulesModalVisible"
      :initial-rules="currentPromptRulesForModal"
      title="Edit Custom Prompt Rules"
      ruleType="prompt"
      @save="handleSavePromptRules"
      @cancel="handleCancelPromptRules"
    />

    <!-- Top Bar: Instruction or Execute Button -->
    <div class="flex items-center justify-end mb-4 space-x-4 min-h-[32px]">
        <div class="flex items-center space-x-2">
            <button
                class="auto-context-button"
                :class="executeButtonClass"
                :disabled="!hasExecutePrerequisites"
                @click="handleExecutePrompt"
                title="Execute prompt with configured LLM"
            >
                <span>
                    {{ isExecuting ? 'Executing...' : 'Execute Prompt' }}
                </span>
            </button>
            <button
                class="text-xs text-blue-600 hover:underline"
                type="button"
                @click="emit('open-llm-settings')"
            >
                Setup model
            </button>
        </div>
    </div>

    <div class="flex-grow flex flex-row space-x-4 overflow-hidden">
      <!-- Left Column: Task, Rules, Files -->
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

        <LargeTextViewer
          label="Files to include:"
          :content="props.fileListContext"
          :tokens-exact="props.fileListContextTokens"
          placeholder="File list from Step 1 (Prepare Context) will appear here..."
          :platform="props.platform"
          min-height="200px"
          :max-display-length="10000"
          copy-button-label="Copy All"
        />
      </div>

      <!-- Right Column: Final Prompt -->
      <div class="w-1/2 flex flex-col overflow-y-auto p-2 border border-gray-200 rounded-md bg-white relative">
        <div class="flex items-center justify-between gap-2 mb-2 flex-shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <h3 class="text-md font-medium text-gray-700 whitespace-nowrap">Prompt:</h3>
            <!-- Small Loading Indicator next to title instead of destroying content -->
            <div v-if="isLoadingFinalPrompt" class="animate-spin rounded-full h-3 w-3 border-b-2 border-blue-500 flex-shrink-0"></div>
            <select
              v-model="selectedPromptTemplateKey"
              class="p-1 border border-gray-300 rounded-md text-xs focus:ring-blue-500 focus:border-blue-500 flex-shrink-0"
              title="Select prompt template"
            >
              <option v-for="(template, key) in promptTemplates" :key="key" :value="key">
                {{ template.name }}
              </option>
            </select>
          </div>
          <button
            @click="copyFinalPromptToClipboard"
            :disabled="!props.finalPrompt || isLoadingFinalPrompt"
            class="px-3 py-1 bg-blue-500 text-white text-xs font-semibold rounded-md hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 disabled:bg-gray-300 whitespace-nowrap flex-shrink-0"
          >
            {{ copyButtonText }}
          </button>
        </div>
        <!-- 
           MODIFIED: 
           1. Removed v-if/v-else switching.
           2. Added transition classes to LargeTextViewer container.
           3. Added overlay class for loading state.
        -->
        <div class="flex flex-col flex-grow relative">
          <div 
            class="flex-grow transition-opacity duration-200 ease-in-out"
            :class="{ 'opacity-50 grayscale': isLoadingFinalPrompt }"
          >
            <LargeTextViewer
              class="flex-grow h-full"
              :content="props.finalPrompt"
              label="Generated prompt preview"
              placeholder="The final prompt will be generated here..."
              :platform="props.platform"
              min-height="300px"
              max-height="1000px"
              :max-display-length="15000"
              :show-copy-button="false"
            />
          </div>
          <p class="text-xs text-gray-500 mt-1">
            Preview is truncated for performance. Use Copy All to grab the full text.
          </p>
        </div>
      </div>
    </div>

    <!-- Response Modal -->
    <div v-if="isResponseModalVisible" class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
        <div class="bg-white rounded-lg p-6 w-[90%] h-[90%] flex flex-col shadow-xl">
            <div class="flex justify-between items-center mb-4">
                <h3 class="text-lg font-semibold">LLM Response</h3>
                <button @click="closeResponseModal" class="text-gray-500 hover:text-gray-700 text-2xl">&times;</button>
            </div>
            <textarea 
                readonly 
                class="flex-grow p-4 border border-gray-300 rounded-md font-mono text-sm mb-4 resize-none bg-gray-50 focus:outline-none" 
                :value="currentResponse"
            ></textarea>
            <div class="flex justify-end space-x-3">
                 <button 
                    @click="copyResponse" 
                    class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
                 >
                    {{ copyResponseButtonText }}
                 </button>
                 <button 
                    @click="closeResponseModal" 
                    class="px-4 py-2 bg-gray-200 text-gray-800 rounded hover:bg-gray-300 transition-colors"
                 >
                    Close
                 </button>
            </div>
        </div>
    </div>

  </div>
</template>

<script setup>
import { ref, watch, onMounted, computed } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';
import { GetCustomPromptRules, SetCustomPromptRules, ExecuteLLMPrompt } from '../../../wailsjs/go/main/App';
import { LogInfo as LogInfoRuntime, LogError as LogErrorRuntime } from '../../../wailsjs/runtime/runtime';
import CustomRulesModal from '../CustomRulesModal.vue';
import LargeTextViewer from '../common/LargeTextViewer.vue';

import devTemplateContentFromFile from '../../../../design/prompts/prompt_makeDiffGitFormat.md?raw';
import architectTemplateContentFromFile from '../../../../design/prompts/prompt_makePlan.md?raw';
import findBugTemplateContentFromFile from '../../../../design/prompts/prompt_analyzeBug.md?raw';
import projectManagerTemplateContentFromFile from '../../../../design/prompts/prompt_projectManager.md?raw';

const props = defineProps({
  fileListContext: {
    type: String,
    default: ''
  },
  fileListContextTokens: {
    type: Number,
    default: null
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
  hasActiveLlmKey: {
    type: Boolean,
    default: false
  }
});

const emit = defineEmits(['update:finalPrompt', 'update:userTask', 'update:rulesContent', 'open-llm-settings']);

const promptTemplates = {
  architect: { name: 'Architect', content: architectTemplateContentFromFile },
  findBug: { name: 'Test', content: findBugTemplateContentFromFile },
  dev: { name: 'Dev', content: devTemplateContentFromFile },
  // architect duplicate removed
  projectManager: { name: 'Project: Update Tasks', content: projectManagerTemplateContentFromFile },
};

const selectedPromptTemplateKey = ref('architect'); // Default template

const isLoadingFinalPrompt = ref(false);
const copyButtonText = ref('Copy All');

let finalPromptDebounceTimer = null;
let userTaskInputDebounceTimer = null;

// Modal state for prompt rules
const isPromptRulesModalVisible = ref(false);
const currentPromptRulesForModal = ref('');

// Response Modal State
const isResponseModalVisible = ref(false);
const currentResponse = ref('');
const isExecuting = ref(false);
const copyResponseButtonText = ref('Copy Response');

const isFirstMount = ref(true);

const localUserTask = ref(props.userTask);

const hasExecutePrerequisites = computed(() => {
  if (!props.hasActiveLlmKey) {
    return false;
  }
  if (!localUserTask.value) {
    return false;
  }
  return localUserTask.value.trim().length > 0;
});

const executeButtonClass = computed(() => {
  if (!hasExecutePrerequisites.value) {
    return 'auto-context-button--disabled';
  }
  if (isExecuting.value) {
    return 'auto-context-button--in-progress';
  }
  return 'auto-context-button--enabled';
});

const DEFAULT_RULES = `пиши по-русски`;

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

  // MODIFIED: Removed the artificial delay (await new Promise...) 
  // to make the update instant and smoother. 
  // The debounce on input is enough to prevent performance issues.
  try {
    const currentTemplateContent = promptTemplates[selectedPromptTemplateKey.value].content;
    let populatedPrompt = currentTemplateContent;
    populatedPrompt = populatedPrompt.replace('{TASK}', props.userTask || "No task provided by the user.");
    populatedPrompt = populatedPrompt.replace('{RULES}', props.rulesContent);
    populatedPrompt = populatedPrompt.replace('{FILE_STRUCTURE}', props.fileListContext || "No file structure context provided.");

    // Insert current date in YYYY-MM-DD format
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    const currentDate = `${yyyy}-${mm}-${dd}`;
    populatedPrompt = populatedPrompt.replaceAll('{CURRENT_DATE}', currentDate);

    emit('update:finalPrompt', populatedPrompt);
  } finally {
    isLoadingFinalPrompt.value = false;
  }
}

function debouncedUpdateFinalPrompt() {
  // Set loading to true immediately to show "working" state via opacity opacity/spinner
  isLoadingFinalPrompt.value = true;

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

  // Use navigator.clipboard.writeText as primary (WailsClipboardSetText has UTF-8 encoding issues with box-drawing chars on darwin)
  try {
    await navigator.clipboard.writeText(props.finalPrompt);
    copyButtonText.value = 'Copied!';
    resetCopyButtonLabel();
    return;
  } catch (err) {
    console.error('Failed to copy final prompt: ', err);
  }

  // Fallback to Wails clipboard API
  try {
    await WailsClipboardSetText(props.finalPrompt);
    copyButtonText.value = 'Copied!';
  } catch (fallbackErr) {
    console.error('Fallback copy attempt for final prompt also failed: ', fallbackErr);
    copyButtonText.value = 'Failed!';
  } finally {
    resetCopyButtonLabel();
  }
}

function resetCopyButtonLabel() {
  setTimeout(() => {
    copyButtonText.value = 'Copy All';
  }, 2000);
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

async function handleExecutePrompt() {
    if (!hasExecutePrerequisites.value) {
        return;
    }
    if (isExecuting.value) {
        return;
    }
    
    isExecuting.value = true;
    LogInfoRuntime('Executing LLM prompt...');
    try {
        const result = await ExecuteLLMPrompt(localUserTask.value, props.finalPrompt);
        if (result && result.response) {
            currentResponse.value = result.response;
            isResponseModalVisible.value = true;
            LogInfoRuntime('LLM Execution successful.');
        } else {
             throw new Error("Received empty response from backend.");
        }
    } catch (err) {
        console.error("Error executing prompt:", err);
        LogErrorRuntime(`Error executing prompt: ${err.message || err}`);
        // Optionally show error in a toast or the modal
        currentResponse.value = `Error: ${err.message || err}`;
        isResponseModalVisible.value = true;
    } finally {
        isExecuting.value = false;
    }
}

function closeResponseModal() {
    isResponseModalVisible.value = false;
    currentResponse.value = '';
}

async function copyResponse() {
    if (!currentResponse.value) return;
    try {
        await navigator.clipboard.writeText(currentResponse.value);
        copyResponseButtonText.value = 'Copied!';
        setTimeout(() => {
            copyResponseButtonText.value = 'Copy Response';
        }, 2000);
    } catch (err) {
        console.error('Failed to copy response:', err);
        copyResponseButtonText.value = 'Failed!';
         setTimeout(() => {
            copyResponseButtonText.value = 'Copy Response';
        }, 2000);
    }
}

defineExpose({});
</script>
