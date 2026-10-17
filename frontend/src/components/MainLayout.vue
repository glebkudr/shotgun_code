<template>
  <div class="flex flex-col h-screen bg-gray-100">
    <HorizontalStepper :current-step="currentStep" :steps="steps" @navigate="navigateToStep" :key="`hstepper-${currentStep}-${steps.map(s=>s.completed).join('')}`" />
    <div class="flex flex-1 overflow-hidden">
      <LeftSidebar 
        v-if="currentStep !== 3"
        :current-step="currentStep" 
        :steps="steps" 
        :project-root="projectRoot"
        :file-tree-nodes="fileTree"
        :use-gitignore="useGitignore"
        :use-custom-ignore="useCustomIgnore"
        :loading-error="loadingError"
        @navigate="navigateToStep"
        @select-folder="selectProjectFolderHandler"
        @toggle-gitignore="toggleGitignoreHandler"
        @toggle-custom-ignore="toggleCustomIgnoreHandler"
        @toggle-exclude="toggleExcludeNode"
        @custom-rules-updated="handleCustomRulesUpdated"
        @add-log="({message, type}) => addLog(message, type)" />
      <CentralPanel :current-step="currentStep" 
                    :shotgun-prompt-context="shotgunPromptContext"
                    :generation-progress="generationProgressData"
                    :is-generating-context="isGeneratingContext"
                    :project-root="projectRoot" 
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
import { ref, reactive, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
import HorizontalStepper from './HorizontalStepper.vue';
import LeftSidebar from './LeftSidebar.vue';
import CentralPanel from './CentralPanel.vue';
import BottomConsole from './BottomConsole.vue';
import LlmSettingsModal from './LlmSettingsModal.vue';
import {
  ListFiles,
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
const consoleHeight = ref(MIN_CONSOLE_HEIGHT); // Initial height in pixels

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

const projectRoot = ref('');
const fileTree = ref([]);
const shotgunPromptContext = ref('');
const loadingError = ref('');
const useGitignore = ref(true);
const useCustomIgnore = ref(true);
const manuallyToggledNodes = reactive(new Map());
const isGeneratingContext = ref(false);
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
  isFileTreeLoading.value = true;
  try {
    shotgunPromptContext.value = '';
    isGeneratingContext.value = false;
    const selectedDir = await SelectDirectoryGo(); 
    if (selectedDir) {
      projectRoot.value = selectedDir;
      loadingError.value = '';
      manuallyToggledNodes.clear();
      fileTree.value = [];
      
      await loadFileTree(selectedDir);

      if (!isFileTreeLoading.value && projectRoot.value) {
         debouncedTriggerShotgunContextGeneration();
      }

      steps.value.forEach(s => s.completed = false);
      currentStep.value = 1;
      addLog(`Project folder selected: ${selectedDir}`, 'info', 'bottom');
    } else {
      isFileTreeLoading.value = false;
    }
  } catch (err) {
    console.error("Error selecting directory:", err);
    const errorMsg = "Failed to select directory: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
    isFileTreeLoading.value = false;
  }
}

async function loadFileTree(dirPath) {
  isFileTreeLoading.value = true;
  loadingError.value = '';
  addLog(`Loading file tree for: ${dirPath}`, 'info', 'bottom');
  try {
    const treeData = await ListFiles(dirPath);
    fileTree.value = mapDataToTreeRecursive(treeData, null);
    updateAllNodesExcludedState(fileTree.value);
    addLog(`File tree loaded successfully. Root items: ${fileTree.value.length}`, 'info', 'bottom');
  } catch (err) {
    console.error("Error listing files:", err);
    const errorMsg = "Failed to load file tree: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
    fileTree.value = [];
  } finally {
    isFileTreeLoading.value = false;
    checkAndProcessPendingFileTreeReload();
  }
}

function calculateNodeExcludedState(node) {
  const manualToggle = manuallyToggledNodes.get(node.relPath);
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

function collectTrulyExcludedPaths(nodes, target) {
  if (!nodes || nodes.length === 0) return;
  nodes.forEach((node) => {
    if (node.excluded && !hasVisuallyIncludedDescendant(node)) {
      target.push(node.relPath);
    } else if (node.children && node.children.length > 0) {
      collectTrulyExcludedPaths(node.children, target);
    }
  });
}

function buildExcludedPathsPayload() {
  const excluded = [];
  collectTrulyExcludedPaths(fileTree.value, excluded);
  return excluded;
}

function collectIgnoredPathsOnly(nodes, target) {
  if (!nodes || nodes.length === 0) return;
  nodes.forEach((node) => {
    const ignoredByGit = useGitignore.value && node.isGitignored;
    const ignoredByCustom = useCustomIgnore.value && node.isCustomIgnored;
    const ignoredByRules = ignoredByGit || ignoredByCustom;

    if (ignoredByRules && node.relPath) {
      // For auto-context, we exclude whole branches based on the root ignored node.
      // Children may still be present in the UI tree (e.g. inside gitignored folders),
      // but for the auto-context tree it's enough to mark only the root path here.
      target.push(node.relPath);
      return;
    }

    if (node.children && node.children.length > 0) {
      collectIgnoredPathsOnly(node.children, target);
    }
  });
}

function buildIgnoredPathsPayloadForAutoContext() {
  const ignored = [];
  collectIgnoredPathsOnly(fileTree.value, ignored);
  return ignored;
}

function normalizeRelPath(value) {
  if (!value) return '';
  return value.replace(/\\/g, '/').replace(/^\.\//, '').replace(/^\/+/, '');
}

function toggleExcludeNode(nodeToToggle) {
  nodeToToggle.excluded = !nodeToToggle.excluded;
  manuallyToggledNodes.set(nodeToToggle.relPath, nodeToToggle.excluded);

  // When toggling a folder, clear manual overrides for all descendants
  // so they inherit the new state of the parent folder.
  if (nodeToToggle.isDir) {
    clearDescendantManualToggles(nodeToToggle);
  }

  updateAllNodesExcludedState(fileTree.value);
  addLog(`Toggled exclusion for ${nodeToToggle.name} to ${nodeToToggle.excluded}`, 'info', 'bottom');
}

function clearDescendantManualToggles(node) {
  if (node.children && node.children.length > 0) {
    node.children.forEach(child => {
      manuallyToggledNodes.delete(child.relPath);
      clearDescendantManualToggles(child);
    });
  }
}

function updateAllNodesExcludedState(nodesToUpdate) { // This is the public-facing function
  _updateAllNodesExcludedStateRecursive(nodesToUpdate, false, false);
}

function _updateAllNodesExcludedStateRecursive(nodesToUpdate, parentIsVisuallyExcluded, ancestorExplicitlyIncluded) {
  if (!nodesToUpdate || nodesToUpdate.length === 0) return;
  nodesToUpdate.forEach(node => {
    const manualToggle = manuallyToggledNodes.get(node.relPath);
    let isExcludedByRule = false;
    if (useGitignore.value && node.isGitignored) isExcludedByRule = true;
    if (useCustomIgnore.value && node.isCustomIgnored) isExcludedByRule = true;

    let isNodeExplicitlyIncluded = false;

    if (manualToggle !== undefined) {
      // Direct manual toggle by user overrides rules
      node.excluded = manualToggle;
      isNodeExplicitlyIncluded = !manualToggle;
    } else if (ancestorExplicitlyIncluded) {
      // Inherit explicit inclusion from an ancestor folder that was checked by user.
      // Custom rules maintain absolute priority and cannot be included.
      if (useCustomIgnore.value && node.isCustomIgnored) {
        node.excluded = true;
        isNodeExplicitlyIncluded = false;
      } else {
        node.excluded = false;
        isNodeExplicitlyIncluded = true;
      }
    } else {
      // Default rule evaluation or inheritance from excluded parent
      node.excluded = isExcludedByRule || parentIsVisuallyExcluded;
      isNodeExplicitlyIncluded = false;
    }

    if (node.children && node.children.length > 0) {
      _updateAllNodesExcludedStateRecursive(node.children, node.excluded, isNodeExplicitlyIncluded);
    }
  });
}

function toggleGitignoreHandler(value) {
  useGitignore.value = value;
  addLog(`.gitignore usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseGitignore(value)
    .then(() => addLog(`Watchman instructed to use .gitignore: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useGitignore in backend: ${err}`, 'error'));
  // Context regeneration is handled by the watch on [fileTree, useGitignore, useCustomIgnore]
  // which calls updateAllNodesExcludedState and debouncedTriggerShotgunContextGeneration.
}

function toggleCustomIgnoreHandler(value) {
  useCustomIgnore.value = value;
  addLog(`Custom ignore rules usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseCustomIgnore(value)
    .then(() => {
      addLog(`Watchman instructed to use custom ignores: ${value}`, 'debug');
      if (projectRoot.value) {
        loadFileTree(projectRoot.value);
      }
    })
    .catch(err => addLog(`Error setting useCustomIgnore in backend: ${err}`, 'error'));
}

function debouncedTriggerShotgunContextGeneration() {
  if (!projectRoot.value) {
    // Clear context and stop loading if no project root
    shotgunPromptContext.value = ''; // Clear previous context
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress
    // isGeneratingContext will be set to false by the return or by the timeout if it runs
    isGeneratingContext.value = false;
    return;
  }

  if (isFileTreeLoading.value) {
    addLog("Debounced trigger skipped: file tree is loading.", 'debug', 'bottom');
    isGeneratingContext.value = false;
    return;
  }

  if (!isGeneratingContext.value) nextTick(() => isGeneratingContext.value = true);

  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    if (!projectRoot.value) { 
        isGeneratingContext.value = false;
        return;
    }
    if (isFileTreeLoading.value) {
        addLog("Debounced execution skipped: file tree became loading.", 'debug', 'bottom');
        isGeneratingContext.value = false;
        return;
    }

    addLog("Debounced trigger: Requesting shotgun context generation...", 'info');
    
    updateAllNodesExcludedState(fileTree.value);
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress before new request

    const excludedPathsArray = buildExcludedPathsPayload();
 
     RequestShotgunContextGeneration(projectRoot.value, excludedPathsArray)
       .catch(err => {
        const errorMsg = "Error calling RequestShotgunContextGeneration: " + (err.message || err);
        addLog(errorMsg, 'error');
        shotgunPromptContext.value = "Error: " + errorMsg; 
      })
      .finally(() => {
         // isGeneratingContext.value = false;
      });
  }, 750); 
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

onMounted(() => {
  EventsOn("shotgunContextGenerated", (output) => {
    addLog("Wails event: shotgunContextGenerated RECEIVED", 'debug', 'bottom');
    
    if (shotgunPromptContext.value !== output) {
      shotgunPromptContext.value = output;
      // Context changed. If we are NOT on Step 2 (which handles live updates),
      // clear the stale finalPrompt so it regenerates when Step 2 mounts.
      if (currentStep.value !== 2) {
        finalPrompt.value = '';
      }
    }

    isGeneratingContext.value = false;
    addLog(`Shotgun context updated (${output.length} chars).`, 'success');
    const step1 = steps.value.find(s => s.id === 1);
    if (step1 && !step1.completed) {
        step1.completed = true;
    }
    if (currentStep.value === 1 && centralPanelRef.value?.updateStep2ShotgunContext) {
        centralPanelRef.value.updateStep2ShotgunContext(output);
    }
    checkAndProcessPendingFileTreeReload(); // Check after context generation
  });

  EventsOn("shotgunContextError", (errorMsg) => {
    addLog(`Wails event: shotgunContextError RECEIVED: ${errorMsg}`, 'debug', 'bottom');
    shotgunPromptContext.value = "Error: " + errorMsg;
    isGeneratingContext.value = false;
    addLog(`Error generating context: ${errorMsg}`, 'error');
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

  unlistenProjectFilesChanged = EventsOn("projectFilesChanged", (changedRootDir) => {
    if (changedRootDir !== projectRoot.value) {
      addLog(`Watchman: Ignoring event for ${changedRootDir}, current root is ${projectRoot.value}`, 'debug');
      return;
    }
    addLog(`Watchman: Event "projectFilesChanged" received for ${changedRootDir}.`, 'debug');
    if (isFileTreeLoading.value || isGeneratingContext.value) {
      projectFilesChangedPendingReload.value = true;
      addLog("Watchman: File change detected, reload queued as system is busy.", 'info');
    } else {
      addLog("Watchman: File change detected, reloading tree immediately.", 'info');
      loadFileTree(projectRoot.value); // This will set isFileTreeLoading = true
      // debouncedTriggerShotgunContextGeneration will be called by the watcher on fileTree if projectRoot is set
    }
  });
});

onBeforeUnmount(async () => {
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  clearTimeout(debounceTimer);
  if (projectRoot.value) {
    await StopFileWatcher().catch(err => console.error("Error stopping file watcher on unmount:", err));
    addLog(`File watcher stopped on component unmount for ${projectRoot.value}`, 'debug');
  }
  if (unlistenProjectFilesChanged) {
    unlistenProjectFilesChanged();
  }
  // Remember to unlisten other events if they return unlistener functions
});

watch([fileTree, useGitignore, useCustomIgnore], ([newFileTree, newUseGitignore, newUseCustomIgnore], [oldFileTree, oldUseGitignore, oldUseCustomIgnore]) => {
  if (isFileTreeLoading.value) {
    addLog("Watcher triggered during file tree load, generation deferred.", 'debug', 'bottom');
    return;
  }
  
  addLog("Watcher detected changes in fileTree, useGitignore, or useCustomIgnore. Re-evaluating context.", 'debug', 'bottom');
  updateAllNodesExcludedState(fileTree.value);
  debouncedTriggerShotgunContextGeneration();
}, { deep: true });

watch(projectRoot, async (newRoot, oldRoot) => {
  if (oldRoot) {
    await StopFileWatcher().catch(err => addLog(`Error stopping watcher for ${oldRoot}: ${err}`, 'error'));
    addLog(`File watcher stopped for ${oldRoot}`, 'debug');
  }
  if (newRoot) {
    // Existing logic to loadFileTree, clear errors, etc., happens in selectProjectFolderHandler
    // which sets projectRoot. Here we just ensure the watcher starts for the new root.
    await StartFileWatcher(newRoot).catch(err => addLog(`Error starting watcher for ${newRoot}: ${err}`, 'error'));
    addLog(`File watcher started for ${newRoot}`, 'debug');
  } else {
    // Project root cleared, ensure watcher is stopped (already handled by oldRoot check if it was set)
    fileTree.value = [];
    shotgunPromptContext.value = '';
    loadingError.value = '';
    manuallyToggledNodes.clear();
    isGeneratingContext.value = false; // Reset generation state
    projectFilesChangedPendingReload.value = false; // Reset pending reload
  }
}, { immediate: false }); // 'immediate: false' to avoid running on initial undefined -> '' or '' -> initial value if set by default

// Helper function to process pending reloads
function checkAndProcessPendingFileTreeReload() {
  if (projectFilesChangedPendingReload.value && !isFileTreeLoading.value && !isGeneratingContext.value) {
    projectFilesChangedPendingReload.value = false;
    addLog("Watchman: Processing queued file tree reload.", 'info');
    // It's important that loadFileTree correctly sets isFileTreeLoading to true at its start
    // and that subsequent context generation is also handled.
    loadFileTree(projectRoot.value);
  }
}

function handleCustomRulesUpdated() {
  addLog("Custom ignore rules updated by user. Reloading file tree.", 'info');
  if (projectRoot.value) {
    // This will call ListFiles in Go, which will use the new custom rules from app.settings.
    // The new tree will have updated IsCustomIgnored flags.
    // The watch on fileTree (and its subsequent call to debouncedTriggerShotgunContextGeneration)
    // will then handle regenerating the context.
    loadFileTree(projectRoot.value);
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

function applyAutoSelection(selectedRelativePaths) {
  if (!Array.isArray(selectedRelativePaths) || selectedRelativePaths.length === 0) {
    addLog('Auto context returned an empty selection.', 'warn', 'bottom');
    return;
  }
  const normalizedSet = new Set(
    selectedRelativePaths.map((path) => normalizeRelPath(path)).filter((path) => path && path !== '.')
  );
  if (normalizedSet.size === 0) {
    addLog('Auto context did not include any valid paths.', 'warn', 'bottom');
    return;
  }

  const markNode = (node) => {
    if (!node) return false;
    const normalized = normalizeRelPath(node.relPath);
    let includeSelf = normalized === '' || normalized === '.' || normalizedSet.has(normalized);
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
    manuallyToggledNodes.set(node.relPath, node.excluded);
    return includeSelf;
  };

  manuallyToggledNodes.clear();
  fileTree.value.forEach((node) => markNode(node));
  updateAllNodesExcludedState(fileTree.value);
  addLog(`Auto context selected ${normalizedSet.size} paths.`, 'success', 'bottom');
  debouncedTriggerShotgunContextGeneration();
}

async function requestAutoContextSelection() {
  if (!projectRoot.value) {
    addLog('Select a project folder before running auto context.', 'warn', 'bottom');
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
    const excludedPathsArray = buildIgnoredPathsPayloadForAutoContext();
    const selection = await RequestAutoContextSelection(
      projectRoot.value,
      excludedPathsArray,
      userTask.value || ''
    );
    if (Array.isArray(selection) && selection.length > 0) {
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
