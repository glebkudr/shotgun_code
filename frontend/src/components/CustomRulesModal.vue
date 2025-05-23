<template>
  <div v-if="isVisible" class="modal-overlay" @click.self="handleCancel">
    <div class="modal-container">
      <div class="modal-content">
        <h3 class="modal-title">{{ title }}</h3>
        <div class="modal-body">
          <textarea 
            v-model="editableRules"
            rows="15"
            class="modal-textarea"
            :placeholder="ruleType === 'prompt' ? 'Enter custom prompt rules...' : 'Enter custom ignore patterns, one per line (e.g., *.log, node_modules/)'"
          ></textarea>
          <p class="modal-hint">{{ descriptionText }}</p>
        </div>
        <div class="modal-actions">
          <button
            @click="handleSave"
            class="btn-primary"
          >
            Save
          </button>
          <button
            @click="handleCancel"
            class="btn-secondary"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  overflow-y: auto;
  height: 100%;
  width: 100%;
  z-index: 50;
  display: flex;
  justify-content: center;
  align-items: center;
  backdrop-filter: blur(2px);
  transition: opacity 0.2s ease-out;
}

.modal-container {
  position: relative;
  margin: 1rem;
  width: 100%;
  max-width: 42rem;
  background-color: var(--surface-1);
  border-radius: 0.5rem;
  box-shadow: var(--shadow-elevation);
  overflow: hidden;
  transition: transform 0.2s ease-out, opacity 0.2s ease-out;
  border: 1px solid var(--border-primary);
}

.modal-content {
  padding: 1.5rem;
  text-align: center;
}

.modal-title {
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 1rem 0;
}

.modal-body {
  margin: 1rem 0;
  padding: 0 0.5rem;
}

.modal-textarea {
  width: 100%;
  min-height: 15rem;
  padding: 0.75rem;
  border: 1px solid var(--input-border);
  border-radius: 0.375rem;
  background-color: var(--input-bg);
  color: var(--text-primary);
  font-family: inherit;
  font-size: 0.9375rem;
  line-height: 1.5;
  resize: vertical;
  transition: border-color 0.2s ease-out, box-shadow 0.2s ease-out;
}

.modal-textarea:focus {
  outline: none;
  border-color: var(--input-focus);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.modal-hint {
  margin-top: 0.5rem;
  font-size: 0.8125rem;
  color: var(--text-hint);
  text-align: left;
  line-height: 1.4;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border-secondary);
  margin-top: 1.5rem;
}

/* Responsive adjustments */
@media (max-width: 640px) {
  .modal-container {
    margin: 0.5rem;
  }
  
  .modal-content {
    padding: 1rem;
  }
  
  .modal-actions {
    flex-direction: column;
    gap: 0.5rem;
  }
}
</style>

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
