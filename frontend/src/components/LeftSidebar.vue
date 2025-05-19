<template>
  <CustomRulesModal
    :is-visible="isCustomRulesModalVisible"
    :initial-rules="currentCustomRulesForModal"
    title="Edit Custom Ignore Rules"
    ruleType="ignore"
    @save="handleSaveCustomRules"
    @cancel="handleCancelCustomRules"
  />
  <aside class="w-64 md:w-72 lg:w-80 bg-gray-50 p-4 border-r border-gray-200 overflow-y-auto flex flex-col flex-shrink-0">
    <!-- Project Selection and File Tree -->
    <div class="mb-6">
      <button 
        @click="$emit('select-folder')"
        class="w-full px-4 py-2 mb-2 bg-blue-600 text-white font-semibold rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
      >
        Select Project Folder
      </button>
      <div v-if="projectRoot" class="text-xs text-gray-600 mb-2 break-all">Selected: {{ projectRoot }}</div>
      
      <div v-if="projectRoot" class="mb-2">
        <label class="flex items-center text-sm text-gray-700" title="Uses .gitignore file if present in the project folder">
          <input 
            type="checkbox" 
            :checked="useGitignore"
            @change="$emit('toggle-gitignore', $event.target.checked)"
            class="form-checkbox h-4 w-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500 mr-2"
          />
          Use .gitignore rules
        </label>
        <label class="flex items-center text-sm text-gray-700 mt-1" title="Uses ignore.glob file if present in the project folder">
          <input
            type="checkbox"
            :checked="useCustomIgnore"
            @change="$emit('toggle-custom-ignore', $event.target.checked)"
            class="form-checkbox h-4 w-4 text-indigo-600 rounded border-gray-300 focus:ring-indigo-500 mr-2"
          />
          Use custom rules
          <button @click="openCustomRulesModal" title="Edit custom ignore rules" class="ml-2 p-0.5 hover:bg-gray-200 rounded text-xs">⚙️</button>
        </label>
      </div>

      <h2 class="text-lg font-semibold text-gray-700 mb-2">Project Files</h2>
      <div class="border border-gray-300 rounded min-h-[200px] bg-white text-sm overflow-auto max-h-[50vh]">
        <FileTree 
            v-if="fileTreeNodes && fileTreeNodes.length" 
            :nodes="fileTreeNodes" 
            :project-root="projectRoot"
            @toggle-exclude="(node) => $emit('toggle-exclude', node)"
        />
        <p v-else-if="projectRoot && !loadingError" class="p-2 text-xs text-gray-500">Loading tree...</p>
        <p v-else-if="!projectRoot" class="p-2 text-xs text-gray-500">Select a project folder to see files.</p>
        <p v-if="loadingError" class="p-2 text-xs text-red-500">{{ loadingError }}</p>
      </div>
    </div>

    <!-- Stepper Navigation (can remain if needed for overall app flow) -->
    <div v-if="steps && steps.length > 0">
      <h2 class="text-lg font-semibold text-gray-700 mb-2">Steps</h2>
      <div class="space-y-1">
        <div v-for="step in steps" :key="step.id">
          <button
            @click="canNavigateToStep(step.id) ? $emit('navigate', step.id) : null"
            :title="step.description"
            :class="[
              'w-full text-left px-3 py-2 rounded-md text-sm font-medium flex justify-between items-center',
              currentStep === step.id ? 'bg-blue-100 text-blue-700' : (step.completed ? 'bg-green-50 text-green-700 hover:bg-green-100' : 'text-gray-600 hover:bg-gray-100'),
              canNavigateToStep(step.id) ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'
            ]"
            :disabled="!canNavigateToStep(step.id)"
          >
            <span>{{ step.id }}. {{ step.title }}</span>
            <span v-if="currentStep === step.id" class="text-blue-500 text-xl">•</span> <!-- Current step indicator -->
            <svg v-else-if="step.completed" class="w-4 h-4 text-green-500" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { defineProps, defineEmits, ref } from 'vue';
import FileTree from './FileTree.vue'; // Import the existing FileTree
import CustomRulesModal from './CustomRulesModal.vue';
import { GetCustomIgnoreRules, SetCustomIgnoreRules } from '../../wailsjs/go/main/App';
import { LogError as LogErrorRuntime, LogInfo as LogInfoRuntime } from '../../wailsjs/runtime/runtime';

/**
 * Props for LeftSidebar:
 * - useGitignore: enables .gitignore rules for file parsing
 * - useCustomIgnore: enables custom ignore.glob rules for file parsing
 */
const props = defineProps({
  currentStep: { type: Number, required: true },
  steps: { type: Array, required: true }, // Array of { id: Number, title: String, completed: Boolean }
  projectRoot: { type: String, default: '' },
  fileTreeNodes: { type: Array, default: () => [] },
  useGitignore: { type: Boolean, default: true },
  useCustomIgnore: { type: Boolean, default: false },
  loadingError: { type: String, default: '' },
});

const emit = defineEmits(['navigate', 'select-folder', 'toggle-gitignore', 'toggle-custom-ignore', 'toggle-exclude', 'custom-rules-updated', 'add-log']);

const isCustomRulesModalVisible = ref(false);
const currentCustomRulesForModal = ref('');

async function openCustomRulesModal() {
  try {
    currentCustomRulesForModal.value = await GetCustomIgnoreRules();
    isCustomRulesModalVisible.value = true;
  } catch (error) {
    console.error("Error fetching custom ignore rules:", error);
    LogErrorRuntime(`Error fetching custom rules: ${error.message || error}`);
    emit('add-log', { message: `Failed to load custom rules: ${error.message || error}`, type: 'error' });
    // Show a placeholder or error message in the textarea if loading fails
    currentCustomRulesForModal.value = "# Error loading rules. Please check application logs.\n# You can still edit and save.";
    isCustomRulesModalVisible.value = true; // Still open modal
  }
}

async function handleSaveCustomRules(newRules) {
  try {
    await SetCustomIgnoreRules(newRules);
    isCustomRulesModalVisible.value = false;
    LogInfoRuntime('Custom ignore rules saved successfully via LeftSidebar.');
    emit('add-log', { message: 'Custom ignore rules saved.', type: 'success' });
    emit('custom-rules-updated'); // Notify MainLayout to refresh
  } catch (error) {
    console.error("Error saving custom ignore rules:", error);
    LogErrorRuntime(`Error saving custom rules: ${error.message || error}`);
    emit('add-log', { message: `Failed to save custom rules: ${error.message || error}`, type: 'error' });
    // Keep modal open for user to retry or copy content, or show an error in the modal itself.
  }
}

function handleCancelCustomRules() {
  isCustomRulesModalVisible.value = false;
}

function canNavigateToStep(stepId) {
  if (stepId === props.currentStep) return true;
  const targetStep = props.steps.find(s => s.id === stepId);
  if (targetStep && targetStep.completed) return true;
  const firstUncompletedStep = props.steps.find(s => !s.completed);
  const firstUncompletedStepId = firstUncompletedStep ? firstUncompletedStep.id : undefined;
  return stepId === firstUncompletedStepId || (firstUncompletedStepId === undefined && targetStep); // Allow any if all completed
}
</script> 

<style scoped>
/* Add your styles here */
</style> 