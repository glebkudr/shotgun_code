<template>
  <div class="openrouter-panel">
    <h2>OpenRouter Configuration & Suggestions</h2>

    <div class="form-group">
      <label for="apiKey">OpenRouter API Key:</label>
      <input type="password" id="apiKey" v-model="apiKey" placeholder="sk-or-..." />
    </div>

    <div class="form-group">
      <label for="modelName">OpenRouter Model:</label>
      <input type="text" id="modelName" v-model="modelName" />
    </div>

    <div class="form-group">
      <label for="systemPrompt">System Prompt:</label>
      <textarea id="systemPrompt" v-model="systemPrompt" rows="4"></textarea>
    </div>

    <div class="form-group">
      <label>Git Diff to Analyze (Read-only):</label>
      <pre class="diff-display"><code>{{ shotgunGitDiff || 'No diff content provided.' }}</code></pre>
    </div>

    <button @click="getSuggestions" :disabled="isLoading">
      <span v-if="isLoading">Loading...</span>
      <span v-else>Get Suggestions</span>
    </button>

    <div v-if="errorMsg" class="error-message">
      {{ errorMsg }}
    </div>

    <div v-if="suggestions" class="suggestions-output">
      <h3>Suggestions:</h3>
      <pre><code>{{ suggestions }}</code></pre>
    </div>
  </div>
</template>

<script setup>
import { ref, defineProps, defineEmits } from 'vue';
import { CallOpenRouter } from '../../wailsjs/go/main/App';

const props = defineProps({
  shotgunGitDiff: String,
});

const emits = defineEmits(['suggestions-obtained', 'suggestions-error']);

const apiKey = ref('');
const modelName = ref('openai/gpt-3.5-turbo'); // Default model
const systemPrompt = ref(
  'You are a code review assistant. Analyze the provided diff and return your suggestions as a code block or in a diff format.'
);
const isLoading = ref(false);
const errorMsg = ref('');
const suggestions = ref('');

async function getSuggestions() {
  isLoading.value = true;
  errorMsg.value = '';
  suggestions.value = '';

  if (!apiKey.value) {
    errorMsg.value = 'OpenRouter API Key is required.';
    isLoading.value = false;
    emits('suggestions-error', errorMsg.value);
    return;
  }

  if (!props.shotgunGitDiff) {
    errorMsg.value = 'Diff content is empty. Cannot get suggestions.';
    isLoading.value = false;
    emits('suggestions-error', errorMsg.value);
    return;
  }

  try {
    const result = await CallOpenRouter(
      apiKey.value,
      modelName.value,
      props.shotgunGitDiff,
      systemPrompt.value
    );
    suggestions.value = result;
    emits('suggestions-obtained', result);
  } catch (err) {
    console.error('Error calling OpenRouter:', err);
    errorMsg.value = `Failed to get suggestions: ${err.message || err}`;
    emits('suggestions-error', errorMsg.value);
  } finally {
    isLoading.value = false;
  }
}
</script>

<style scoped>
.openrouter-panel {
  padding: 20px;
  border: 1px solid #ccc;
  border-radius: 8px;
  margin-bottom: 20px;
  display: flex;
  flex-direction: column;
  gap: 15px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.form-group label {
  font-weight: bold;
}

input[type="text"],
input[type="password"],
textarea {
  width: 100%;
  padding: 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
  box-sizing: border-box;
}

textarea {
  resize: vertical;
  min-height: 60px;
}

.diff-display {
  background-color: #f0f0f0;
  padding: 10px;
  border-radius: 4px;
  max-height: 200px;
  overflow-y: auto;
  white-space: pre-wrap; /* Allows wrapping long lines */
  word-break: break-all; /* Breaks long words/lines if necessary */
}

button {
  padding: 10px 15px;
  background-color: #007bff;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.2s;
}

button:disabled {
  background-color: #aaa;
  cursor: not-allowed;
}

button:not(:disabled):hover {
  background-color: #0056b3;
}

.error-message {
  color: red;
  border: 1px solid red;
  padding: 10px;
  border-radius: 4px;
  background-color: #ffe0e0;
}

.suggestions-output {
  margin-top: 15px;
}

.suggestions-output h3 {
  margin-bottom: 5px;
}

.suggestions-output pre {
  background-color: #f9f9f9;
  border: 1px solid #eee;
  padding: 10px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow-y: auto;
}
</style>
