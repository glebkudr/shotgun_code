<template>
  <aside class="sidebar-container left-sidebar flex flex-col h-full relative" :style="{ width: width + 'px' }">
    <div class="resize-handle left" @mousedown="startResize"></div>
    <!-- Step Navigation -->
    <div class="flex-grow">
      <h2 class="text-subtitle mb-4">Steps</h2>
      <div class="space-y-2">
        <button 
          v-for="step in steps" :key="step.id"
          @click="canNavigateToStep(step.id) ? $emit('navigate', step.id) : null"
          :disabled="!canNavigateToStep(step.id)"
          :class="[
            'step-button',
            currentStep === step.id ? 'step-button-current' : '',
            step.completed ? 'step-button-completed' : '',
            !canNavigateToStep(step.id) ? 'step-button-disabled' : ''
          ]"
        >
          <div class="flex items-center">
            <span class="mr-2">{{ step.id }}.</span>
            <span>{{ step.title }}</span>
          </div>
        </button>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { defineProps, defineEmits, ref, onMounted, onUnmounted } from 'vue';

const props = defineProps({
  width: {
    type: Number,
    default: 250
  },
  currentStep: { type: Number, required: true },
  steps: { type: Array, required: true },
});

const emit = defineEmits(['navigate', 'resize']);

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
  
  const dx = event.clientX - startX.value;
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

function canNavigateToStep(stepId) {
  // Always allow navigation to the current step
  if (stepId === props.currentStep) return true;
  
  // Always allow navigation to steps 2, 3, and 4 and completed steps
  const step = props.steps.find(s => s.id === stepId);
  return (stepId === 2 || stepId === 3 || stepId === 4) || (step && step.completed);
}
</script>