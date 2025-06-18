<template>
  <aside class="sidebar-container left-sidebar" v-show="width > 0" :style="{ width: width + 'px' }">
    <div class="resize-handle left" @mousedown="startResize"></div>
    
    <div class="section-wrapper overflow-auto" style="height: 100%;">
      <div class="p-2">
        <h2 class="text-subtitle mb-2 text-center text-xs">Steps</h2>
        <div class="space-y-3">
        <button 
          v-for="step in steps" :key="step.id"
          @click="canNavigateToStep(step.id) ? $emit('navigate', step.id) : null"
          :disabled="!canNavigateToStep(step.id)"
          :class="[
            isCompact ? 'step-button-icon' : 'step-button',
            currentStep === step.id ? 'step-button-current' : '',
            step.completed ? 'step-button-completed' : '',
            !canNavigateToStep(step.id) ? 'step-button-disabled' : ''
          ]"
          :title="step.title"
        >
          <div :class="isCompact ? 'flex items-center justify-center' : 'flex flex-col items-center'">
            <span class="step-number">{{ step.id }}{{ isCompact ? '' : '.' }}</span>
            <span v-if="!isCompact" class="step-title">{{ step.title }}</span>
          </div>
        </button>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { defineProps, defineEmits, ref, computed, onMounted, onUnmounted } from 'vue';

const props = defineProps({
  width: {
    type: Number,
    default: 250
  },
  currentStep: { type: Number, required: true },
  steps: { type: Array, required: true },
});

const emit = defineEmits(['navigate', 'resize']);

const isCompact = computed(() => props.width < 140);
const isResizing = ref(false);
const startX = ref(0);
const startWidth = ref(0);

function startResize(event) {
  isResizing.value = true;
  startX.value = event.clientX;
  startWidth.value = props.width;
  
  document.documentElement.classList.add('resize-transition-paused');
  
  document.addEventListener('mousemove', doResize);
  document.addEventListener('mouseup', stopResize);
  event.preventDefault();
}

let animationFrameId = null;

function doResize(event) {
  if (!isResizing.value) return;
  
  if (animationFrameId) {
    cancelAnimationFrame(animationFrameId);
  }
  animationFrameId = requestAnimationFrame(() => {
    const dx = event.clientX - startX.value;
    const newWidth = Math.round(startWidth.value + dx);
    
    const minWidth = 80;
    const maxWidth = 220;
    
    // Apply constraints
    const constrainedWidth = Math.max(minWidth, Math.min(maxWidth, newWidth));
    
    if (constrainedWidth !== props.width) {
      emit('resize', constrainedWidth);
    }

    const wasCompact = startWidth.value < 140;
    const isNowCompact = constrainedWidth < 140;
    
    if (wasCompact !== isNowCompact) {
      document.documentElement.classList.add('transition-delayed');
      setTimeout(() => {
        document.documentElement.classList.remove('transition-delayed');
      }, 100);
    }
  });
}

function stopResize() {
  isResizing.value = false;

  if (animationFrameId) {
    cancelAnimationFrame(animationFrameId);
    animationFrameId = null;
  }

  document.documentElement.classList.remove('resize-transition-paused');
  
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  
  const finalWidth = props.width;
  if (finalWidth < 85 && finalWidth > 0) {
    emit('resize', 85);
  } else if (finalWidth > 200) {
    emit('resize', 200);
  }
}

function canNavigateToStep(stepId) {
  if (stepId === props.currentStep) return true;
  const step = props.steps.find(s => s.id === stepId);
  return (stepId === 2 || stepId === 3 || stepId === 4) || (step && step.completed);
}
</script>