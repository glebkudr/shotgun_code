<template>
  <main class="flex-1 p-0 overflow-y-auto bg-white dark:bg-slate-900 relative">
    <Step1CopyStructure
      v-if="currentStep === 1"
      @action="handleAction"
      @auto-context="emit('auto-context')"
      @open-llm-settings="emit('open-llm-settings')"
      :generated-context="shotgunPromptContext"
      :is-loading-context="props.isGeneratingContext"
      :project-root="props.projectRoot"
      :generation-progress="props.generationProgress"
      :platform="props.platform"
      :has-active-llm-key="props.hasActiveLlmKey"
      :is-auto-context-loading="props.isAutoContextLoading"
      :user-task="props.userTask"
      @update:user-task="(val) => emit('update:userTask', val)"
    />
    <Step2ComposePrompt 
      v-if="currentStep === 2" 
      @action="handleAction" 
      ref="step2Ref" 
      :file-list-context="props.shotgunPromptContext" 
      @update:finalPrompt="(val) => emit('update-composed-prompt', val)" 
      :platform="props.platform" 
      :user-task="props.userTask" 
      :rules-content="props.rulesContent" 
      :final-prompt="props.finalPrompt" 
      :has-active-llm-key="props.hasActiveLlmKey"
      @update:userTask="(val) => emit('update:userTask', val)" 
      @update:rulesContent="(val) => emit('update:rulesContent', val)" 
      @open-llm-settings="emit('open-llm-settings')"
    />
    <Step3ExecutePrompt v-if="currentStep === 3" ref="step3Ref" />
  </main>
</template>

<script setup>
import { defineProps, defineEmits, ref } from 'vue';
import Step1CopyStructure from './steps/Step1PrepareContext.vue';
import Step2ComposePrompt from './steps/Step2ComposePrompt.vue';
import Step3ExecutePrompt from './steps/Step3ExecutePrompt.vue';

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
  hasActiveLlmKey: { type: Boolean, default: false },
  isAutoContextLoading: { type: Boolean, default: false }
});

const emit = defineEmits(['stepAction', 'update-composed-prompt', 'update:userTask', 'update:rulesContent', 'auto-context', 'open-llm-settings']);

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
