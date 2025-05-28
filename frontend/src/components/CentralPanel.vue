<template>
  <main class="central-panel">
    <Step1CopyStructure 
      v-if="currentStep === 1" 
      @action="handleAction" 
      @step-completed="handleStepCompleted"
      ref="step1Ref" 
      :generated-context="shotgunPromptContext" 
      :is-loading-context="props.isGeneratingContext" 
      :project-root="props.projectRoot" 
      :generation-progress="props.generationProgress" 
      :platform="props.platform" 
    />
    
    <Step2ComposePrompt 
      v-if="currentStep === 2" 
      @action="handleAction" 
      @step-completed="handleStepCompleted"
      ref="step2Ref" 
      :file-list-context="props.shotgunPromptContext" 
      :file-structure-context="props.shotgunPromptContext" 
      @update:finalPrompt="(val) => emit('update-composed-prompt', val)" 
      :platform="props.platform" 
      :user-task="props.userTask" 
      :rules-content="props.rulesContent" 
      :final-prompt="props.finalPrompt" 
      :selected-template="props.selectedTemplate"
      @update:userTask="(val) => emit('update:userTask', val)" 
      @update:rulesContent="(val) => emit('update:rulesContent', val)" 
    />
    
    <Step3ExecutePrompt 
      v-if="currentStep === 3" 
      @action="handleAction" 
      @step-completed="handleStepCompleted"
      ref="step3Ref" 
      :initial-git-diff="initialGitDiff" 
      :initial-split-line-limit="initialSplitLineLimit" 
      @update:shotgunGitDiff="(val) => emit('update:shotgunGitDiff', val)" 
      @update:splitLineLimit="(val) => emit('update:splitLineLimit', val)" 
    />
    
    <Step4ApplyPatch 
      v-if="currentStep === 4" 
      @action="handleAction" 
      @step-completed="handleStepCompleted"
      ref="step4Ref" 
      :split-diffs="props.splitDiffs" 
      :is-loading="props.isLoadingSplitDiffs" 
      :platform="props.platform" 
      :split-line-limit="initialSplitLineLimit" 
    />
  </main>
</template>

<script setup>
import { defineProps, defineEmits, ref, computed, watch, onMounted, nextTick } from 'vue';
import Step1CopyStructure from './steps/Step1PrepareContext.vue';
import Step2ComposePrompt from './steps/Step2ComposePrompt.vue';
import Step3ExecutePrompt from './steps/Step3ExecutePrompt.vue';
import Step4ApplyPatch from './steps/Step4ApplyPatch.vue';

const props = defineProps({
  currentStep: { type: Number, required: true },
  shotgunPromptContext: { type: String, default: '' },
  fileStructureContext: { type: String, default: '' },
  isGeneratingContext: { type: Boolean, default: false },
  projectRoot: { type: String, default: '' },
  generationProgress: { type: Object, default: () => ({ current: 0, total: 0 }) },
  platform: { type: String, default: 'unknown' },
  userTask: { type: String, default: '' },
  rulesContent: { type: String, default: '' },
  finalPrompt: { type: String, default: '' },
  splitDiffs: { type: Array, default: () => [] },
  isLoadingSplitDiffs: { type: Boolean, default: false },
  shotgunGitDiff: { type: String, default: '' },
  splitLineLimitValue: { type: Number, default: 0 },
  selectedTemplate: { type: String, default: 'dev' }
});

const emit = defineEmits([
  'stepAction', 
  'step-completed',
  'update-composed-prompt', 
  'update:userTask', 
  'update:rulesContent', 
  'update:shotgunGitDiff', 
  'update:splitLineLimit'
]);

const step1Ref = ref(null);
const step2Ref = ref(null);
const step3Ref = ref(null);
const step4Ref = ref(null);

const initialGitDiff = computed(() => {
  return props.shotgunGitDiff || '';
});

const initialSplitLineLimit = computed(() => {
  return props.splitLineLimitValue || 500;
});

function handleStepCompleted(stepId, data = null) {  
  const isValid = validateStepData(stepId, data);
  
  if (isValid) {
    emit('step-completed', stepId, data);
  } else {
    console.warn(`Step ${stepId} completion validation failed`, data);
  }
}

function validateStepData(stepId, data) {
  switch (stepId) {
    case 1:
      return props.shotgunPromptContext && 
             props.shotgunPromptContext.trim() !== '' && 
             !props.shotgunPromptContext.startsWith('Error:');
    
    case 2:
      return props.userTask && 
             props.userTask.trim() !== '' && 
             props.finalPrompt && 
             props.finalPrompt.trim() !== '' &&
             !props.finalPrompt.includes("No task provided by the user.");
    
    case 3:
      return props.shotgunGitDiff && props.shotgunGitDiff.trim() !== '';
    
    case 4:
      return data && (data.atLeastOneCopied === true || data.completed === true);
    
    default:
      return false;
  }
}

watch(() => props.shotgunPromptContext, (newVal) => {
  if (props.currentStep === 1 && newVal && newVal.trim() !== '' && !newVal.startsWith('Error:')) {
    nextTick(() => {
      if (step1Ref.value && typeof step1Ref.value.checkCompletion === 'function') {
        const isComplete = step1Ref.value.checkCompletion();
        if (isComplete) {
          handleStepCompleted(1);
        }
      } else {
        handleStepCompleted(1);
      }
    });
  }
});

watch([() => props.userTask, () => props.finalPrompt], ([newUserTask, newFinalPrompt]) => {
  if (props.currentStep === 2 && newUserTask && newFinalPrompt) {
    nextTick(() => {
      if (step2Ref.value && typeof step2Ref.value.checkCompletion === 'function') {
        const isComplete = step2Ref.value.checkCompletion();
        if (isComplete) {
          handleStepCompleted(2);
        }
      } else {
        if (validateStepData(2)) {
          handleStepCompleted(2);
        }
      }
    });
  }
});

watch(() => props.shotgunGitDiff, (newVal) => {
  if (props.currentStep === 3 && newVal && newVal.trim() !== '') {
    nextTick(() => {
      if (step3Ref.value && typeof step3Ref.value.checkCompletion === 'function') {
        const isComplete = step3Ref.value.checkCompletion();
        if (isComplete) {
          handleStepCompleted(3);
        }
      } else {
        handleStepCompleted(3);
      }
    });
  }
});

watch(() => props.currentStep, async (newStep, oldStep) => {
  await nextTick();
  
  switch (newStep) {
    case 1:
      if (props.shotgunPromptContext && !props.shotgunPromptContext.startsWith('Error:')) {
        setTimeout(() => {
          if (validateStepData(1)) {
            handleStepCompleted(1);
          }
        }, 100);
      }
      break;
      
    case 2:
      if (step2Ref.value) {
        if (typeof step2Ref.value.forceUpdatePrompt === 'function') {
          setTimeout(() => {
            if (step2Ref.value && typeof step2Ref.value.forceUpdatePrompt === 'function') {
              step2Ref.value.forceUpdatePrompt();
              
              setTimeout(() => {
                if (validateStepData(2)) {
                  handleStepCompleted(2);
                }
              }, 500);
            } 
          }, 100);
        } else {
          console.warn('forceUpdatePrompt function not available');
          if (validateStepData(2)) {
            handleStepCompleted(2);
          }
        }
      } else {
        console.warn('step2Ref is not available');
        if (validateStepData(2)) {
          handleStepCompleted(2);
        }
      }
      break;
      
    case 3:
      if (props.shotgunGitDiff && props.shotgunGitDiff.trim() !== '') {
        setTimeout(() => {
          if (validateStepData(3)) {
            handleStepCompleted(3);
          }
        }, 100);
      }
      break;
      
    case 4:
      break;
  }
});
function handleAction(actionName, payload) {
  emit('stepAction', actionName, payload);
}

const updateStep2DiffOutput = (output) => {
  if (step2Ref.value && step2Ref.value.setDiffOutput) {
    step2Ref.value.setDiffOutput(output);
  }
};

const updateStep2ShotgunContext = (context) => {
  if (step2Ref.value && step2Ref.value.setShotgunContext) {
    step2Ref.value.setShotgunContext(context);
  }
};

const addLogToStep3Console = (message, type) => {
  if (step3Ref.value && step3Ref.value.addLog) {
    step3Ref.value.addLog(message, type);
  }
};

const forceCompletionCheck = () => {
  const currentStepRef = getCurrentStepRef();
  if (currentStepRef && typeof currentStepRef.checkCompletion === 'function') {
    const isComplete = currentStepRef.checkCompletion();
    if (isComplete) {
      handleStepCompleted(props.currentStep);
    }
    return isComplete;
  }
  return false;
};

const getCurrentStepRef = () => {
  switch (props.currentStep) {
    case 1: return step1Ref.value;
    case 2: return step2Ref.value;
    case 3: return step3Ref.value;
    case 4: return step4Ref.value;
    default: return null;
  }
};

defineExpose({ 
  updateStep2DiffOutput, 
  addLogToStep3Console, 
  updateStep2ShotgunContext,
  forceCompletionCheck,
  step1Ref,
  step2Ref,
  step3Ref,
  step4Ref
});
</script>