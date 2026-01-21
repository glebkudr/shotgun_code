<template>
  <div class="p-4 h-full flex flex-col">
    <!-- State 1: No Project Selected -->
    <p v-if="!hasProjectRoots" class="text-xs text-gray-500 mt-2 flex-grow flex justify-center items-center">
      Select a project folder to begin.
    </p>

    <!-- State 2: Project Selected (Always visible container) -->
    <div v-else class="mt-0 flex-grow flex flex-col space-y-4">
      <!-- TOP BLOCK: User Task Input (Never disappears now) -->
      <div>
        <div class="flex items-center justify-between mb-1">
          <label for="user-task-ai-step1" class="block text-sm font-medium text-gray-700">Your task for AI:</label>
          <div class="flex items-center space-x-4">
            <div
              class="flex items-center space-x-3 text-xs text-gray-600"
              title="Repo scan is attached to your context extraction prompt to better understand the repository structure and extract the right context."
            >
              <label class="flex items-center space-x-1 cursor-pointer hover:text-gray-900">
                <input
                  type="checkbox"
                  v-model="includeRepoScan"
                  class="h-3.5 w-3.5 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                />
                <span>Use repo scan:</span>
                <span class="font-mono bg-gray-100 px-1 rounded text-gray-500">{{ repoScanTokensLabel }}</span>
              </label>
              <label v-if="projectRoots.length > 1" class="flex items-center space-x-1">
                <span>Root:</span>
                <select
                  v-model="selectedRepoScanRoot"
                  class="text-xs border border-gray-300 rounded px-1 py-0.5 bg-white"
                >
                  <option v-for="root in projectRoots" :key="root.path" :value="root.path">
                    {{ root.label }}
                  </option>
                </select>
              </label>
              <button
                type="button"
                class="text-blue-600 hover:underline"
                @click="openRepoScanEditor"
              >
                edit
              </button>
            </div>
            <div class="h-4 w-px bg-gray-300"></div>
            <div class="flex items-center space-x-2">
              <button
                class="auto-context-button"
                :class="autoContextButtonClass"
                :disabled="!hasAutoContextPrerequisites"
                data-testid="auto-context-btn"
                @click="handleAutoContextClick"
              >
                <span>
                  {{ props.isAutoContextLoading ? 'Auto selecting…' : 'Auto context' }}
                </span>
              </button>
              <button
                class="text-xs text-blue-600 hover:underline"
                type="button"
                data-testid="setup-api-key-link"
                @click="emit('open-llm-settings')"
              >
                Setup model
              </button>
            </div>
          </div>
        </div>
        <textarea
          id="user-task-ai-step1"
          v-model="localUserTask"
          rows="6"
          class="w-full p-2 border border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 text-sm"
          placeholder="Describe what the AI should do..."
        ></textarea>
      </div>

      <!-- BOTTOM BLOCK: Generated Context (Switches between Loading/Content) -->
      <div class="flex-grow flex flex-col min-h-0">
        <div class="flex items-center justify-between mb-1">
          <h3 class="text-sm font-semibold text-gray-700">Generated Project Context:</h3>

          <!-- Hide controls while loading to prevent interaction -->
          <div
            v-if="!isLoadingContext && generatedContext && !generatedContext.startsWith('Error:')"
            class="flex items-center space-x-3 text-xs"
          >
            <span :class="['font-medium', generatedContextTokensColorClass]">
              {{ generatedContextTokensLabel }}
            </span>
            <button
              @click="copyGeneratedContextToClipboard"
              class="px-3 py-1 bg-gray-200 text-gray-700 text-xs font-semibold rounded-md hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-opacity-50"
            >
              {{ copyButtonText }}
            </button>
          </div>
        </div>

        <!-- Container for Viewer / Loader / Error -->
        <div class="relative flex-grow flex flex-col border border-gray-300 rounded-md bg-white overflow-hidden">
          <!-- 1. Loading State Overlay -->
          <div
            v-if="isLoadingContext"
            class="absolute inset-0 z-10 flex justify-center items-center bg-white bg-opacity-90"
          >
            <div class="text-center">
              <div class="w-64 mx-auto">
                <p class="text-gray-600 mb-1 text-sm">Generating project context...</p>
                <div class="w-full bg-gray-200 rounded-full h-2.5 dark:bg-gray-700">
                  <div
                    class="bg-blue-600 h-2.5 rounded-full transition-all duration-300 ease-out"
                    :style="{ width: progressBarWidth }"
                  ></div>
                </div>
                <p class="text-gray-500 mt-1 text-xs">
                  {{ generationProgress.current }} /
                  {{ generationProgress.total > 0 ? generationProgress.total : 'calculating...' }} items
                </p>
              </div>
            </div>
          </div>

          <!-- 2. Error State -->
          <div
            v-else-if="generatedContext && generatedContext.startsWith('Error:')"
            class="text-red-500 p-3 bg-red-50 flex-grow flex flex-col justify-center items-center h-full"
          >
            <h4 class="font-semibold mb-1">Error Generating Context:</h4>
            <pre
              class="text-xs whitespace-pre-wrap text-left w-full bg-white p-2 border border-red-200 rounded max-h-60 overflow-auto"
            >{{ generatedContext.substring(6).trim() }}</pre>
          </div>

          <!-- 3. Content State -->
          <LargeTextViewer
            v-else-if="generatedContext"
            class="flex-grow h-full"
            :content="generatedContext"
            label=""
            :platform="props.platform"
            placeholder="Context will appear here."
            copy-button-label="Copy All"
            min-height="100%"
            :max-display-length="10000"
            :show-copy-button="false"
            :show-header="false"
            :show-footer="false"
          />

          <!-- 4. Empty/Initial State -->
          <div v-else class="flex-grow flex justify-center items-center bg-gray-50 h-full">
            <p class="text-xs text-gray-500 px-4 text-center">
              Project context will be generated automatically.
              <br />
              If empty after generation, ensure files are selected and not all excluded.
            </p>
          </div>

          <!-- Footer Note (Only visible when content exists and not loading) -->
          <div
            v-if="!isLoadingContext && generatedContext && !generatedContext.startsWith('Error:')"
            class="bg-gray-50 p-1 border-t border-gray-200"
          >
            <p class="text-xs text-gray-500 text-center">
              Preview is truncated for performance. Use Copy All to grab the full text.
            </p>
          </div>
        </div>
      </div>
    </div>

    <RepoScanModal
      :isVisible="isRepoScanModalVisible"
      :initialScan="repoScanContent"
      @save="handleSaveRepoScan"
      @cancel="isRepoScanModalVisible = false"
    />
  </div>
</template>

<script setup>
import { defineProps, ref, computed, defineEmits, watch } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';
import { SaveRepoScan, LoadRepoScan, CountTokens } from '../../../wailsjs/go/main/App';
import RepoScanModal from '../RepoScanModal.vue';
import LargeTextViewer from '../common/LargeTextViewer.vue';

const props = defineProps({
  generatedContext: {
    type: String,
    default: ''
  },
  generatedContextTokens: {
    type: Number,
    default: null
  },
  projectRoots: {
    type: Array,
    default: () => []
  },
  isLoadingContext: {
    type: Boolean,
    default: false
  },
  generationProgress: {
    type: Object,
    default: () => ({ current: 0, total: 0 })
  },
  platform: {
    type: String,
    default: 'unknown'
  },
  hasActiveLlmKey: {
    type: Boolean,
    default: false
  },
  isAutoContextLoading: {
    type: Boolean,
    default: false
  },
  userTask: {
    type: String,
    default: ''
  }
});

const emit = defineEmits(['auto-context', 'open-llm-settings', 'update:userTask']);

const hasProjectRoots = computed(() => Array.isArray(props.projectRoots) && props.projectRoots.length > 0);
const selectedRepoScanRoot = ref('');

const progressBarWidth = computed(() => {
  if (props.generationProgress && props.generationProgress.total > 0) {
    const percentage = (props.generationProgress.current / props.generationProgress.total) * 100;
    return `${Math.min(100, Math.max(0, percentage))}%`;
  }
  return '0%';
});

const copyButtonText = ref('Copy All');
const localUserTask = ref(props.userTask);
let userTaskInputDebounceTimer = null;

const includeRepoScan = ref(false);
const repoScanTokenCount = ref(null);
const repoScanContent = ref('');
const isRepoScanModalVisible = ref(false);

const repoScanTokensLabel = computed(() => {
  if (!repoScanContent.value) {
    return 'empty';
  }
  if (typeof repoScanTokenCount.value !== 'number') {
    return 'n/a';
  }
  return `${formatTokenCount(repoScanTokenCount.value)} tokens`;
});

const generatedContextTokensLabel = computed(() => {
  if (!props.generatedContext) {
    return 'empty';
  }
  if (typeof props.generatedContextTokens !== 'number') {
    return 'n/a';
  }
  return `${formatTokenCount(props.generatedContextTokens)} tokens`;
});

const generatedContextTokensColorClass = computed(() => {
  const count = props.generatedContextTokens;
  if (typeof count !== 'number' || count <= 0) {
    return 'text-gray-500';
  }
  if (count < 1000000) {
    return 'text-green-600';
  } else if (count <= 4000000) {
    return 'text-yellow-500';
  }
  return 'text-red-600';
});

const hasAutoContextPrerequisites = computed(() => {
  if (!props.hasActiveLlmKey) {
    return false;
  }
  if (!localUserTask.value) {
    return false;
  }
  return localUserTask.value.trim().length > 0;
});

const autoContextButtonClass = computed(() => {
  if (!hasAutoContextPrerequisites.value) {
    return 'auto-context-button--disabled';
  }
  if (props.isAutoContextLoading) {
    return 'auto-context-button--in-progress';
  }
  return 'auto-context-button--enabled';
});

watch(
  () => props.userTask,
  (newValue) => {
    if (newValue !== localUserTask.value) {
      localUserTask.value = newValue;
    }
  }
);

watch(localUserTask, (currentValue) => {
  if (userTaskInputDebounceTimer) {
    clearTimeout(userTaskInputDebounceTimer);
  }
  userTaskInputDebounceTimer = setTimeout(() => {
    if (currentValue !== props.userTask) {
      emit('update:userTask', currentValue);
    }
  }, 300);
});

watch(
  () => props.projectRoots,
  (newRoots) => {
    if (!Array.isArray(newRoots) || newRoots.length === 0) {
      selectedRepoScanRoot.value = '';
      repoScanContent.value = '';
      includeRepoScan.value = false;
      repoScanTokenCount.value = 0;
      return;
    }
    const current = selectedRepoScanRoot.value;
    const stillAvailable = newRoots.some((root) => root.path === current);
    if (!current || !stillAvailable) {
      selectedRepoScanRoot.value = newRoots[0].path;
    }
  },
  { immediate: true, deep: true }
);

// Watch selected repo scan root to load repo scan
watch(
  () => selectedRepoScanRoot.value,
  async (newRoot) => {
    if (!newRoot) {
      repoScanContent.value = '';
      includeRepoScan.value = false;
      repoScanTokenCount.value = 0;
      return;
    }

    try {
      const content = await LoadRepoScan(newRoot);
      if (content) {
        repoScanContent.value = content;
        includeRepoScan.value = true;
        await updateTokenCount(content);
        return;
      }

      repoScanContent.value = '';
      includeRepoScan.value = false;
      repoScanTokenCount.value = 0;
    } catch (err) {
      console.error('Failed to load repo scan:', err);
      repoScanContent.value = '';
      includeRepoScan.value = false;
      repoScanTokenCount.value = 0;
    }
  },
  { immediate: true }
);

function formatTokenCount(value) {
  return value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

let repoScanTokenRequestId = 0;
async function updateTokenCount(text) {
  const requestId = ++repoScanTokenRequestId;
  if (!text) {
    repoScanTokenCount.value = 0;
    return;
  }
  try {
    const count = await CountTokens(text);
    if (requestId === repoScanTokenRequestId) {
      repoScanTokenCount.value = count;
    }
  } catch (err) {
    console.error('Failed to count repo scan tokens:', err);
    if (requestId === repoScanTokenRequestId) {
      repoScanTokenCount.value = null;
    }
  }
}

async function copyGeneratedContextToClipboard() {
  if (!props.generatedContext) {
    return;
  }

  // Use navigator.clipboard.writeText as primary (WailsClipboardSetText has UTF-8 encoding issues with box-drawing chars on darwin)
  try {
    await navigator.clipboard.writeText(props.generatedContext);
    copyButtonText.value = 'Copied!';
    resetContextCopyLabel();
    return;
  } catch (err) {
    console.error('Failed to copy context preview: ', err);
  }

  // Fallback to Wails clipboard API
  try {
    await WailsClipboardSetText(props.generatedContext);
    copyButtonText.value = 'Copied!';
  } catch (fallbackErr) {
    console.error('Fallback clipboard copy also failed for context:', fallbackErr);
    copyButtonText.value = 'Failed!';
  } finally {
    resetContextCopyLabel();
  }
}

function resetContextCopyLabel() {
  setTimeout(() => {
    copyButtonText.value = 'Copy All';
  }, 2000);
}

function openRepoScanEditor() {
  isRepoScanModalVisible.value = true;
}

function handleAutoContextClick() {
  if (!hasAutoContextPrerequisites.value) {
    return;
  }
  if (props.isAutoContextLoading) {
    return;
  }
  emit('auto-context');
}

async function handleSaveRepoScan(content) {
  repoScanContent.value = content;
  await updateTokenCount(content);
  isRepoScanModalVisible.value = false;

  const rootPath = selectedRepoScanRoot.value;
  if (content && rootPath) {
    try {
      await SaveRepoScan(rootPath, content);
      includeRepoScan.value = true;
    } catch (err) {
      console.error('Failed to save repo scan:', err);
    }
    return;
  }
  if (!content && rootPath) {
    try {
      await SaveRepoScan(rootPath, '');
      includeRepoScan.value = false;
    } catch (err) {
      console.error('Failed to clear repo scan:', err);
    }
  }
}
</script>


