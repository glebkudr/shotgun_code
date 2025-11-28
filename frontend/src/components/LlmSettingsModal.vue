<template>
  <div
    v-if="isVisible"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50"
    @click.self="handleCancel"
  >
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-xl w-full max-w-xl p-6">
      <div class="flex items-start justify-between mb-4">
        <h2 class="text-xl font-semibold text-gray-800 dark:text-gray-100">LLM Settings</h2>
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-600 dark:text-gray-300">Theme</span>
          <button
            type="button"
            @click="toggleTheme"
            class="px-2 py-1 rounded-md border border-gray-300 dark:border-slate-600 text-sm bg-white dark:bg-slate-700 text-gray-800 dark:text-gray-100"
          >
            {{ themeIsDark ? 'Dark' : 'Light' }}
          </button>
        </div>
      </div>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1" for="provider-select">Provider</label>
        <select
          id="provider-select"
          v-model="localProvider"
          @change="handleProviderChange"
          class="w-full border rounded-md p-2 text-sm bg-white dark:bg-slate-700 border-gray-300 dark:border-slate-600 text-gray-900 dark:text-gray-100"
          data-testid="provider-select"
        >
          <option v-for="option in providerOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </div>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1" for="api-key-input">API Key</label>
        <input
          id="api-key-input"
          type="password"
          v-model="localApiKeys[localProvider]"
          placeholder="Paste the API key for the selected provider"
          class="w-full border rounded-md p-2 text-sm bg-white dark:bg-slate-700 border-gray-300 dark:border-slate-600 text-gray-900 dark:text-gray-100"
          data-testid="api-key-input"
        />
        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Keys are stored locally inside the Shotgun settings file.</p>
      </div>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1" for="base-url-input">
          Custom Base URL (optional)
        </label>
        <input
          id="base-url-input"
          type="text"
          v-model="localBaseUrl"
          placeholder="https://example.com/v1"
          class="w-full border rounded-md p-2 text-sm bg-white dark:bg-slate-700 border-gray-300 dark:border-slate-600 text-gray-900 dark:text-gray-100"
        />
      </div>

      <div class="mb-4">
        <div class="flex justify-between items-center">
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-200" for="model-input">Model</label>
          <button
            class="text-xs text-blue-600 dark:text-blue-400 hover:underline disabled:text-gray-400"
            :disabled="isLoadingModels"
            @click="fetchModels"
          >
            {{ isLoadingModels ? 'Loading...' : 'Refresh models' }}
          </button>
        </div>
        <input
          id="model-input"
          type="text"
          v-model="localModel"
          placeholder="Type a model name"
          class="w-full border rounded-md p-2 text-sm bg-white dark:bg-slate-700 border-gray-300 dark:border-slate-600 text-gray-900 dark:text-gray-100"
          :disabled="isLoadingModels"
          data-testid="model-select"
        />
        <div
          v-if="filteredModelSuggestions.length"
          class="mt-2 border rounded-md max-h-40 overflow-y-auto divide-y"
          :class="['border-gray-200 dark:border-slate-700', 'divide-gray-100 dark:divide-slate-700']"
        >
          <button
            v-for="model in filteredModelSuggestions"
            :key="model"
            type="button"
            class="w-full text-left px-3 py-2 text-sm hover:bg-gray-50 dark:hover:bg-slate-700"
            @click="selectSuggestion(model)"
          >
            {{ model }}
          </button>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Start typing to narrow down the suggestions or enter any custom value.</p>
      </div>

      <p v-if="errorMessage" class="text-red-600 text-sm mb-4 whitespace-pre-wrap">{{ errorMessage }}</p>

      <div class="flex justify-end space-x-2">
        <button
          class="px-4 py-2 rounded-md border text-sm border-gray-300 dark:border-slate-600 text-gray-700 dark:text-gray-200"
          @click="handleCancel"
          data-testid="cancel-btn"
        >
          Cancel
        </button>
        <button
          class="px-4 py-2 rounded-md text-white text-sm"
          :class="isSaving ? 'bg-blue-400' : 'bg-blue-600 hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600'"
          :disabled="isSaving"
          @click="handleSave"
          data-testid="save-btn"
        >
          {{ isSaving ? 'Saving…' : 'Save' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch, onMounted } from 'vue';
import {
  ListLlmModels,
  SetLlmApiKey,
  SetLlmBaseURL,
  SetLlmModel,
  SetLlmProvider,
} from '../../wailsjs/go/main/App';

const props = defineProps({
  isVisible: {
    type: Boolean,
    default: false,
  },
  initialSettings: {
    type: Object,
    default: () => ({}),
  },
});

const emit = defineEmits(['close', 'saved']);

const providerOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'openrouter', label: 'OpenRouter' },
  { value: 'gemini', label: 'Google Gemini' },
];

const providerDefaultModels = {
  openai: 'gpt-5',
  openrouter: 'openai/gpt-5',
  gemini: 'gemini-2.5-pro',
};

const localProvider = ref('openai');
const localModel = ref('');
const localBaseUrl = ref('');
const localApiKeys = reactive({
  openai: '',
  openrouter: '',
  gemini: '',
});

const modelOptions = ref([]);
const isLoadingModels = ref(false);
const isSaving = ref(false);
const errorMessage = ref('');

// Theme state for the modal toggle. We keep this local but authoritative state
// is the presence of the `dark` class on `document.documentElement` and
// `localStorage['shotgun_theme']`. A global helper `window.toggleShotgunTheme`
// is defined in `frontend/src/main.js` and used to toggle/persist the theme.
const themeIsDark = ref(false);

function initThemeState() {
  try {
    const saved = localStorage.getItem('shotgun_theme');
    if (saved === 'dark') {
      themeIsDark.value = true;
    } else if (saved === 'light') {
      themeIsDark.value = false;
    } else {
      // fallback to checking DOM class
      themeIsDark.value = document.documentElement.classList.contains('dark');
    }
  } catch (e) {
    themeIsDark.value = document.documentElement.classList.contains('dark');
  }
}

onMounted(() => {
  initThemeState();
});

const activeKey = computed(() => localApiKeys[localProvider.value] || '');
const filteredModelSuggestions = computed(() => {
  const query = (localModel.value || '').trim().toLowerCase();
  return modelOptions.value.filter((option) => {
    const normalizedOption = (option || '').toLowerCase();
    if (!normalizedOption) {
      return false;
    }
    if (query && normalizedOption === query) {
      return false;
    }
    if (!query) {
      return true;
    }
    return normalizedOption.includes(query);
  });
});

function syncStateFromProps() {
  const settings = props.initialSettings || {};
  localProvider.value = settings.activeProvider || 'openai';
  localModel.value = settings.model || providerDefaultModels[localProvider.value] || '';
  localBaseUrl.value = settings.baseURL || '';
  localApiKeys.openai = settings.openAIKey || '';
  localApiKeys.openrouter = settings.openRouterKey || '';
  localApiKeys.gemini = settings.geminiKey || '';
  modelOptions.value = [];
  errorMessage.value = '';
}

watch(
  () => props.initialSettings,
  () => {
    syncStateFromProps();
  },
  { immediate: true }
);

watch(
  () => props.isVisible,
  (visible) => {
    if (visible) {
      syncStateFromProps();
      fetchModels();
    } else {
      modelOptions.value = [];
      errorMessage.value = '';
    }
  }
);

function handleProviderChange() {
  errorMessage.value = '';
  modelOptions.value = [];
  localModel.value = providerDefaultModels[localProvider.value] || '';
  fetchModels();
}

async function fetchModels() {
  if (!localProvider.value) {
    modelOptions.value = [];
    return;
  }
  isLoadingModels.value = true;
  errorMessage.value = '';
  try {
    const response = await ListLlmModels(localProvider.value);
    const names = Array.isArray(response) ? response.map((m) => m.name || m.Name || '').filter(Boolean) : [];
    modelOptions.value = names;
    if (!localModel.value && names.length) {
      localModel.value = names[0];
    }
  } catch (err) {
    errorMessage.value = `Failed to load models: ${err?.message || err}`;
  } finally {
    isLoadingModels.value = false;
  }
}

async function handleSave() {
  if (!activeKey.value) {
    errorMessage.value = 'API key is required.';
    return;
  }
  if (!localModel.value) {
    localModel.value = providerDefaultModels[localProvider.value] || '';
  }

  isSaving.value = true;
  errorMessage.value = '';
  try {
    await SetLlmApiKey(localProvider.value, activeKey.value);
    await SetLlmBaseURL(localBaseUrl.value || '');
    await SetLlmProvider(localProvider.value);
    await SetLlmModel(localProvider.value, localModel.value);
    emit('saved');
    emit('close');
  } catch (err) {
    errorMessage.value = err?.message || `${err}`;
  } finally {
    isSaving.value = false;
  }
}

function handleCancel() {
  emit('close');
}

function selectSuggestion(value) {
  localModel.value = value;
}

function toggleTheme() {
  try {
    // Prefer using the global helper defined in main.js
    if (typeof window.toggleShotgunTheme === 'function') {
      const res = window.toggleShotgunTheme();
      if (res !== null && typeof res !== 'undefined') {
        themeIsDark.value = !!res;
        return;
      }
    }
    // Fallback: toggle class and persist
    const isNowDark = document.documentElement.classList.toggle('dark');
    try { localStorage.setItem('shotgun_theme', isNowDark ? 'dark' : 'light'); } catch (e) {}
    themeIsDark.value = isNowDark;
  } catch (err) {
    // ignore
  }
}
</script>
