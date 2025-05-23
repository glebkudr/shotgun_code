<template>
  <CustomRulesModal
    v-if="modalRuleType"
    :is-visible="isCustomRulesModalVisible"
    :initial-rules="currentCustomRulesForModal"
    :title="modalTitle"
    :rule-type="modalRuleType"
    @save="handleSaveCustomRules"
    @cancel="handleCancelCustomRules"
  />
  <aside class="sidebar-container right-sidebar relative" :style="{ width: width + 'px' }">
    <div class="resize-handle right" @mousedown="startResize"></div>
    <div v-if="currentStep === 1" class="h-full flex flex-col p-4">
      <div class="mb-4">
        <button 
          @click="handleSelectFolder"
          class="btn-primary w-full mb-2 flex items-center justify-center"
          :disabled="isSelecting"
        >
          <span v-if="isSelecting" class="mr-2 animate-spin">⟳</span>
          {{ isSelecting ? 'Selecting...' : 'Select Project Folder' }}
        </button>
        <div v-if="projectRoot" class="text-hint mb-2 break-all">Selected: {{ projectRoot }}</div>
      </div>
      
      <div v-if="projectRoot" class="mb-4">
        <div class="checkbox-container">
          <input 
            type="checkbox" 
            id="gitignore-toggle"
            :checked="useGitignore"
            @change="$emit('toggle-gitignore', $event.target.checked)"
            class="input-checkbox"
          />
          <label for="gitignore-toggle" class="text-body" title="Uses .gitignore file if present in the project folder">
            Use .gitignore rules
          </label>
        </div>
        
        <div class="checkbox-container mt-2">
          <input
            type="checkbox"
            id="custom-rules-toggle"
            :checked="useCustomIgnore"
            @change="$emit('toggle-custom-ignore', $event.target.checked)"
            class="input-checkbox"
          />
          <label for="custom-rules-toggle" class="flex items-center text-body font-medium" title="Uses ignore.glob file if present in the project folder">
            <span>Use custom rules</span>
            <button @click="openCustomRulesModal('ignore')" title="Edit custom ignore rules" class="btn-icon ml-2 text-sm">⚙️</button>
          </label>
        </div>
      </div>

      <h2 class="text-subtitle mb-3">Project Files</h2>
      <div class="file-tree-container flex-grow overflow-auto">
        <div v-if="loadingError" class="text-error p-2">
          {{ loadingError }}
        </div>
        <div v-else-if="!projectRoot" class="text-hint p-2">
          Select a project folder to view files
        </div>
        <div v-else-if="fileTreeNodes.length === 0" class="text-hint p-2">
          No files found in the selected directory
        </div>
        <FileTree 
          v-else 
          :nodes="fileTreeNodes" 
          @toggle-exclude="$emit('toggle-exclude', $event)"
        />
      </div>
    </div>

    <div v-else-if="currentStep === 2" class="h-full flex flex-col p-4">
      <!-- Step 2: Prompt Properties -->
      <h2 class="text-subtitle mb-4">Prompt Settings</h2>
      
      <div class="mb-4">
        <label class="block text-sm font-medium mb-2 text-body">Template Selection</label>
        <div class="relative" ref="dropdownRef">
          <button 
            @click="toggleDropdown" 
            type="button" 
            class="custom-dropdown-button w-full flex items-center justify-between text-left"
          >
            <span>{{ promptTemplates[selectedPromptTemplate].name }}</span>
            <svg class="w-4 h-4 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
            </svg>
          </button>
          <!-- Dropdown menu -->
          <div v-if="isDropdownOpen" class="custom-dropdown-menu w-full">
            <button 
              v-for="(template, key) in promptTemplates" 
              :key="key" 
              @click="selectTemplate(key)"
              class="custom-dropdown-item"
              :class="{'active': selectedPromptTemplate === key}"
            >
              {{ template.name }}
            </button>
          </div>
        </div>
      </div>
      
      <div class="mb-4">
        <div class="flex items-center justify-between mb-2">
          <label class="block text-sm font-medium text-body">Custom Rules</label>
          <button 
            @click="openCustomRulesModal('prompt')" 
            title="Edit custom prompt rules" 
            class="btn-icon"
          >⚙️</button>
        </div>
        <textarea
          :value="localRulesContent"
          rows="5"
          class="input-textarea resize-none text-sm w-full"
          style="max-height: 120px;"
          placeholder="Rules for AI..."
          @input="$emit('update:rules-content', localRulesContent)"
        ></textarea>
      </div>
      
      <div class="mt-6">
        <h3 class="text-sm font-medium mb-2 text-body">Token Count</h3>
        <div class="token-counter w-full">
          <span
            v-show="!isLoadingFinalPrompt"
            :class="['text-body font-medium', charCountColorClass]"
            :title="tooltipText">
            <span class="font-normal">Token:</span> {{ approximateTokens }}
          </span>
          <div class="mt-1 text-xs text-hint">
            This is an approximation based on the current prompt.
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="currentStep === 3" class="h-full flex flex-col p-4">
      <!-- Step 3: Execution Properties -->
      <h2 class="text-subtitle mb-4">Execution Settings</h2>
      
      <div class="mb-4">
        <label class="block text-sm font-medium mb-2 text-body">Model Selection</label>
        <div class="relative" ref="modelDropdownRef">
          <button 
            @click="toggleModelDropdown" 
            type="button" 
            class="custom-dropdown-button w-full flex items-center justify-between text-left"
          >
            <span>{{ selectedModel }}</span>
            <svg class="w-4 h-4 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
            </svg>
          </button>
          <!-- Dropdown menu -->
          <div v-if="isModelDropdownOpen" class="custom-dropdown-menu w-full">
            <button 
              v-for="model in modelOptions" 
              :key="model" 
              @click="selectModel(model)"
              class="custom-dropdown-item"
              :class="{'active': selectedModel === model}"
            >
              {{ model }}
            </button>
          </div>
        </div>
      </div>
      
      <div class="mb-4">
        <label class="block text-sm font-medium mb-2 text-body">Temperature</label>
        <div class="flex items-center">
          <input 
            type="range" 
            min="0" 
            max="1" 
            step="0.1" 
            v-model="temperature" 
            class="w-full mr-2 slider-input accent-primary"
          />
          <span class="text-sm font-mono text-body">{{ temperature }}</span>
        </div>
        <p class="text-xs text-hint mt-1">
          Lower values = more deterministic, higher values = more creative
        </p>
      </div>
      
      <div>
        <label class="block text-sm font-medium mb-2 text-body">Advanced Options</label>
        <div class="checkbox-container">
          <input type="checkbox" id="stream-output" class="input-checkbox" />
          <label for="stream-output" class="text-body">Stream output</label>
        </div>
        <div class="checkbox-container mt-2">
          <input type="checkbox" id="save-history" class="input-checkbox" />
          <label for="save-history" class="text-body">Save conversation history</label>
        </div>
      </div>
    </div>

    <div v-else-if="currentStep === 4" class="h-full flex flex-col">
      <!-- Step 4: Patch Properties -->
      <h2 class="text-subtitle mb-4">Patch Settings</h2>
      
      <div class="mb-4">
        <label class="block text-sm font-medium mb-2 text-body">Patch Format</label>
        <div class="relative" ref="patchFormatDropdownRef">
          <button 
            @click="togglePatchFormatDropdown" 
            type="button" 
            class="custom-dropdown-button w-full flex items-center justify-between text-left"
          >
            <span>{{ selectedPatchFormat }}</span>
            <svg class="w-4 h-4 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
            </svg>
          </button>
          <!-- Dropdown menu -->
          <div v-if="isPatchFormatDropdownOpen" class="custom-dropdown-menu w-full">
            <button 
              v-for="format in patchFormatOptions" 
              :key="format.value" 
              @click="selectPatchFormat(format)"
              class="custom-dropdown-item"
              :class="{'active': selectedPatchFormat === format.label}"
            >
              {{ format.label }}
            </button>
          </div>
        </div>
      </div>
      
      <div class="mb-4">
        <div class="checkbox-container">
          <input type="checkbox" id="include-context" class="input-checkbox" checked />
          <label for="include-context" class="text-body">Include context lines</label>
        </div>
      </div>
      
      <div>
        <label class="block text-sm font-medium mb-2 text-body">Apply Options</label>
        <div class="checkbox-container">
          <input type="checkbox" id="create-backup" class="input-checkbox" checked />
          <label for="create-backup" class="text-body">Create backup files</label>
        </div>
        <div class="checkbox-container mt-2">
          <input type="checkbox" id="auto-commit" class="input-checkbox" />
          <label for="auto-commit" class="text-body">Auto-commit changes</label>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, computed, watch, onUnmounted, onMounted, defineProps, defineEmits } from 'vue';
import FileTree from './FileTree.vue';
import CustomRulesModal from './CustomRulesModal.vue';

// Import template contents
import devTemplateContentFromFile from '../../../design/prompts/prompt_makeDiffGitFormat.md?raw';
import architectTemplateContentFromFile from '../../../design/prompts/prompt_makePlan.md?raw';
import findBugTemplateContentFromFile from '../../../design/prompts/prompt_analyzeBug.md?raw';
import projectManagerTemplateContentFromFile from '../../../design/prompts/prompt_projectManager.md?raw';
import { GetCustomPromptRules, SetCustomPromptRules, GetCustomIgnoreRules, SetCustomIgnoreRules, SelectDirectory as SelectDirectoryGo } from '../../wailsjs/go/main/App';

const emit = defineEmits([
  'select-folder',
  'toggle-gitignore',
  'toggle-custom-ignore',
  'update:selectedTemplate',
  'update:rulesContent',
  'toggle-exclude',
  'custom-rules-updated',
  'template-change',
  'step-completed',
  'update:rules-content',
  'resize'
]);

const props = defineProps({
  width: {
    type: Number,
    default: 300
  },
  currentStep: { type: Number, default: 1 },
  projectRoot: { type: String, default: '' },
  fileTreeNodes: { type: Array, default: () => [] },
  useGitignore: { type: Boolean, default: true },
  useCustomIgnore: { type: Boolean, default: false },
  loadingError: { type: String, default: '' },
  rulesContent: { type: String, default: '' },
  finalPrompt: { type: String, default: '' },
  selectedTemplate: { type: String, default: 'dev' }
});

// Removed duplicate emit declaration

// Template selection
const promptTemplates = {
  dev: { 
    name: 'Development', 
    content: devTemplateContentFromFile 
  },
  architect: { 
    name: 'Architecture', 
    content: architectTemplateContentFromFile 
  },
  findBug: { 
    name: 'Find Bug', 
    content: findBugTemplateContentFromFile 
  },
  projectManager: { 
    name: 'Project Manager', 
    content: projectManagerTemplateContentFromFile 
  },
};

const selectedPromptTemplate = ref(props.selectedTemplate);
const isDropdownOpen = ref(false);
const dropdownRef = ref(null);

const isModelDropdownOpen = ref(false);
const modelDropdownRef = ref(null);
const isPatchFormatDropdownOpen = ref(false);
const patchFormatDropdownRef = ref(null);

// Model options
const modelOptions = ['GPT-4', 'GPT-3.5 Turbo', 'Claude 3'];
const selectedModel = ref('GPT-4');

// Patch format options
const patchFormatOptions = [
  { value: 'unified', label: 'Unified Diff' },
  { value: 'git', label: 'Git Format' },
  { value: 'context', label: 'Context Diff' }
];
const selectedPatchFormat = ref('Unified Diff');

// Watch for prop changes
watch(() => props.selectedTemplate, (newVal) => {
  if (newVal !== selectedPromptTemplate.value) {
    selectedPromptTemplate.value = newVal;
  }
}, { immediate: true });

// Template selection functions
function toggleDropdown() {
  isDropdownOpen.value = !isDropdownOpen.value;
  isModelDropdownOpen.value = false;
  isPatchFormatDropdownOpen.value = false;
}

function toggleModelDropdown() {
  isModelDropdownOpen.value = !isModelDropdownOpen.value;
  isDropdownOpen.value = false;
  isPatchFormatDropdownOpen.value = false;
}

function togglePatchFormatDropdown() {
  isPatchFormatDropdownOpen.value = !isPatchFormatDropdownOpen.value;
  isDropdownOpen.value = false;
  isModelDropdownOpen.value = false;
}

function selectTemplate(key) {
  selectedPromptTemplate.value = key;
  isDropdownOpen.value = false;
  emit('update:selectedTemplate', key);
}

function selectModel(model) {
  selectedModel.value = model;
  isModelDropdownOpen.value = false;
}

function selectPatchFormat(format) {
  selectedPatchFormat.value = format.label;
  isPatchFormatDropdownOpen.value = false;
}

// Close dropdowns when clicking outside
onMounted(() => {
  document.addEventListener('click', (event) => {
    if (dropdownRef.value && !dropdownRef.value.contains(event.target)) {
      isDropdownOpen.value = false;
    }
    if (modelDropdownRef.value && !modelDropdownRef.value.contains(event.target)) {
      isModelDropdownOpen.value = false;
    }
    if (patchFormatDropdownRef.value && !patchFormatDropdownRef.value.contains(event.target)) {
      isPatchFormatDropdownOpen.value = false;
    }
  });
});

// Modal state
const isCustomRulesModalVisible = ref(false);
const currentCustomRulesForModal = ref('');
const modalTitle = ref('');
const modalRuleType = ref('');

// Prompt settings
const localRulesContent = ref(props.rulesContent);

// Execution settings
const temperature = ref(0.7);

// Clean up event listeners when component is unmounted
onUnmounted(() => {
  document.removeEventListener('mousemove', handleResize);
  document.removeEventListener('mouseup', stopResize);
});

// Watch for rulesContent changes from parent
watch(() => props.rulesContent, (newValue) => {
  if (newValue !== localRulesContent.value) {
    localRulesContent.value = newValue;
  }
});

watch(() => props.projectRoot, (newValue) => {
  if (newValue && props.currentStep === 1) {
    emit('step-completed', 1);
  }
});

// Token count calculation
const approximateTokens = computed(() => {
  const tokens = Math.round((props.finalPrompt || '').length / 3);
  return tokens.toString().replace(/\B(?=(\d{3})+(?!\d))/g, " ");
});

function openCustomRulesModal(type) {
  if (type === 'prompt') {
    modalTitle.value = 'Edit Custom Prompt Rules';
    modalRuleType.value = 'prompt';
    GetCustomPromptRules().then(rules => {
      currentCustomRulesForModal.value = rules;
      isCustomRulesModalVisible.value = true;
    });
  } else {
    modalTitle.value = 'Edit Custom Ignore Rules';
    modalRuleType.value = 'ignore';
    GetCustomIgnoreRules().then(rules => {
      currentCustomRulesForModal.value = rules;
      isCustomRulesModalVisible.value = true;
    });
  }
}

function handleSaveCustomRules(newRules) {
  if (modalRuleType.value === 'prompt') {
    SetCustomPromptRules(newRules).then(() => {
      isCustomRulesModalVisible.value = false;
      emit('custom-rules-updated');
    });
  } else {
    SetCustomIgnoreRules(newRules).then(() => {
      isCustomRulesModalVisible.value = false;
      emit('custom-rules-updated');
    });
  }
}

function handleCancelCustomRules() {
  isCustomRulesModalVisible.value = false;
}

// Resize functionality
const isResizing = ref(false);
const startX = ref(0);
const startWidth = ref(0);

function startResize(event) {
  isResizing.value = true;
  startX.value = event.clientX;
  startWidth.value = props.width;
  
  // Add class to pause transitions during resize
  document.documentElement.classList.add('resize-transition-paused');
  
  document.addEventListener('mousemove', doResize);
  document.addEventListener('mouseup', stopResize);
  event.preventDefault();
}

function doResize(event) {
  if (!isResizing.value) return;
  
  const dx = startX.value - event.clientX; // Note: for right sidebar, we subtract
  const newWidth = startWidth.value + dx;
  
  // Set min and max width constraints
  const minWidth = 180;
  const maxWidth = window.innerWidth * 0.4; // 40% of window width
  
  if (newWidth >= minWidth && newWidth <= maxWidth) {
    emit('resize', newWidth);
  }
}

function stopResize() {
  isResizing.value = false;
  
  // Remove transition pause class
  document.documentElement.classList.remove('resize-transition-paused');
  
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
}

const isSelecting = ref(false);

const handleSelectFolder = () => {
  if (isSelecting.value) return;
  
  isSelecting.value = true;
  console.log('Emitting select-folder event');
  
  emit('select-folder');
  setTimeout(() => {
    isSelecting.value = false;
  }, 500);
};
</script>
