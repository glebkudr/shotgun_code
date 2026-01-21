<template>
  <div class="flex flex-col h-screen bg-gray-100">
    <HorizontalStepper :current-step="currentStep" :steps="steps" @navigate="navigateToStep" :key="`hstepper-${currentStep}-${steps.map(s=>s.completed).join('')}`" />
    <div ref="layoutRef" class="flex flex-1 overflow-hidden">
      <template v-if="currentStep !== 3">
        <div class="flex-shrink-0 h-full" :style="{ width: `${sidebarWidth}px` }">
          <LeftSidebar 
            class="w-full h-full"
            :current-step="currentStep" 
            :steps="steps" 
            :project-roots="projectRoots"
            :file-tree-nodes="fileTree"
            :use-gitignore="useGitignore"
            :use-custom-ignore="useCustomIgnore"
            :loading-error="loadingError"
            @navigate="navigateToStep"
            @select-folder="selectProjectFolderHandler"
            @remove-root="removeProjectRootHandler"
            @toggle-gitignore="toggleGitignoreHandler"
            @toggle-custom-ignore="toggleCustomIgnoreHandler"
            @toggle-exclude="toggleExcludeNode"
            @custom-rules-updated="handleCustomRulesUpdated"
            @add-log="({message, type}) => addLog(message, type)" />
        </div>
        <div
          class="w-1.5 bg-gray-300 hover:bg-gray-400 cursor-col-resize select-none"
          title="Resize sidebar"
          @mousedown="startSidebarResize"
        ></div>
      </template>
      <CentralPanel class="flex-1 min-w-0" :current-step="currentStep" 
                    :shotgun-prompt-context="shotgunPromptContext"
                    :shotgun-prompt-context-tokens="shotgunPromptContextTokens"
                    :generation-progress="generationProgressData"
                    :is-generating-context="isGeneratingContext"
                    :project-roots="projectRoots" 
                    :platform="platform"
                    :user-task="userTask"
                    :rules-content="rulesContent"
                    :final-prompt="finalPrompt"
                    :has-active-llm-key="hasActiveLlmKey"
                    :is-auto-context-loading="isAutoContextLoading"
                    @auto-context="requestAutoContextSelection"
                    @open-llm-settings="openLlmSettingsModal"
                    @step-action="handleStepAction"
                    @update-composed-prompt="handleComposedPromptUpdate"
                    @update:user-task="handleUserTaskUpdate"
                    @update:rules-content="handleRulesContentUpdate"
                    ref="centralPanelRef" />
    </div>
    <div 
      @mousedown="startResize"
      class="w-full h-2 bg-gray-300 hover:bg-gray-400 cursor-row-resize select-none"
      title="Resize console height"
    >
    </div>
    <BottomConsole :log-messages="logMessages" :height="consoleHeight" ref="bottomConsoleRef" />
    <LlmSettingsModal
      :is-visible="isLlmSettingsModalVisible"
      :initial-settings="llmSettings"
      @close="closeLlmSettingsModal"
      @saved="handleLlmSettingsSaved"
    />
  </div>
</template>

<script setup>
import { ref, reactive, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import HorizontalStepper from './HorizontalStepper.vue';
import LeftSidebar from './LeftSidebar.vue';
import CentralPanel from './CentralPanel.vue';
import BottomConsole from './BottomConsole.vue';
import LlmSettingsModal from './LlmSettingsModal.vue';
import {
  AddProjectRoot,
  GetProjectRoots,
  ListFiles,
  RemoveProjectRoot,
  RequestAutoContextSelection,
  RequestShotgunContextGeneration,
  SelectDirectory as SelectDirectoryGo,
  StartFileWatcher,
  StopFileWatcher,
  SetUseGitignore,
  SetUseCustomIgnore,
  GetLlmSettings,
  HasActiveLlmKey,
  GetAutoContextButtonTexture,
} from '../../wailsjs/go/main/App';
import { EventsOn, Environment } from '../../wailsjs/runtime/runtime';

const currentStep = ref(1);
const steps = ref([
  { id: 1, title: 'Prepare Context', completed: false, description: 'Select project folder, review files, and generate the initial project context for the LLM.' },
  { id: 2, title: 'Compose Prompt', completed: false, description: 'Provide a prompt to the LLM based on the project context to generate a code diff.' },
  { id: 3, title: 'Prompt History', completed: false, description: 'Review previously executed prompts and responses.', alwaysAccessible: true },
]);

const logMessages = ref([]);
const centralPanelRef = ref(null); 
const bottomConsoleRef = ref(null);
const MIN_CONSOLE_HEIGHT = 50;
const SIDEBAR_MIN_WIDTH = 240;
const SIDEBAR_MAX_WIDTH = 640;
const SIDEBAR_DEFAULT_WIDTH = 320;
const MAIN_MIN_WIDTH = 360;
const CONTEXT_GENERATION_DEBOUNCE_MS = 2000;
const consoleHeight = ref(MIN_CONSOLE_HEIGHT); // Initial height in pixels
const sidebarWidth = ref(SIDEBAR_DEFAULT_WIDTH);
const isSidebarResizing = ref(false);
const layoutRef = ref(null);

function addLog(message, type = 'info', targetConsole = 'bottom') {
  const logEntry = {
    message,
    type,
    timestamp: new Date().toLocaleTimeString()
  };

  if (targetConsole === 'bottom' || targetConsole === 'both') {
    logMessages.value.push(logEntry);
  }
  if (targetConsole === 'step' || targetConsole === 'both') {
    if (centralPanelRef.value && currentStep.value === 3 && centralPanelRef.value.addLogToStep3Console) {
      centralPanelRef.value.addLogToStep3Console(message, type);
    }
  }
}
function nodeToggleKey(node) {
  if (!node) return '';
  if (node.path) return node.path;
  if (node.rootPath || node.relPath) return `${node.rootPath || ''}::${node.relPath || ''}`;
  return '';
}

const projectRoots = ref([]);
const projectRootPaths = computed(() => projectRoots.value.map((root) => root.path).filter(Boolean));
const hasProjectRoots = computed(() => projectRootPaths.value.length > 0);
const fileTree = ref([]);
const shotgunPromptContext = ref('');
const shotgunPromptContextTokens = ref(null);
const loadingError = ref('');
const useGitignore = ref(true);
const useCustomIgnore = ref(true);
const manuallyToggledNodes = reactive(new Map());
const isGeneratingContext = ref(false);
const contextRegenerationPending = ref(false);
const generationProgressData = ref({ current: 0, total: 0 });
const isFileTreeLoading = ref(false);
const platform = ref('unknown'); // To store OS platform (e.g., 'darwin', 'windows', 'linux')
const userTask = ref('');
const rulesContent = ref('');
const finalPrompt = ref('');
const hasActiveLlmKey = ref(false);
const isAutoContextLoading = ref(false);
const autoContextButtonTexture = ref('');
const isLlmSettingsModalVisible = ref(false);
const llmSettings = ref({});
let debounceTimer = null;

// Watcher related
const projectFilesChangedPendingReload = ref(false);
let unlistenProjectFilesChanged = null;

async function selectProjectFolderHandler() {
  try {
    shotgunPromptContext.value = '';
    isGeneratingContext.value = false;
    const selectedDir = await SelectDirectoryGo(); 
    if (selectedDir) {
      const updatedRoots = await AddProjectRoot(selectedDir);
      projectRoots.value = updatedRoots;
      loadingError.value = '';
      manuallyToggledNodes.clear();
      fileTree.value = [];

      steps.value.forEach(s => s.completed = false);
      currentStep.value = 1;
      addLog(`Project folder added: ${selectedDir}`, 'info', 'bottom');
    }
  } catch (err) {
    console.error("Error selecting directory:", err);
    const errorMsg = "Failed to select directory: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
  }
}

async function removeProjectRootHandler(rootPath) {
  if (!rootPath) {
    return;
  }
  try {
    shotgunPromptContext.value = '';
    isGeneratingContext.value = false;
    const updatedRoots = await RemoveProjectRoot(rootPath);
    projectRoots.value = updatedRoots;
    loadingError.value = '';
    manuallyToggledNodes.clear();
    fileTree.value = [];
    steps.value.forEach(s => s.completed = false);
    currentStep.value = 1;
    addLog(`Project folder removed: ${rootPath}`, 'info', 'bottom');
  } catch (err) {
    console.error("Error removing directory:", err);
    const errorMsg = "Failed to remove directory: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
  }
}

async function loadFileTree(rootDirs) {
  const roots = Array.isArray(rootDirs) ? rootDirs : projectRootPaths.value;
  if (!roots || roots.length === 0) {
    fileTree.value = [];
    isFileTreeLoading.value = false;
    return;
  }
  let loadSucceeded = false;
  isFileTreeLoading.value = true;
  loadingError.value = '';
  addLog(`Loading file tree for ${roots.length} root(s).`, 'info', 'bottom');
  try {
    const treeData = await ListFiles(roots);
    fileTree.value = mapDataToTreeRecursive(treeData, null);
    updateAllNodesExcludedState(fileTree.value);
    loadSucceeded = true;
    addLog(`File tree loaded successfully. Root items: ${fileTree.value.length}`, 'info', 'bottom');
  } catch (err) {
    console.error("Error listing files:", err);
    const errorMsg = "Failed to load file tree: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
    fileTree.value = [];
  } finally {
    isFileTreeLoading.value = false;
    if (loadSucceeded) {
      debouncedTriggerShotgunContextGeneration();
    }
    checkAndProcessPendingFileTreeReload();
  }
}

function calculateNodeExcludedState(node) {
  const manualToggle = manuallyToggledNodes.get(nodeToggleKey(node));
  if (manualToggle !== undefined) return manualToggle;
  if (useGitignore.value && node.isGitignored) return true;
  if (useCustomIgnore.value && node.isCustomIgnored) return true;
  return false;
}

function mapDataToTreeRecursive(nodes, parent) {
  if (!nodes) return [];
  return nodes.map(node => {
    const isRootNode = parent === null;
    const reactiveNode = reactive({
      ...node,
      expanded: node.isDir ? isRootNode : undefined,
      parent: parent,
      children: [] 
    });
    reactiveNode.excluded = calculateNodeExcludedState(reactiveNode);

    if (node.children && node.children.length > 0) {
      reactiveNode.children = mapDataToTreeRecursive(node.children, reactiveNode);
    }
    return reactiveNode;
  });
}

function isAnyParentVisuallyExcluded(node) {
  if (!node || !node.parent) {
    return false;
  }
  let current = node.parent;
  while (current) {
    if (current.excluded) { // current.excluded reflects its visual/checkbox state
      return true;
    }
    current = current.parent;
  }
  return false;
}

function hasVisuallyIncludedDescendant(node) {
  if (!node || !node.children || node.children.length === 0) {
    return false;
  }
  return node.children.some((child) => !child.excluded || hasVisuallyIncludedDescendant(child));
}
function collectTrulyExcludedPathsByRoot(nodes, targetByRoot) {
  if (!nodes || nodes.length === 0) return;
  nodes.forEach((node) => {
    if (node.excluded && !hasVisuallyIncludedDescendant(node)) {
      const rootPath = node.rootPath;
      if (!rootPath) {
        return;
      }
      if (!targetByRoot[rootPath]) {
        targetByRoot[rootPath] = [];
      }
      targetByRoot[rootPath].push(node.relPath);
    } else if (node.children && node.children.length > 0) {
      collectTrulyExcludedPathsByRoot(node.children, targetByRoot);
    }
  });
}

function buildExcludedPathsPayload() {
  const excludedByRoot = {};
  collectTrulyExcludedPathsByRoot(fileTree.value, excludedByRoot);
  return excludedByRoot;
}

function collectIgnoredPathsOnlyByRoot(nodes, targetByRoot) {
  if (!nodes || nodes.length === 0) return;
  nodes.forEach((node) => {
    const ignoredByGit = useGitignore.value && node.isGitignored;
    const ignoredByCustom = useCustomIgnore.value && node.isCustomIgnored;
    const ignoredByRules = ignoredByGit || ignoredByCustom;

    if (ignoredByRules && node.relPath) {
      // For auto-context, we exclude whole branches based on the root ignored node.
      // Children may still be present in the UI tree (e.g. inside gitignored folders),
      // but for the auto-context tree it's enough to mark only the root path here.
      const rootPath = node.rootPath;
      if (!rootPath) {
        return;
      }
      if (!targetByRoot[rootPath]) {
        targetByRoot[rootPath] = [];
      }
      targetByRoot[rootPath].push(node.relPath);
      return;
    }

    if (node.children && node.children.length > 0) {
      collectIgnoredPathsOnlyByRoot(node.children, targetByRoot);
    }
  });
}

function buildIgnoredPathsPayloadForAutoContext() {
  const ignoredByRoot = {};
  collectIgnoredPathsOnlyByRoot(fileTree.value, ignoredByRoot);
  return ignoredByRoot;
}

function normalizeRelPath(value) {
  if (!value) return '';
  return value.replace(/\\/g, '/').replace(/^\.\//, '').replace(/^\/+/, '');
}

function toggleExcludeNode(nodeToToggle) {
  // If the node is under an unselected parent and is currently unselected itself (nodeToToggle.excluded is true),
  // the first click should select it (set nodeToToggle.excluded to false).
  if (isAnyParentVisuallyExcluded(nodeToToggle) && nodeToToggle.excluded) {
    nodeToToggle.excluded = false;
  } else {
    // Otherwise, normal toggle behavior.
    nodeToToggle.excluded = !nodeToToggle.excluded;
  }
  manuallyToggledNodes.set(nodeToggleKey(nodeToToggle), nodeToToggle.excluded);

  // FIX: When toggling a folder, clear manual overrides for all descendants
  // so they inherit the new state of the parent. This fixes the issue where
  // Auto Context pins all files, preventing parent folders from affecting children.
  if (nodeToToggle.isDir) {
    clearDescendantManualToggles(nodeToToggle);
    if (nodeToToggle.children && nodeToToggle.children.length > 0) {
      _updateAllNodesExcludedStateRecursive(nodeToToggle.children, nodeToToggle.excluded);
    }
  }

  addLog(`Toggled exclusion for ${nodeToToggle.name} to ${nodeToToggle.excluded}`, 'info', 'bottom');
  debouncedTriggerShotgunContextGeneration();
}

function clearDescendantManualToggles(node) {
  if (node.children && node.children.length > 0) {
    node.children.forEach(child => {
      manuallyToggledNodes.delete(nodeToggleKey(child));
      clearDescendantManualToggles(child);
    });
  }
}

function updateAllNodesExcludedState(nodesToUpdate) { // This is the public-facing function
  // It calls the recursive helper, starting with parentIsVisuallyExcluded = false for root nodes.
  _updateAllNodesExcludedStateRecursive(nodesToUpdate, false);
}

function _updateAllNodesExcludedStateRecursive(nodesToUpdate, parentIsVisuallyExcluded) {
   if (!nodesToUpdate || nodesToUpdate.length === 0) return;
   nodesToUpdate.forEach(node => {
    const manualToggle = manuallyToggledNodes.get(nodeToggleKey(node));
    let isExcludedByRule = false;
    if (useGitignore.value && node.isGitignored) isExcludedByRule = true;
    if (useCustomIgnore.value && node.isCustomIgnored) isExcludedByRule = true;

    if (manualToggle !== undefined) {
      // If there's a manual toggle, it dictates the state.
      node.excluded = manualToggle;
    } else {
      // If not manually toggled, it's excluded if a rule matches OR if its parent is visually excluded.
      // This establishes the default inherited exclusion for visual purposes.
      node.excluded = isExcludedByRule || parentIsVisuallyExcluded;
    }

     if (node.children && node.children.length > 0) {
      _updateAllNodesExcludedStateRecursive(node.children, node.excluded); // Pass current node's new visual excluded state
     }
   });
 }

function toggleGitignoreHandler(value) {
  useGitignore.value = value;
  addLog(`.gitignore usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseGitignore(value)
    .then(() => addLog(`Watchman instructed to use .gitignore: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useGitignore in backend: ${err}`, 'error'));
  updateAllNodesExcludedState(fileTree.value);
  debouncedTriggerShotgunContextGeneration();
}

function toggleCustomIgnoreHandler(value) {
  useCustomIgnore.value = value;
  addLog(`Custom ignore rules usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseCustomIgnore(value)
    .then(() => addLog(`Watchman instructed to use custom ignores: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useCustomIgnore in backend: ${err}`, 'error'));
  updateAllNodesExcludedState(fileTree.value);
  debouncedTriggerShotgunContextGeneration();
}

function debouncedTriggerShotgunContextGeneration() {
  if (!hasProjectRoots.value) {
    // Clear context and stop loading if no project root
    shotgunPromptContext.value = ''; // Clear previous context
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress
    // isGeneratingContext will be set to false by the return or by the timeout if it runs
    isGeneratingContext.value = false;
    return;
  }

  if (isFileTreeLoading.value) {
    contextRegenerationPending.value = true;
    addLog("Context regeneration queued: file tree is loading.", 'debug', 'bottom');
    return;
  }
  if (isGeneratingContext.value) {
    contextRegenerationPending.value = true;
    addLog("Context regeneration queued: generation in progress.", 'debug', 'bottom');
    return;
  }
  contextRegenerationPending.value = false;

  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    if (!hasProjectRoots.value) { 
      isGeneratingContext.value = false;
      return;
    }
    if (isFileTreeLoading.value) {
      contextRegenerationPending.value = true;
      addLog("Context regeneration queued: file tree became loading.", 'debug', 'bottom');
      return;
    }
    isGeneratingContext.value = true;

    addLog("Debounced trigger: Requesting shotgun context generation...", 'info');
    
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress before new request
    const excludedByRoot = buildExcludedPathsPayload();
 
     RequestShotgunContextGeneration(projectRootPaths.value, excludedByRoot)
       .catch(err => {
        const errorMsg = "Error calling RequestShotgunContextGeneration: " + (err.message || err);
        addLog(errorMsg, 'error');
        shotgunPromptContext.value = "Error: " + errorMsg;
        isGeneratingContext.value = false;
        if (contextRegenerationPending.value) {
          contextRegenerationPending.value = false;
          debouncedTriggerShotgunContextGeneration();
        }
      })
      .finally(() => {
         // isGeneratingContext.value = false;
      });
  }, CONTEXT_GENERATION_DEBOUNCE_MS); 
}

function navigateToStep(stepId) {
  const targetStep = steps.value.find(s => s.id === stepId);
  if (!targetStep) return;

  if (stepId === currentStep.value) {
    currentStep.value = stepId;
    return;
  }

  if (targetStep.alwaysAccessible) {
    currentStep.value = stepId;
    return;
  }

  if (targetStep.completed) {
    currentStep.value = stepId;
    return;
  }

  const firstUncompletedStep = steps.value.find(s => !s.completed);
  if (!firstUncompletedStep || stepId === firstUncompletedStep.id) {
    currentStep.value = stepId;
  } else {
    addLog(`Cannot navigate to step ${stepId} yet. Please complete step ${firstUncompletedStep.id}.`, 'warn');
  }
}

function handleComposedPromptUpdate(prompt) {
  finalPrompt.value = prompt;
  addLog(`MainLayout: Composed LLM prompt updated (${prompt.length} chars).`, 'debug', 'bottom');
  // Logic to mark step 2 as complete can go here
  if (currentStep.value === 2 && prompt && steps.value[0].completed) {
    const step2 = steps.value.find(s => s.id === 2);
    if (step2 && !step2.completed) {
      step2.completed = true;
      addLog("Step 2: Prompt composed. Ready to proceed to Step 3.", "success", "bottom");
    }
  }
}

async function handleStepAction(actionName, payload) {
  addLog(`Action: ${actionName} triggered from step ${currentStep.value}.`, 'info', 'bottom');
  if (payload && actionName === 'composePrompt') {
    addLog(`Prompt for diff: "${payload.prompt}"`, 'info', 'bottom');
    return;
  }

  if (!actionName) {
    return;
  }

  addLog(`No handler registered for action: ${actionName}`, 'debug', 'bottom');
}

const isResizing = ref(false);

function startResize(event) {
  isResizing.value = true;
  document.addEventListener('mousemove', doResize);
  document.addEventListener('mouseup', stopResize);
  event.preventDefault(); 
}

function doResize(event) {
  if (!isResizing.value) return;
  const newHeight = window.innerHeight - event.clientY;
  const minHeight = MIN_CONSOLE_HEIGHT;
  const maxHeight = window.innerHeight * 0.7;
  consoleHeight.value = Math.max(minHeight, Math.min(newHeight, maxHeight));
}

function stopResize() {
  isResizing.value = false;
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
}

function clampSidebarWidth() {
  const container = layoutRef.value;
  if (!container) {
    return;
  }
  const rect = container.getBoundingClientRect();
  const maxAllowed = Math.max(
    SIDEBAR_MIN_WIDTH,
    Math.min(SIDEBAR_MAX_WIDTH, rect.width - MAIN_MIN_WIDTH)
  );
  if (sidebarWidth.value > maxAllowed) {
    sidebarWidth.value = maxAllowed;
  }
  if (sidebarWidth.value < SIDEBAR_MIN_WIDTH) {
    sidebarWidth.value = SIDEBAR_MIN_WIDTH;
  }
}

function startSidebarResize(event) {
  isSidebarResizing.value = true;
  document.addEventListener('mousemove', doSidebarResize);
  document.addEventListener('mouseup', stopSidebarResize);
  event.preventDefault();
}

function doSidebarResize(event) {
  if (!isSidebarResizing.value) {
    return;
  }
  const container = layoutRef.value;
  if (!container) {
    return;
  }
  const rect = container.getBoundingClientRect();
  let nextWidth = event.clientX - rect.left;
  const maxAllowed = Math.max(
    SIDEBAR_MIN_WIDTH,
    Math.min(SIDEBAR_MAX_WIDTH, rect.width - MAIN_MIN_WIDTH)
  );
  nextWidth = Math.max(SIDEBAR_MIN_WIDTH, Math.min(nextWidth, maxAllowed));
  sidebarWidth.value = Math.round(nextWidth);
}

function stopSidebarResize() {
  isSidebarResizing.value = false;
  document.removeEventListener('mousemove', doSidebarResize);
  document.removeEventListener('mouseup', stopSidebarResize);
}

onMounted(() => {
  clampSidebarWidth();
  window.addEventListener('resize', clampSidebarWidth);
  EventsOn("shotgunContextGenerated", (output) => {
    addLog("Wails event: shotgunContextGenerated RECEIVED", 'debug', 'bottom');

    let contextOutput = '';
    let contextTokens = null;
    if (typeof output === 'string') {
      contextOutput = output;
    } else if (output && typeof output === 'object') {
      contextOutput = typeof output.output === 'string' ? output.output : '';
      contextTokens = typeof output.tokens === 'number' ? output.tokens : null;
    }

    if (shotgunPromptContext.value !== contextOutput) {
      shotgunPromptContext.value = contextOutput;
      // Context changed. If we are NOT on Step 2 (which handles live updates),
      // clear the stale finalPrompt so it regenerates when Step 2 mounts.
      if (currentStep.value !== 2) {
        finalPrompt.value = '';
      }
    }
    shotgunPromptContextTokens.value = contextTokens;

    isGeneratingContext.value = false;
    addLog(`Shotgun context updated (${contextOutput.length} chars).`, 'success');
    if (contextRegenerationPending.value) {
      contextRegenerationPending.value = false;
      debouncedTriggerShotgunContextGeneration();
    }
    const step1 = steps.value.find(s => s.id === 1);
    if (step1 && !step1.completed) {
        step1.completed = true;
    }
    if (currentStep.value === 1 && centralPanelRef.value?.updateStep2ShotgunContext) {
        centralPanelRef.value.updateStep2ShotgunContext(contextOutput);
    }
    checkAndProcessPendingFileTreeReload(); // Check after context generation
  });

  EventsOn("shotgunContextError", (errorMsg) => {
    addLog(`Wails event: shotgunContextError RECEIVED: ${errorMsg}`, 'debug', 'bottom');
    shotgunPromptContext.value = "Error: " + errorMsg;
    shotgunPromptContextTokens.value = null;
    isGeneratingContext.value = false;
    addLog(`Error generating context: ${errorMsg}`, 'error');
    if (contextRegenerationPending.value) {
      contextRegenerationPending.value = false;
      debouncedTriggerShotgunContextGeneration();
    }
    checkAndProcessPendingFileTreeReload(); // Check after context generation error
  });

  EventsOn("shotgunContextGenerationProgress", (progress) => {
    // console.log("FE: Progress event:", progress); // For debugging in Browser console
    generationProgressData.value = progress;
  });
  EventsOn("autoContextError", (message) => {
    isAutoContextLoading.value = false;
    addLog(`Auto context error: ${message}`, 'error', 'bottom');
  });

  // Get platform information
  (async () => {
    try {
      const envInfo = await Environment();
      platform.value = envInfo.platform;
      addLog(`Platform detected: ${platform.value}`, 'debug');
    } catch (err) {
      addLog(`Error getting platform: ${err}`, 'error');
      // platform.value remains 'unknown' as fallback
    }
  })();
  refreshLlmSettingsState();

  (async () => {
    try {
      const texture = await GetAutoContextButtonTexture();
      if (texture && typeof texture === 'string' && texture.length > 0) {
        autoContextButtonTexture.value = texture;
        document.documentElement.style.setProperty(
          '--auto-context-button-bg',
          `url(${texture})`
        );
      }
    } catch (err) {
      addLog(`Failed to load auto-context button texture: ${err?.message || err}`, 'error', 'bottom');
    }
  })();

  (async () => {
    try {
      const roots = await GetProjectRoots();
      if (Array.isArray(roots)) {
        projectRoots.value = roots;
        if (roots.length > 0) {
          addLog(`Loaded ${roots.length} project root(s) from settings.`, 'debug');
        }
      }
    } catch (err) {
      addLog(`Failed to load project roots: ${err?.message || err}`, 'error', 'bottom');
    }
  })();

  unlistenProjectFilesChanged = EventsOn("projectFilesChanged", (changedRootDir) => {
    const roots = projectRootPaths.value;
    if (!roots.includes(changedRootDir)) {
      addLog(`Watchman: Ignoring event for ${changedRootDir}, not in active roots.`, 'debug');
      return;
    }
    addLog(`Watchman: Event "projectFilesChanged" received for ${changedRootDir}.`, 'debug');
    if (isFileTreeLoading.value || isGeneratingContext.value) {
      projectFilesChangedPendingReload.value = true;
      addLog("Watchman: File change detected, reload queued as system is busy.", 'info');
    } else {
      addLog("Watchman: File change detected, reloading tree immediately.", 'info');
      loadFileTree(projectRootPaths.value); // This will set isFileTreeLoading = true
      // loadFileTree will schedule context regeneration after refresh
    }
  });
});

onBeforeUnmount(async () => {
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  document.removeEventListener('mousemove', doSidebarResize);
  document.removeEventListener('mouseup', stopSidebarResize);
  window.removeEventListener('resize', clampSidebarWidth);
  clearTimeout(debounceTimer);
  if (projectRootPaths.value.length > 0) {
    await StopFileWatcher().catch(err => console.error("Error stopping file watcher on unmount:", err));
    addLog(`File watcher stopped on component unmount for ${projectRootPaths.value.length} root(s)`, 'debug');
  }
  if (unlistenProjectFilesChanged) {
    unlistenProjectFilesChanged();
  }
  // Remember to unlisten other events if they return unlistener functions
});


watch(projectRootPaths, async (newRoots, oldRoots) => {
  const previousRoots = Array.isArray(oldRoots) ? oldRoots : [];
  if (previousRoots.length > 0) {
    await StopFileWatcher().catch(err => addLog(`Error stopping watcher for ${previousRoots.length} root(s): ${err}`, 'error'));
    addLog(`File watcher stopped for ${previousRoots.length} root(s)`, 'debug');
  }
  if (newRoots && newRoots.length > 0) {
    await StartFileWatcher(newRoots).catch(err => addLog(`Error starting watcher for ${newRoots.length} root(s): ${err}`, 'error'));
    addLog(`File watcher started for ${newRoots.length} root(s)`, 'debug');
    await loadFileTree(newRoots);
  } else {
    fileTree.value = [];
    shotgunPromptContext.value = '';
    loadingError.value = '';
    manuallyToggledNodes.clear();
    isGeneratingContext.value = false; // Reset generation state
    projectFilesChangedPendingReload.value = false; // Reset pending reload
  }
}, { deep: true });

// Helper function to process pending reloads
function checkAndProcessPendingFileTreeReload() {
  if (projectFilesChangedPendingReload.value && !isFileTreeLoading.value && !isGeneratingContext.value) {
    projectFilesChangedPendingReload.value = false;
    addLog("Watchman: Processing queued file tree reload.", 'info');
    // It's important that loadFileTree correctly sets isFileTreeLoading to true at its start
    // and that subsequent context generation is also handled.
    loadFileTree(projectRootPaths.value);
  }
}

function handleCustomRulesUpdated() {
  addLog("Custom ignore rules updated by user. Reloading file tree.", 'info');
  if (projectRootPaths.value.length > 0) {
    // This will call ListFiles in Go, which will use the new custom rules from app.settings.
    // The new tree will have updated IsCustomIgnored flags.
    // loadFileTree will schedule context regeneration after refresh.
    loadFileTree(projectRootPaths.value);
  }
}

function handleUserTaskUpdate(val) {
  if (userTask.value !== val) {
    userTask.value = val;
    if (currentStep.value !== 2) {
      finalPrompt.value = '';
    }
  }
}

function handleRulesContentUpdate(val) {
  if (rulesContent.value !== val) {
    rulesContent.value = val;
    if (currentStep.value !== 2) {
      finalPrompt.value = '';
    }
  }
}

async function refreshLlmSettingsState() {
  try {
    llmSettings.value = await GetLlmSettings();
    hasActiveLlmKey.value = await HasActiveLlmKey();
  } catch (err) {
    addLog(`Failed to load LLM settings: ${err?.message || err}`, 'error', 'bottom');
  }
}

function openLlmSettingsModal() {
  isLlmSettingsModalVisible.value = true;
}

function closeLlmSettingsModal() {
  isLlmSettingsModalVisible.value = false;
}

async function handleLlmSettingsSaved() {
  await refreshLlmSettingsState();
  addLog('LLM settings updated.', 'success', 'bottom');
}

function applyAutoSelection(selectionByRoot) {
  if (!selectionByRoot || typeof selectionByRoot !== 'object') {
    addLog('Auto context returned an empty selection.', 'warn', 'bottom');
    return;
  }

  const selectionSets = new Map();
  let totalSelected = 0;
  Object.entries(selectionByRoot).forEach(([rootPath, paths]) => {
    if (!Array.isArray(paths)) {
      return;
    }
    const normalizedSet = new Set(
      paths.map((path) => normalizeRelPath(path)).filter((path) => path && path !== '.')
    );
    if (normalizedSet.size > 0) {
      selectionSets.set(rootPath, normalizedSet);
      totalSelected += normalizedSet.size;
    }
  });

  if (selectionSets.size === 0) {
    addLog('Auto context did not include any valid paths.', 'warn', 'bottom');
    return;
  }

  const markNode = (node) => {
    if (!node) return false;
    const rootPath = node.rootPath;
    const normalized = normalizeRelPath(node.relPath);
    const selectedSet = rootPath ? selectionSets.get(rootPath) : null;
    let includeSelf = selectedSet ? selectedSet.has(normalized) : false;
    if (node.children && node.children.length > 0) {
      let childIncluded = false;
      node.children.forEach((child) => {
        if (markNode(child)) {
          childIncluded = true;
        }
      });
      includeSelf = includeSelf || childIncluded;
    }
    node.excluded = !includeSelf;
    manuallyToggledNodes.set(nodeToggleKey(node), node.excluded);
    return includeSelf;
  };

  manuallyToggledNodes.clear();
  fileTree.value.forEach((node) => markNode(node));
  updateAllNodesExcludedState(fileTree.value);
  addLog(`Auto context selected ${totalSelected} paths.`, 'success', 'bottom');
  debouncedTriggerShotgunContextGeneration();
}

async function requestAutoContextSelection() {
  if (!hasProjectRoots.value) {
    addLog('Select at least one project folder before running auto context.', 'warn', 'bottom');
    return;
  }
  if (!hasActiveLlmKey.value) {
    addLog('Configure an LLM provider before requesting auto context.', 'warn', 'bottom');
    openLlmSettingsModal();
    return;
  }
  if (isAutoContextLoading.value) {
    return;
  }
  isAutoContextLoading.value = true;
  addLog('Requesting auto context selection…', 'info', 'bottom');
  try {
    // For Auto context we want the project tree that is NOT filtered by user selections,
    // only reduced by .gitignore and ignore.glob rules (when enabled).
    const excludedByRoot = buildIgnoredPathsPayloadForAutoContext();
    const selection = await RequestAutoContextSelection(
      projectRootPaths.value,
      excludedByRoot,
      userTask.value || ''
    );
    if (selection && typeof selection === 'object' && Object.keys(selection).length > 0) {
      applyAutoSelection(selection);
    } else {
      addLog('Auto context call completed but returned no files.', 'warn', 'bottom');
    }
  } catch (err) {
    addLog(`Auto context failed: ${err?.message || err}`, 'error', 'bottom');
  } finally {
    isAutoContextLoading.value = false;
  }
}

</script>

<style scoped>
.flex-1 {
  min-height: 0;
}
</style> 
