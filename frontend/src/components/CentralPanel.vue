<template>
  <main class="flex-1 p-0 overflow-y-auto bg-white relative">
    <Step1CopyStructure v-if="currentStep === 1" @action="handleAction" :generated-context="shotgunPromptContext" :is-loading-context="props.isGeneratingContext" :project-root="props.projectRoot" :generation-progress="props.generationProgress" :platform="props.platform" />
    <Step2ComposePrompt v-if="currentStep === 2" @action="handleAction" ref="step2Ref" :file-list-context="props.shotgunPromptContext" @update:finalPrompt="(val) => emit('update-composed-prompt', val)" :platform="props.platform" :user-task="props.userTask" :rules-content="props.rulesContent" :final-prompt="props.finalPrompt" @update:userTask="(val) => emit('update:userTask', val)" @update:rulesContent="(val) => emit('update:rulesContent', val)" />
    <Step3ExecutePrompt v-if="currentStep === 3" @action="handleAction" ref="step3Ref" :initial-git-diff="initialGitDiff" :initial-split-line-limit="initialSplitLineLimit" @update:shotgunGitDiff="(val) => emit('update:shotgunGitDiff', val)" @update:splitLineLimit="(val) => emit('update:splitLineLimit', val)" />
    <Step4ApplyPatch v-if="currentStep === 4" @action="handleAction" :split-diffs="props.splitDiffs" :is-loading="props.isLoadingSplitDiffs" :platform="props.platform" :split-line-limit="initialSplitLineLimit" />
  </main>
</template>

<script setup>
import { defineProps, defineEmits, ref, computed, watch } from 'vue';
import Step1CopyStructure from './steps/Step1PrepareContext.vue';
import Step2ComposePrompt from './steps/Step2ComposePrompt.vue';
import Step3ExecutePrompt from './steps/Step3ExecutePrompt.vue';
import Step4ApplyPatch from './steps/Step4ApplyPatch.vue';
import OpenRouterPanel from './OpenRouterPanel.vue'; // Import the new panel

const props = defineProps({
  currentStep: { type: Number, required: true },
  shotgunPromptContext: { type: String, default: '' },
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
  splitLineLimitValue: { type: Number, default: 0 }
});

const initialGitDiff = computed(() => {
  const value = props.shotgunGitDiff || '';
    return value;
});

const initialSplitLineLimit = computed(() => {
  const value = props.splitLineLimitValue || 0;
    return value;
});

const emit = defineEmits(['stepAction', 'update-composed-prompt', 'update:userTask', 'update:rulesContent', 'update:shotgunGitDiff', 'update:splitLineLimit']);

const step2Ref = ref(null);
const step3Ref = ref(null);

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

defineExpose({ updateStep2DiffOutput, addLogToStep3Console, updateStep2ShotgunContext });
</script> 