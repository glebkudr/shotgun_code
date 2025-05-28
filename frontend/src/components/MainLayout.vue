<template>
  <div class="flex flex-col h-screen w-screen bg-primary overflow-hidden">
    <!-- Top Bar -->
    <TopBar @toggle-settings="toggleSettings" />
    
    <div class="flex flex-1 overflow-hidden">
      <!-- Left Sidebar - Navigation Only -->
      <LeftSidebar 
        :current-step="currentStep" 
        :steps="steps" 
        :width="leftSidebarWidth"
        @navigate="navigateToStep" 
        @resize="handleLeftSidebarResize"
      />
      <!-- Central Panel -->
      <CentralPanel 
        :current-step="currentStep" 
        :shotgun-prompt-context="shotgunPromptContext"
        :file-structure-context="fileStructureContext"
        :generation-progress="generationProgressData"
        :is-generating-context="isGeneratingContext"
        :project-root="projectRoot" 
        :platform="platform"
        :user-task="userTask"
        :rules-content="rulesContent"
        :split-diffs="splitDiffs"
        :is-loading-split-diffs="isLoadingSplitDiffs"
        :final-prompt="finalPrompt"
        :split-line-limit-value="splitLineLimitValue"
        :shotgun-git-diff="shotgunGitDiff"
        :selected-template="selectedTemplate"
        @step-action="handleStepAction"
        @update-composed-prompt="handleComposedPromptUpdate"
        @update:user-task="handleUserTaskUpdate"
        @update:rules-content="handleRulesContentUpdate"
        @update:shotgunGitDiff="handleShotgunGitDiffUpdate"
        @update:splitLineLimit="handleSplitLineLimitUpdate"
        ref="centralPanelRef" 
        class="flex-1"
      />
      
      <!-- Right Sidebar - Context Sensitive -->
      <RightSidebar
        :current-step="currentStep"
        :project-root="projectRoot"
        :file-tree-nodes="fileTree"
        :use-gitignore="useGitignore"
        :use-custom-ignore="useCustomIgnore"
        :loading-error="loadingError"
        :rules-content="rulesContent"
        :final-prompt="finalPrompt"
        :selected-template="selectedTemplate"
        :width="rightSidebarWidth"
        @select-folder="selectProjectFolderHandler"
        @toggle-gitignore="toggleGitignoreHandler"
        @toggle-custom-ignore="toggleCustomIgnoreHandler"
        @toggle-exclude="toggleExcludeNode"
        @custom-rules-updated="handleCustomRulesUpdated"
        @template-change="handleTemplateChange"
        @update:rules-content="handleRulesContentUpdate"
        @step-completed="handleStepCompletion"
        @resize="handleRightSidebarResize"
      />
    </div>
    <div 
      v-if="isConsoleVisible"
      @mousedown="startResize"
      class="console-resize-handle"
      title="Resize console height"
    ></div>
    <div class="status-bar">
      <div class="status-item" v-if="currentStep >= 2">
        <span class="status-item-icon">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
          </svg>
        </span>
        <span class="token-counter">
          <span class="font-normal">Token: </span> 
          <span class="font-medium">{{ approximateTokens }}</span>
          <span class="text-xs text-hint ml-2">(approximation)</span>
        </span>
      </div>
      <div v-else class="status-item">
        <span class="status-item-icon status-item-warning">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </span>
        <span>No prompt composed yet</span>
      </div>

      <div class="flex-grow"></div>

      <button 
        @click="toggleConsole" 
        class="status-item"
        :title="isConsoleVisible ? 'Hide console' : 'Show console'"
      >
        <span class="status-item-icon">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" :class="{'transform rotate-180': !isConsoleVisible}">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
          </svg>
        </span>
        <span>{{ isConsoleVisible ? 'Hide Console' : 'Show Console' }}</span>
      </button>
    </div>
    <BottomConsole v-if="isConsoleVisible" :log-messages="logMessages" :height="consoleHeight" ref="bottomConsoleRef" />
  </div>
</template>

<script setup>
import { ref, reactive, watch, onMounted, onBeforeUnmount, nextTick, computed } from 'vue';
import LeftSidebar from './LeftSidebar.vue';
import CentralPanel from './CentralPanel.vue';
import BottomConsole from './BottomConsole.vue';
import TopBar from './TopBar.vue';
import RightSidebar from './RightSidebar.vue';
import { ListFiles, RequestShotgunContextGeneration, SelectDirectory as SelectDirectoryGo, StartFileWatcher, StopFileWatcher, SetUseGitignore, SetUseCustomIgnore, SplitShotgunDiff } from '../../wailsjs/go/main/App';
import { EventsOn, Environment } from '../../wailsjs/runtime/runtime';
import themeStore from '../theme.js';

const currentStep = ref(1);
const steps = ref([
  { id: 1, title: 'Prepare Context', completed: false, description: 'Select project folder, review files, and generate the initial project context for the LLM.' },
  { id: 2, title: 'Compose Prompt', completed: false, description: 'Provide a prompt to the LLM based on the project context to generate a code diff.' },
  { id: 3, title: 'Execute Prompt', completed: false, description: 'Paste a large shotgunDiff and split it into smaller, manageable parts.' },
  { id: 4, title: 'Apply Patch', completed: false, description: 'Copy and apply the smaller diff parts to your project.' },
]);

const logMessages = ref([]);
const centralPanelRef = ref(null); 
const bottomConsoleRef = ref(null);
const consoleMinHeight = 50;
const consoleHeight = ref(parseInt(localStorage.getItem('shotgun-console-height')) || 150);
const isConsoleVisible = ref(false);

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
const fileStructureContext = ref('');
const loadingError = ref('');
const useGitignore = ref(true);
const useCustomIgnore = ref(true);
const manuallyToggledNodes = reactive(new Map());
const isGeneratingContext = ref(false);
const generationProgressData = ref({ current: 0, total: 0 });
const isFileTreeLoading = ref(false);
const composedLlmPrompt = ref('');
const platform = ref('unknown');
const userTask = ref('');
const rulesContent = ref('');
const finalPrompt = ref('');
const isLoadingSplitDiffs = ref(false);
const splitDiffs = ref([]);
const shotgunGitDiff = ref('');
const splitLineLimitValue = ref(0);
const rightSidebarWidth = ref(parseInt(localStorage.getItem('shotgun-right-sidebar-width')) || 250);
const leftSidebarWidth = ref(parseInt(localStorage.getItem('shotgun-left-sidebar-width')) || 250);
const isResizing = ref(false);
const startX = ref(0);
const startWidth = ref(0);
const selectedTemplate = ref('dev');
let isExcludedStateChanging = false;
let debounceTimer = null;

// Token count calculation
const approximateTokens = computed(() => {
  const combinedText = `${userTask.value || ''} ${finalPrompt.value || ''}`.trim();
  const tokens = Math.round(combinedText.length / 3);
  return tokens.toLocaleString();
});

const charCountColorClass = computed(() => {
  const count = charCount.value;
  if (count < 1000000) {
    return 'text-success';
  } else if (count <= 4000000) {
    return 'text-warning';
  } else {
    return 'text-error';
  }
});

// Watcher related
const projectFilesChangedPendingReload = ref(false);
let unlistenProjectFilesChanged = null;

async function selectProjectFolderHandler() {
  isFileTreeLoading.value = true;
  try {  
    const selectedDir = await SelectDirectoryGo(); 
    if (selectedDir) {
      if (selectedDir !== projectRoot.value) {
        shotgunPromptContext.value = '';
        fileStructureContext.value = '';
        finalPrompt.value = '';
        userTask.value = '';
        isGeneratingContext.value = false;
        generationProgressData.value = { current: 0, total: 0 };
        
        steps.value.forEach(step => {
          step.completed = false;
        });
        
        projectRoot.value = selectedDir;
        loadingError.value = '';
        manuallyToggledNodes.clear();
        fileTree.value = [];
        splitDiffs.value = []; 

        await loadFileTree(selectedDir);

        if (!isFileTreeLoading.value) {
           addLog('Triggering context generation for new project', 'info', 'bottom');
           await debouncedTriggerShotgunContextGeneration();
        }

        addLog(`New project folder selected: ${selectedDir}`, 'info', 'bottom');
      } else {
        addLog(`Same project folder selected again: ${selectedDir}`, 'info', 'bottom');
      }
      
      if (currentStep.value !== 1) {
        currentStep.value = 1;
      }
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
    if (current.excluded) {
      return true;
    }
    current = current.parent;
  }
  return false;
}

function toggleExcludeNode(nodeToToggle) {
  if (isAnyParentVisuallyExcluded(nodeToToggle) && nodeToToggle.excluded) {
    nodeToToggle.excluded = false;
  } else {
    nodeToToggle.excluded = !nodeToToggle.excluded;
  }
  manuallyToggledNodes.set(nodeToToggle.relPath, nodeToToggle.excluded);
  addLog(`Toggled exclusion for ${nodeToToggle.name} to ${nodeToToggle.excluded}`, 'info', 'bottom');
  
  isExcludedStateChanging = true;
  regenerateContextIfNeeded();
}

function updateAllNodesExcludedState(nodesToUpdate) {
  _updateAllNodesExcludedStateRecursive(nodesToUpdate, false);
}

function _updateAllNodesExcludedStateRecursive(nodesToUpdate, parentIsVisuallyExcluded) {
   if (!nodesToUpdate || nodesToUpdate.length === 0) return;
   nodesToUpdate.forEach(node => {
    const manualToggle = manuallyToggledNodes.get(node.relPath);
    let isExcludedByRule = false;
    if (useGitignore.value && node.isGitignored) isExcludedByRule = true;
    if (useCustomIgnore.value && node.isCustomIgnored) isExcludedByRule = true;

    if (manualToggle !== undefined) {
      node.excluded = manualToggle;
    } else {
      node.excluded = isExcludedByRule || parentIsVisuallyExcluded;
    }

     if (node.children && node.children.length > 0) {
      _updateAllNodesExcludedStateRecursive(node.children, node.excluded);
     }
   });
 }

function toggleGitignoreHandler(value) {
  useGitignore.value = value;
  addLog(`.gitignore usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseGitignore(value)
    .then(() => addLog(`Watchman instructed to use .gitignore: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useGitignore in backend: ${err}`, 'error'));
}

function toggleCustomIgnoreHandler(value) {
  useCustomIgnore.value = value;
  addLog(`Custom ignore rules usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseCustomIgnore(value)
    .then(() => addLog(`Watchman instructed to use custom ignores: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useCustomIgnore in backend: ${err}`, 'error'));
}

function debouncedTriggerShotgunContextGeneration() {
  if (!projectRoot.value) {
    shotgunPromptContext.value = '';
    generationProgressData.value = { current: 0, total: 0 }; 
    isGeneratingContext.value = false;
    return;
  }
  
  shotgunPromptContext.value = '';

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
    generationProgressData.value = { current: 0, total: 0 }; 

    const excludedPathsArray = [];
    
    function hasVisuallyIncludedDescendant(node) {
      if (!node.isDir || !node.children || node.children.length === 0) {
        return false;
      }
      for (const child of node.children) {
        if (!child.excluded) {
          return true;
        }
        if (hasVisuallyIncludedDescendant(child)) { 
          return true;
        }
      }
      return false;
    }

    function collectTrulyExcludedPaths(nodes) {
       if (!nodes) return;
       nodes.forEach(node => {
        if (node.excluded && !hasVisuallyIncludedDescendant(node)) {
          excludedPathsArray.push(node.relPath);
        } else {
          if (node.children && node.children.length > 0) {
            collectTrulyExcludedPaths(node.children);
          }
        }
       });
     }
    collectTrulyExcludedPaths(fileTree.value);
 
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
  if (stepId < currentStep.value) {
    steps.value.forEach(step => {
      if (step.id > stepId && step.completed) {
        step.completed = false;
        addLog(`Reset step ${step.id} completion status`, 'info', 'bottom');
      }
    });
    
    currentStep.value = stepId;
    return;
  }

  if (stepId === 2) {
    if (!projectRoot.value) {
      addLog("Cannot proceed to Step 2: No project folder selected.", 'error');
      return;
    }
    
    if (!shotgunPromptContext.value) {
      addLog("Project folder selected but context not generated. Generating context...", 'info');
      debouncedTriggerShotgunContextGeneration();
    }

    fileStructureContext.value = shotgunPromptContext.value || '';
  }

  const firstUncompletedStep = steps.value.find(s => !s.completed);
  if (!firstUncompletedStep || stepId === firstUncompletedStep.id) {
    const previousStep = currentStep.value;
    currentStep.value = stepId;

    if (stepId > previousStep) {
      nextTick(() => {
        if (stepId === 2 && centralPanelRef.value?.step2Ref?.checkCompletion) {
          centralPanelRef.value.step2Ref.checkCompletion();
        } else if (stepId === 3 && centralPanelRef.value?.step3Ref?.checkCompletion) {
          centralPanelRef.value.step3Ref.checkCompletion();
        } else if (stepId === 4 && centralPanelRef.value?.step4Ref?.checkCompletion) {
          centralPanelRef.value.step4Ref.checkCompletion();
        }
      });
    }
  } else {
    addLog(`Cannot skip to step ${stepId}. Complete step ${firstUncompletedStep.id} first.`, 'warn');
  }
}

function handleComposedPromptUpdate(prompt) {
  composedLlmPrompt.value = prompt;
  finalPrompt.value = prompt;
  addLog(`MainLayout: Composed LLM prompt updated (${prompt.length} chars).`, 'debug', 'bottom');
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
  }

  const currentStepObj = steps.value.find(s => s.id === currentStep.value);
  
  switch (actionName) {
    case 'generateShotgunContext':
      debouncedTriggerShotgunContextGeneration();
      break;
    case 'executePrompt':
      navigateToStep(4); 
      break;
    case 'executePromptAndSplitDiff':
      if (!payload || !payload.gitDiff || payload.lineLimit <= 0) {
        addLog("Invalid payload for splitting diff.", 'error', 'bottom');
        return;
      }
      addLog(`Splitting diff (approx ${payload.lineLimit} lines per split)...`, 'info', 'bottom');
      isLoadingSplitDiffs.value = true;
      splitDiffs.value = [];
      shotgunGitDiff.value = payload.gitDiff;
      splitLineLimitValue.value = payload.lineLimit;
      try {
        const result = await SplitShotgunDiff(payload.gitDiff, payload.lineLimit);
        splitDiffs.value = result;
        addLog(`Diff split into ${result.length} parts.`, 'success', 'bottom');
        
        if (currentStepObj) currentStepObj.completed = true;
        navigateToStep(4);
      } catch (err) {
        const errorMsg = `Error splitting diff: ${err.message || err}`;
        addLog(errorMsg, 'error', 'bottom');
      } finally {
        isLoadingSplitDiffs.value = false;
      }
      break;
    case 'applySelectedPatches':
    case 'applyAllPatches':
      addLog(`Simulating backend: Applying patches (${actionName})...`, 'info', 'bottom');
      await new Promise(resolve => setTimeout(resolve, 1000));
      addLog('Backend: Patches applied. Process complete!', 'info', 'bottom');
      if (currentStepObj) currentStepObj.completed = true;
      break;
    case 'finishSplitting':
      addLog("Finished with split diffs.", 'info', 'bottom');
      if (currentStepObj) currentStepObj.completed = true;
      break;
    default:
      addLog(`Unknown action: ${actionName}`, 'error', 'bottom');
  }
}

const tempConsoleHeight = ref(0);


function toggleConsole() {
  isConsoleVisible.value = !isConsoleVisible.value;
  
  if (isConsoleVisible.value && consoleHeight.value <= consoleMinHeight) {
    consoleHeight.value = 150;
  }
}

function startResize(event) {
  isResizing.value = true;
  tempConsoleHeight.value = consoleHeight.value;
  document.documentElement.classList.add('resize-transition-paused');
  document.addEventListener('mousemove', doResize);
  document.addEventListener('mouseup', stopResize);
  event.preventDefault(); 
}

function doResize(event) {
  if (!isResizing.value) return;
  const newHeight = window.innerHeight - event.clientY;
  const minHeight = consoleMinHeight;
  const maxHeight = window.innerHeight * 0.7;
  
  consoleHeight.value = Math.max(minHeight, Math.min(newHeight, maxHeight));
  tempConsoleHeight.value = consoleHeight.value;
}

function stopResize() {
  isResizing.value = false;
  document.documentElement.classList.remove('resize-transition-paused');
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  localStorage.setItem('shotgun-console-height', consoleHeight.value);
}

function handleLeftSidebarResize(newWidth) {
  leftSidebarWidth.value = newWidth;
  localStorage.setItem('shotgun-left-sidebar-width', newWidth);
}

function handleRightSidebarResize(newWidth) {
  rightSidebarWidth.value = newWidth;
  localStorage.setItem('shotgun-right-sidebar-width', newWidth);
}

onMounted(() => {
  // Load rules content from local storage if available
  const savedRules = localStorage.getItem('shotgun-rules-content');
  if (savedRules) {
    rulesContent.value = savedRules;
    addLog('Loaded custom rules from local storage', 'debug', 'bottom');
  }

  EventsOn("shotgunContextGenerated", (output) => {
    addLog("Wails event: shotgunContextGenerated RECEIVED", 'debug', 'bottom');
    shotgunPromptContext.value = output;
    fileStructureContext.value = output;
    isGeneratingContext.value = false;
    addLog(`Shotgun context updated (${output.length} chars).`, 'success');
    const step1 = steps.value.find(s => s.id === 1);
    if (step1 && !step1.completed) {
        step1.completed = true;
    }
    if (currentStep.value === 1 && centralPanelRef.value?.updateStep2ShotgunContext) {
        centralPanelRef.value.updateStep2ShotgunContext(output);
    }
    checkAndProcessPendingFileTreeReload();
  });

  EventsOn("shotgunContextError", (errorMsg) => {
    addLog(`Wails event: shotgunContextError RECEIVED: ${errorMsg}`, 'debug', 'bottom');
    shotgunPromptContext.value = "Error: " + errorMsg;
    isGeneratingContext.value = false;
    addLog(`Error generating context: ${errorMsg}`, 'error');
    checkAndProcessPendingFileTreeReload();
  });

  EventsOn("shotgunContextGenerationProgress", (progress) => {
    generationProgressData.value = progress;
  });

  (async () => {
    try {
      const envInfo = await Environment();
      platform.value = envInfo.platform;
      addLog(`Platform detected: ${platform.value}`, 'debug');
    } catch (err) {
      addLog(`Error getting platform: ${err}`, 'error');
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

// Watch for checkbox/exclusion changes
function regenerateContextIfNeeded() {
  if (isFileTreeLoading.value || !isExcludedStateChanging) {
    return;
  }
  
  addLog("Regenerating context due to exclusion changes", 'debug', 'bottom');
  isExcludedStateChanging = false;
  debouncedTriggerShotgunContextGeneration();
}

// Watch for gitignore and custom ignore changes
watch([useGitignore, useCustomIgnore], ([newUseGitignore, newUseCustomIgnore], [oldUseGitignore, oldUseCustomIgnore]) => {
  if (isFileTreeLoading.value) {
    addLog("Ignore settings changed during file tree load, generation deferred.", 'debug', 'bottom');
    return;
  }
  
  addLog("Watcher detected changes in useGitignore or useCustomIgnore. Re-evaluating context.", 'debug', 'bottom');
  updateAllNodesExcludedState(fileTree.value);
  debouncedTriggerShotgunContextGeneration();
});

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
  userTask.value = val;
}

function handleRulesContentUpdate(val) {
  rulesContent.value = val;
  localStorage.setItem('shotgun-rules-content', val);
}

function handleShotgunGitDiffUpdate(val) {
  shotgunGitDiff.value = val;
}

function handleSplitLineLimitUpdate(val) {
  splitLineLimitValue.value = val;
}

function handleTemplateChange(templateKey) {
  selectedTemplate.value = templateKey;
  if (currentStep.value === 2) {
    debouncedTriggerShotgunContextGeneration();
  }
  
  addLog(`Changed prompt template to: ${templateKey}`, 'info');
}

function toggleSettings() {
  addLog('Settings toggled', 'info', 'bottom');
}

function handleStepCompletion(stepId) {
  const step = steps.value.find(s => s.id === stepId);
  if (step) {
    step.completed = true;
    addLog(`Step ${stepId} marked as completed`, 'info');
    
    if (currentStep.value === stepId) {
      const nextStepId = stepId + 1;
      const nextStep = steps.value.find(s => s.id === nextStepId);
      if (nextStep) {
        navigateToStep(nextStepId);
        addLog(`Navigating to step ${nextStepId}`, 'info');
      }
    }
  }
}

</script>
