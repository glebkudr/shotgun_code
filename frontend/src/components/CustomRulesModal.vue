<template>
  <div v-if="isVisible" class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50 flex justify-center items-center" @click.self="handleCancel">
    <div class="relative mx-auto p-5 border w-full max-w-2xl shadow-lg rounded-md bg-white">
      <div class="mt-3 text-center">
        <h3 class="text-lg leading-6 font-medium text-gray-900">{{ title }}</h3>
        <div class="mt-2 px-7 py-3">
          <textarea 
            v-model="editableRules"
            rows="15"
            class="w-full p-2 border border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 text-sm font-mono bg-gray-50"
            placeholder="Enter custom ignore patterns, one per line (e.g., *.log, node_modules/)"
          ></textarea>
          <p class="text-xs text-gray-500 mt-1 text-left">{{ descriptionText }}</p>
        </div>
        <div class="items-center px-4 py-3">
          <button
            @click="handleSave"
            class="px-4 py-2 bg-blue-500 text-white text-base font-medium rounded-md w-auto hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 mr-2"
          >
            Save
          </button>
          <button
            @click="handleCancel"
            class="px-4 py-2 bg-gray-200 text-gray-800 text-base font-medium rounded-md w-auto hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-400"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, defineProps, defineEmits, computed } from 'vue';

const props = defineProps({
  isVisible: {
    type: Boolean,
    required: true,
  },
  initialRules: {
    type: String,
    default: '',
  },
  title: {
    type: String,
    default: 'Edit Custom Rules'
  },
  ruleType: {
    type: String,
    required: true,
    validator: (value) => ['ignore', 'prompt'].includes(value)
  }
});

const emit = defineEmits(['save', 'cancel']);

const editableRules = ref('');

const descriptionText = computed(() => {
  if (props.ruleType === 'prompt') {
    return 'These rules provide specific instructions or pre-defined text for the AI. They will be included in the final prompt.';
  }
  // Default to the description for ignore rules
  return 'These rules use .gitignore pattern syntax. They are applied globally when "Use custom rules" is checked.';
});

watch(() => props.initialRules, (newVal) => {
  editableRules.value = newVal;
}, { immediate: true });

watch(() => props.isVisible, (newVal) => {
  if (newVal) {
    // When modal becomes visible, ensure textarea reflects the latest initialRules
    editableRules.value = props.initialRules;
  }
});

function handleSave() {
  emit('save', editableRules.value);
}

function handleCancel() {
  emit('cancel');
}
</script>

<style scoped>
/* Basic styling for modal, can be enhanced with Tailwind further if needed */
</style> 