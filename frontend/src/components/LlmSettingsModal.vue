<template>
  <div
    v-if="isVisible"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50"
    @click.self="handleCancel"
  >
    <div class="bg-white rounded-lg shadow-xl w-full max-w-xl p-6">
      <h2 class="text-xl font-semibold text-gray-800 mb-4">LLM Settings</h2>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 mb-1" for="provider-select">Provider</label>
        <select
          id="provider-select"
          v-model="localProvider"
          @change="handleProviderChange"
          class="w-full border border-gray-300 rounded-md p-2 text-sm"
          data-testid="provider-select"
        >
          <option v-for="option in providerOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </div>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 mb-1" for="api-key-input">API Key</label>
        <input
          id="api-key-input"
          type="password"
          v-model="localApiKeys[localProvider]"
          placeholder="Paste the API key for the selected provider"
          class="w-full border border-gray-300 rounded-md p-2 text-sm"
          data-testid="api-key-input"
        />
        <p class="text-xs text-gray-500 mt-1">Keys are stored locally inside the Shotgun settings file.</p>
      </div>

      <div class="mb-4">
        <label class="block text-sm font-medium text-gray-700 mb-1" for="base-url-input">
          Custom Base URL (optional)
        </label>
        <input
          id="base-url-input"
          type="text"
          v-model="localBaseUrl"
          placeholder="https://example.com/v1"
          class="w-full border border-gray-300 rounded-md p-2 text-sm"
        />
      </div>

      <div class="mb-4">
        <div class="flex justify-between items-center">
          <label class="block text-sm font-medium text-gray-700" for="model-input">Model</label>
          <button
            class="text-xs text-blue-600 hover:underline disabled:text-gray-400"
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
          class="w-full border border-gray-300 rounded-md p-2 text-sm"
          :disabled="isLoadingModels"
          data-testid="model-select"
        />
        <div
          v-if="filteredModelSuggestions.length"
          class="mt-2 border border-gray-200 rounded-md max-h-40 overflow-y-auto divide-y divide-gray-100"
        >
          <button
            v-for="model in filteredModelSuggestions"
            :key="model"
            type="button"
            class="w-full text-left px-3 py-2 text-sm hover:bg-gray-50"
            @click="selectSuggestion(model)"
          >
            {{ model }}
          </button>
        </div>
        <p class="text-xs text-gray-500 mt-1">Start typing to narrow down the suggestions or enter any custom value.</p>
      </div>

      <p v-if="errorMessage" class="text-red-600 text-sm mb-4 whitespace-pre-wrap">{{ errorMessage }}</p>

      <div class="flex justify-end space-x-2">
        <button
          class="px-4 py-2 rounded-md border border-gray-300 text-gray-700 text-sm"
          @click="handleCancel"
          data-testid="cancel-btn"
        >
          Cancel
        </button>
        <button
          class="px-4 py-2 rounded-md text-white text-sm"
          :class="isSaving ? 'bg-blue-400' : 'bg-blue-600 hover:bg-blue-700'"
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
import { computed, reactive, ref, watch } from 'vue';
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
  { value: 'requesty', label: 'Requesty' },
  { value: 'gemini', label: 'Google Gemini' },
];

const providerDefaultModels = {
  openai: 'gpt-5',
  openrouter: 'openai/gpt-5',
  requesty: 'openai/gpt-4o-mini',
  gemini: 'gemini-2.5-pro',
};

const localProvider = ref('openai');
const localModel = ref('');
const localBaseUrl = ref('');
const localApiKeys = reactive({
  openai: '',
  openrouter: '',
  requesty: '',
  gemini: '',
});

const modelOptions = ref([]);
const isLoadingModels = ref(false);
const isSaving = ref(false);
const errorMessage = ref('');

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
  localApiKeys.requesty = settings.requestyKey || '';
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
</script>
