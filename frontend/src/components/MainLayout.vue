<template>
  <div class="flex flex-col h-screen bg-gray-100">
    <HorizontalStepper :current-step="currentStep" :steps="steps" @navigate="navigateToStep" :key="`hstepper-${currentStep}-${steps.map(s=>s.completed).join('')}`" />
    <div class="flex flex-1 overflow-hidden">      <LeftSidebar 
        :current-step="currentStep" 
        :steps="steps" 
        :projects="projects"
        :all-file-nodes="allFileNodes"
        :use-gitignore="useGitignore"
        :use-custom-ignore="useCustomIgnore"
        :loading-error="loadingError"
        @navigate="navigateToStep"
        @add-project="addProjectHandler"
        @remove-project="removeProjectHandler"
        @toggle-gitignore="toggleGitignoreHandler"
        @toggle-custom-ignore="toggleCustomIgnoreHandler"
        @toggle-exclude="toggleExcludeNode"
        @custom-rules-updated="handleCustomRulesUpdated"
        @add-log="({message, type}) => addLog(message, type)" />      <CentralPanel :current-step="currentStep" 
                    :shotgun-prompt-context="shotgunPromptContext"
                    :generation-progress="generationProgressData"
                    :is-generating-context="isGeneratingContext"
                    :projects="projects" 
                    :platform="platform"
                    :user-task="userTask"
                    :rules-content="rulesContent"
                    :split-diffs="splitDiffs"
                    :is-loading-split-diffs="isLoadingSplitDiffs"
                    :final-prompt="finalPrompt"
                    :split-line-limit="splitLineLimitValue"
                    :shotgun-git-diff="shotgunGitDiff"
                    :split-line-limit-value="splitLineLimitValue"
                    @step-action="handleStepAction"
                    @update-composed-prompt="handleComposedPromptUpdate"
                    @update:user-task="handleUserTaskUpdate"
                    @update:rules-content="handleRulesContentUpdate"
                    @update:shotgunGitDiff="handleShotgunGitDiffUpdate"
                    @update:splitLineLimit="handleSplitLineLimitUpdate"
                    ref="centralPanelRef" />
    </div>
    <div 
      @mousedown="startResize"
      class="w-full h-2 bg-gray-300 hover:bg-gray-400 cursor-row-resize select-none"
      title="Resize console height"
    >
    </div>
    <BottomConsole :log-messages="logMessages" :height="consoleHeight" ref="bottomConsoleRef" />
  </div>
</template>

<script setup>
import { ref, reactive, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
import HorizontalStepper from './HorizontalStepper.vue';
import LeftSidebar from './LeftSidebar.vue';
import CentralPanel from './CentralPanel.vue';
import BottomConsole from './BottomConsole.vue';
import { ListFiles, RequestShotgunContextGeneration, SelectDirectory as SelectDirectoryGo, StartFileWatcher, StopFileWatcher, SetUseGitignore, SetUseCustomIgnore, SplitShotgunDiff } from '../../wailsjs/go/main/App';
import { EventsOn, Environment } from '../../wailsjs/runtime/runtime';

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

const projects = ref([]); // Array of {name, path, fileNodes}
const allFileNodes = ref([]); // Combined file nodes from all projects
const shotgunPromptContext = ref('');
const loadingError = ref('');
const useGitignore = ref(true);
const useCustomIgnore = ref(true);
const manuallyToggledNodes = reactive(new Map());
const isGeneratingContext = ref(false);
const generationProgressData = ref({ current: 0, total: 0 });
const isFileTreeLoading = ref(false);
const composedLlmPrompt = ref(''); // To store the prompt from Step 2
const platform = ref('unknown'); // To store OS platform (e.g., 'darwin', 'windows', 'linux')
const userTask = ref('');
const rulesContent = ref('');
const finalPrompt = ref('');
const isLoadingSplitDiffs = ref(false);
const splitDiffs = ref([]);
const shotgunGitDiff = ref('');
const splitLineLimitValue = ref(0); // Add new state variable
let debounceTimer = null;

// Watcher related
const projectFilesChangedPendingReload = ref(false);
let unlistenProjectFilesChanged = null;

async function addProjectHandler() {
  try {
    const selectedDir = await SelectDirectoryGo(); 
    if (selectedDir) {
      // Check if project already exists
      const existingProject = projects.value.find(p => p.path === selectedDir);
      if (existingProject) {
        addLog(`Project already added: ${selectedDir}`, 'warning', 'bottom');
        return;
      }

      // Add new project
      const projectName = selectedDir.split(/[\\/]/).pop() || selectedDir;
      const newProject = {
        name: projectName,
        path: selectedDir,
        fileNodes: []
      };

      projects.value.push(newProject);
      loadingError.value = '';

      await loadFileTreeForProject(newProject);
      
      // Clear any previous splits when projects change
      splitDiffs.value = [];

      if (projects.value.length === 1) {
        // First project added, reset steps
        steps.value.forEach(s => s.completed = false);
        currentStep.value = 1;
      }

      addLog(`Project added: ${selectedDir}`, 'info', 'bottom');
      
      // Trigger context generation for all projects
      debouncedTriggerShotgunContextGeneration();
    }
  } catch (err) {
    console.error("Error selecting directory:", err);
    const errorMsg = "Failed to select directory: " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
  }
}

function removeProjectHandler(projectIndex) {
  if (projectIndex >= 0 && projectIndex < projects.value.length) {
    const removedProject = projects.value.splice(projectIndex, 1)[0];
    addLog(`Project removed: ${removedProject.path}`, 'info', 'bottom');
    
    // Rebuild combined file nodes
    rebuildAllFileNodes();
    
    // Clear context if no projects left
    if (projects.value.length === 0) {
      shotgunPromptContext.value = '';
      isGeneratingContext.value = false;
      splitDiffs.value = [];
      steps.value.forEach(s => s.completed = false);
      currentStep.value = 1;
    } else {
      // Trigger context generation for remaining projects
      debouncedTriggerShotgunContextGeneration();
    }  }
}

function calculateNodeExcludedState(node) {
  const manualToggle = manuallyToggledNodes.get(node.relPath);
  if (manualToggle !== undefined) return manualToggle;
  if (useGitignore.value && node.isGitignored) return true;
  if (useCustomIgnore.value && node.isCustomIgnored) return true;
  return false;
}

function mapDataToTreeRecursive(nodes, parent, projectPath = null) {
  if (!nodes) return [];
  return nodes.map(node => {
    const isRootNode = parent === null;
    const reactiveNode = reactive({
      ...node,
      expanded: node.isDir ? isRootNode : undefined,
      parent: parent,
      children: [],
      projectPath: projectPath || (parent && parent.projectPath) || node.path
    });
    reactiveNode.excluded = calculateNodeExcludedState(reactiveNode);

    if (node.children && node.children.length > 0) {
      reactiveNode.children = mapDataToTreeRecursive(node.children, reactiveNode, projectPath);
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

function toggleExcludeNode(nodeToToggle) {
  // If the node is under an unselected parent and is currently unselected itself (nodeToToggle.excluded is true),
  // the first click should select it (set nodeToToggle.excluded to false).
  if (isAnyParentVisuallyExcluded(nodeToToggle) && nodeToToggle.excluded) {
    nodeToToggle.excluded = false;
  } else {
    // Otherwise, normal toggle behavior.
    nodeToToggle.excluded = !nodeToToggle.excluded;
  }
  manuallyToggledNodes.set(nodeToToggle.relPath, nodeToToggle.excluded);
  addLog(`Toggled exclusion for ${nodeToToggle.name} to ${nodeToToggle.excluded}`, 'info', 'bottom');
}

function updateAllNodesExcludedState(nodesToUpdate) { // This is the public-facing function
  // It calls the recursive helper, starting with parentIsVisuallyExcluded = false for root nodes.
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
  // Context regeneration is handled by the watch on [fileTree, useGitignore, useCustomIgnore]
  // which calls updateAllNodesExcludedState and debouncedTriggerShotgunContextGeneration.
}

function toggleCustomIgnoreHandler(value) {
  useCustomIgnore.value = value;
  addLog(`Custom ignore rules usage changed to: ${value}. Updating tree and watcher...`, 'info', 'bottom');
  SetUseCustomIgnore(value)
    .then(() => addLog(`Watchman instructed to use custom ignores: ${value}`, 'debug'))
    .catch(err => addLog(`Error setting useCustomIgnore in backend: ${err}`, 'error'));
}

function debouncedTriggerShotgunContextGeneration() {
  if (projects.value.length === 0) {
    // Clear context and stop loading if no projects
    shotgunPromptContext.value = ''; // Clear previous context
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress
    isGeneratingContext.value = false;
    return;
  }

  if (!isGeneratingContext.value) nextTick(() => isGeneratingContext.value = true);

  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    if (projects.value.length === 0) { 
        isGeneratingContext.value = false;
        return;
    }

    addLog("Debounced trigger: Requesting shotgun context generation for all projects...", 'info');
    
    // Update all nodes excluded state for all projects
    projects.value.forEach(project => {
      updateAllNodesExcludedState(project.fileNodes);
    });
    
    generationProgressData.value = { current: 0, total: 0 }; // Reset progress before new request

    // Get all project paths
    const projectPaths = projects.value.map(p => p.path);
    
    // Collect excluded paths from all projects
    const excludedPathsArray = [];
    
    // Helper to determine if a node has any visually included (checkbox checked) descendants
    function hasVisuallyIncludedDescendant(node) {
      if (!node.isDir || !node.children || node.children.length === 0) {
        return false;
      }
      for (const child of node.children) {
        if (!child.excluded) { // If child itself is visually included (checkbox is checked)
          return true;
        }
        if (hasVisuallyIncludedDescendant(child)) { // Or if any of its descendants are
          return true;
        }
      }
      return false;
    }

    function collectTrulyExcludedPaths(nodes) {
       if (!nodes) return;
       nodes.forEach(node => {
        // A node is TRULY excluded if its checkbox is unchecked (node.excluded is true)
        // AND it does not have any descendant that is checked (visually included).
        if (node.excluded && !hasVisuallyIncludedDescendant(node)) {
          excludedPathsArray.push(node.relPath);
          // If a node is truly excluded, its children are implicitly excluded from generation,
          // so no need to recurse further for collecting excluded paths under this node.
        } else {
          // If the node is visually included OR it's visually excluded but has an included descendant
          // (meaning this node's path needs to be in the tree structure for its descendant),
          // then we must check its children for their own exclusion status.
          if (node.children && node.children.length > 0) {
            collectTrulyExcludedPaths(node.children);
          }
        }
       });
     }
    
    // Collect excluded paths from all projects
    projects.value.forEach(project => {
      collectTrulyExcludedPaths(project.fileNodes);
    });
 
     RequestShotgunContextGeneration(projectPaths, excludedPathsArray)
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

  if (targetStep.completed || stepId === currentStep.value) {
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
  composedLlmPrompt.value = prompt;
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
  }

  const currentStepObj = steps.value.find(s => s.id === currentStep.value);

  switch (actionName) {
    case 'executePrompt':
      if (!composedLlmPrompt.value) {
        addLog("Cannot execute prompt: Prompt from Step 2 is empty.", 'warn', 'both');
        return;
      }
      addLog(`Simulating backend: Executing prompt (LLM call)... \nPrompt Preview (first 100 chars): "${composedLlmPrompt.value.substring(0,100)}..."`, 'info', 'step');
      // Here, you would actually send composedLlmPrompt.value to an LLM
      await new Promise(resolve => setTimeout(resolve, 1000));
      addLog('Backend: LLM call simulated. (Mocked response/diff would be processed here).', 'info', 'step');
      if (currentStepObj) currentStepObj.completed = true;
      // For now, just navigate to Step 4, as Step 3's "execution" is conceptual.
      // In a real app, Step 3 might display LLM output before proceeding.
      navigateToStep(4); 
      break;
    case 'executePromptAndSplitDiff': // Handle the actual splitting action
      if (!payload || !payload.gitDiff || payload.lineLimit <= 0) {
        addLog("Invalid payload for splitting diff.", 'error', 'bottom');
        return;
      }
      addLog(`Splitting diff (approx ${payload.lineLimit} lines per split)...`, 'info', 'bottom');
      isLoadingSplitDiffs.value = true;
      splitDiffs.value = []; // Clear previous splits
      shotgunGitDiff.value = payload.gitDiff;
      splitLineLimitValue.value = payload.lineLimit; // Store the line limit
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
    shotgunPromptContext.value = output;
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
  unlistenProjectFilesChanged = EventsOn("projectFilesChanged", (changedRootDir) => {
    // Find the project that matches the changed directory
    const affectedProject = projects.value.find(p => p.path === changedRootDir);
    if (!affectedProject) {
      addLog(`Watchman: Ignoring event for ${changedRootDir}, not in current projects`, 'debug');
      return;
    }
    addLog(`Watchman: Event "projectFilesChanged" received for ${changedRootDir}.`, 'debug');
    if (isFileTreeLoading.value || isGeneratingContext.value) {
      projectFilesChangedPendingReload.value = true;
      addLog("Watchman: File change detected, reload queued as system is busy.", 'info');
    } else {
      addLog("Watchman: File change detected, reloading tree immediately.", 'info');
      loadFileTreeForProject(affectedProject); // This will set isFileTreeLoading = true
      // debouncedTriggerShotgunContextGeneration will be called by the watcher on projects
    }
  });
});

onBeforeUnmount(async () => {
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  clearTimeout(debounceTimer);
  if (projects.value.length > 0) {
    // Stop file watchers for all projects
    for (const project of projects.value) {
      await StopFileWatcher(project.path).catch(err => console.error("Error stopping file watcher on unmount:", err));
      addLog(`File watcher stopped on component unmount for ${project.path}`, 'debug');
    }
  }
  if (unlistenProjectFilesChanged) {
    unlistenProjectFilesChanged();
  }
  // Remember to unlisten other events if they return unlistener functions
});

watch([projects, useGitignore, useCustomIgnore], ([newProjects, newUseGitignore, newUseCustomIgnore], [oldProjects, oldUseGitignore, oldUseCustomIgnore]) => {
  addLog("Watcher detected changes in projects, useGitignore, or useCustomIgnore. Re-evaluating context.", 'debug', 'bottom');
  
  // Update all nodes excluded state for all projects
  projects.value.forEach(project => {
    updateAllNodesExcludedState(project.fileNodes);
  });
  
  // Rebuild combined file nodes
  rebuildAllFileNodes();
  
  debouncedTriggerShotgunContextGeneration();
}, { deep: true });

watch(projects, async (newProjects, oldProjects) => {
  // Stop watchers for removed projects
  if (oldProjects) {
    const oldPaths = oldProjects.map(p => p.path);
    const newPaths = newProjects.map(p => p.path);
    const removedPaths = oldPaths.filter(path => !newPaths.includes(path));
    
    for (const removedPath of removedPaths) {
      await StopFileWatcher(removedPath).catch(err => addLog(`Error stopping watcher for ${removedPath}: ${err}`, 'error'));
      addLog(`File watcher stopped for ${removedPath}`, 'debug');
    }
  }
  
  // Start watchers for new projects
  if (newProjects) {
    const oldPaths = oldProjects ? oldProjects.map(p => p.path) : [];
    const newPaths = newProjects.map(p => p.path);
    const addedPaths = newPaths.filter(path => !oldPaths.includes(path));
    
    for (const addedPath of addedPaths) {
      await StartFileWatcher(addedPath).catch(err => addLog(`Error starting watcher for ${addedPath}: ${err}`, 'error'));
      addLog(`File watcher started for ${addedPath}`, 'debug');
    }
  }
  
  if (newProjects.length === 0) {
    // No projects, clear everything
    allFileNodes.value = [];
    shotgunPromptContext.value = '';
    loadingError.value = '';
    manuallyToggledNodes.clear();
    isGeneratingContext.value = false;
    projectFilesChangedPendingReload.value = false;
  }
}, { deep: true });// 'immediate: false' to avoid running on initial undefined -> '' or '' -> initial value if set by default

// Helper function to process pending reloads
function checkAndProcessPendingFileTreeReload() {
  if (projectFilesChangedPendingReload.value && !isGeneratingContext.value && projects.value.length > 0) {
    projectFilesChangedPendingReload.value = false;
    addLog("Watchman: Processing queued file tree reload for all projects.", 'info');
    
    // Reload all projects
    Promise.all(projects.value.map(project => loadFileTreeForProject(project)))
      .then(() => {
        addLog("All project file trees reloaded successfully.", 'info');
      })
      .catch(err => {
        addLog(`Error reloading project file trees: ${err.message || err}`, 'error');
      });
  }
}

function handleCustomRulesUpdated() {
  addLog("Custom ignore rules updated by user. Reloading file tree.", 'info');
  if (projects.value.length > 0) {
    // This will call ListFiles in Go, which will use the new custom rules from app.settings.
    // The new tree will have updated IsCustomIgnored flags.
    // The watch on projects (and its subsequent call to debouncedTriggerShotgunContextGeneration)
    // will then handle regenerating the context.
    projects.value.forEach(project => {
      loadFileTreeForProject(project);
    });
  }
}

function handleUserTaskUpdate(val) {
  userTask.value = val;
}

function handleRulesContentUpdate(val) {
  rulesContent.value = val;
}

// Add handlers for the new updates
function handleShotgunGitDiffUpdate(val) {
  shotgunGitDiff.value = val;
}

function handleSplitLineLimitUpdate(val) {
  splitLineLimitValue.value = val;
}

async function loadFileTreeForProject(project) {
  addLog(`Loading file tree for project: ${project.path}`, 'info', 'bottom');
  try {
    const treeData = await ListFiles(project.path);
    project.fileNodes = mapDataToTreeRecursive(treeData, null, project.path);
    addLog(`File tree loaded for ${project.name}. Root items: ${project.fileNodes.length}`, 'info', 'bottom');
    
    // Rebuild combined file nodes
    rebuildAllFileNodes();
  } catch (err) {
    console.error("Error listing files for project:", err);
    const errorMsg = "Failed to load file tree for " + project.name + ": " + (err.message || err);
    loadingError.value = errorMsg;
    addLog(errorMsg, 'error', 'bottom');
    project.fileNodes = [];
  }
}

function rebuildAllFileNodes() {
  // Combine all project file nodes into a single tree structure
  allFileNodes.value = [];
  
  projects.value.forEach(project => {
    if (project.fileNodes && project.fileNodes.length > 0) {
      // Create a project root node
      const projectRootNode = reactive({
        name: project.name,
        path: project.path,
        relPath: project.path,
        isDir: true,
        expanded: true,
        excluded: false,
        children: project.fileNodes,
        isProjectRoot: true,
        projectPath: project.path
      });
      
      allFileNodes.value.push(projectRootNode);
    }
  });
}
</script>

<style scoped>
.flex-1 {
  min-height: 0;
}
</style>