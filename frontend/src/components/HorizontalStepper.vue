<template>
  <nav class="bg-white shadow-md sticky top-0 z-10">
    <div class="container mx-auto px-4 py-3">
      <ol class="flex items-center justify-between">
        <li v-for="(step, index) in steps" :key="step.id" 
            class="flex-1 group"
            :class="{'flex items-center': index < steps.length - 1}">
          <div class="flex items-center w-full">
            <button
              @click.prevent="canNavigateToStep(step.id) ? $emit('navigate', step.id) : null"
              :class="[
                'flex flex-col items-center text-xs sm:text-sm font-medium text-center p-2 rounded-md w-full',
                canNavigateToStep(step.id) ? 'cursor-pointer hover:bg-gray-100' : 'cursor-not-allowed opacity-60'
              ]"
              :disabled="!canNavigateToStep(step.id)"
              :title="step.description"
            >
              <div
                :class="[
                  'flex items-center justify-center w-8 h-8 sm:w-10 sm:h-10 rounded-full border-2 mb-1',
                  currentStep === step.id ? 'border-blue-600 bg-blue-100 text-blue-700' : 
                  (step.completed ? 'border-green-600 bg-green-100 text-green-700' : 'border-gray-400 bg-gray-50 text-gray-500 group-hover:border-gray-500')
                ]"
              >
                <span v-if="!(step.completed && currentStep !== step.id)">{{ step.id }}</span>
                <svg v-else class="w-4 h-4 sm:w-5 sm:h-5" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>
              </div>
              <span :class="['whitespace-nowrap', currentStep === step.id ? 'text-blue-600 font-semibold' : (step.completed ? 'text-green-600' : 'text-gray-500 group-hover:text-gray-700')]">
                {{ step.title }}
              </span>
            </button>
          </div>
          <!-- Connector line -->
          <div v-if="index < steps.length - 1" class="flex-auto border-t-2 transition-all duration-300 ease-in-out"
               :class="step.completed ? 'border-green-500' : 'border-gray-300'">
          </div>
        </li>
      </ol>
    </div>
  </nav>
</template>
<script setup>
import { defineProps, defineEmits } from 'vue';

const props = defineProps({
  currentStep: { type: Number, required: true },
  steps: { type: Array, required: true }, // Array of { id: Number, title: String, completed: Boolean }
});

const emit = defineEmits(['navigate']);

function canNavigateToStep(stepId) {
  if (stepId === props.currentStep) return true;
  const targetStep = props.steps.find(s => s.id === stepId);
  if (targetStep && targetStep.completed) return true;
  let firstUncompletedStepId = props.steps.find(s => !s.completed)?.id;
  return stepId === firstUncompletedStepId || (firstUncompletedStepId === undefined && targetStep); // Allow any if all completed
}
</script> 