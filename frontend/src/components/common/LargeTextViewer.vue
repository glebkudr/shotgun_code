<template>
  <div class="flex flex-col w-full">
    <div v-if="showHeader && (label || totalSizeLabel)" class="flex items-center justify-between mb-1">
      <label v-if="label" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{{ label }}</label>
      <span class="text-xs text-gray-500 dark:text-gray-400" v-if="totalSizeLabel">{{ totalSizeLabel }}</span>
    </div>

    <div
      class="w-full p-2 border border-gray-300 rounded-md shadow-sm bg-gray-50 dark:bg-slate-900 font-mono text-xs text-gray-800 dark:text-gray-100 whitespace-pre-wrap break-words overflow-auto"
      :style="{ minHeight, maxHeight }"
    >
      <template v-if="hasContent">
        {{ displayContent }}
      </template>
      <template v-else>
        <span class="text-gray-400">{{ placeholder }}</span>
      </template>
    </div>

    <div v-if="showFooter && hasContent" class="flex items-center justify-between mt-2">
      <p class="text-xs" :class="isTruncated ? 'text-amber-600 dark:text-amber-500' : 'text-gray-500 dark:text-gray-400'">
        <template v-if="isTruncated">
          Showing preview of {{ previewSizeLabel }} ({{ displayedCharactersLabel }}) out of {{ totalSizeLabel }} ({{ totalCharactersLabel }})
        </template>
        <template v-else>
          Total size: {{ totalSizeLabel }} ({{ totalCharactersLabel }})
        </template>
      </p>
      <button
        v-if="showCopyButton"
        type="button"
        @click="copyFullContent"
        class="px-3 py-1 bg-gray-200 text-gray-700 text-xs font-semibold rounded-md hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-opacity-50 dark:bg-slate-700 dark:text-gray-100 dark:hover:bg-slate-600"
      >
        {{ copyButtonText }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';

const props = defineProps({
  content: {
    type: String,
    default: ''
  },
  label: {
    type: String,
    default: ''
  },
  maxDisplayLength: {
    type: Number,
    default: 6000
  },
  placeholder: {
    type: String,
    default: 'Content preview will appear here.'
  },
  platform: {
    type: String,
    default: 'unknown'
  },
  minHeight: {
    type: String,
    default: '150px'
  },
  maxHeight: {
    type: String,
    default: '1000px'
  },
  showCopyButton: {
    type: Boolean,
    default: true
  },
  copyButtonLabel: {
    type: String,
    default: 'Copy Full Content'
  },
  showHeader: {
    type: Boolean,
    default: true
  },
  showFooter: {
    type: Boolean,
    default: true
  }
});

const emit = defineEmits(['copied']);

const copyButtonText = ref(props.copyButtonLabel);

watch(() => props.copyButtonLabel, (newValue) => {
  copyButtonText.value = newValue;
});

const totalCharacters = computed(() => (props.content || '').length);
const displayedCharacters = computed(() => Math.min(totalCharacters.value, props.maxDisplayLength));
const hasContent = computed(() => totalCharacters.value > 0);
const isTruncated = computed(() => totalCharacters.value > props.maxDisplayLength);
const displayContent = computed(() => (props.content || '').slice(0, props.maxDisplayLength));

const totalSizeLabel = computed(() => formatBytes(totalCharacters.value));
const previewSizeLabel = computed(() => formatBytes(displayedCharacters.value));
const displayedCharactersLabel = computed(() => `${displayedCharacters.value.toLocaleString()} chars`);
const totalCharactersLabel = computed(() => `${totalCharacters.value.toLocaleString()} chars`);

function formatBytes(length) {
  if (!length) {
    return '0 B';
  }
  if (length >= 1024 * 1024) {
    return `${(length / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (length >= 1024) {
    return `${(length / 1024).toFixed(1)} KB`;
  }
  return `${length} B`;
}

async function copyFullContent() {
  if (!props.content) return;

  try {
    if (props.platform === 'darwin') {
      await WailsClipboardSetText(props.content);
    } else {
      await navigator.clipboard.writeText(props.content);
    }
    copyButtonText.value = 'Copied!';
    emit('copied');
    resetCopyLabelLater();
    return;
  } catch (err) {
    console.error('Failed to copy content preview:', err);
  }

  // Fallback to whichever option we have not tried yet
  try {
    if (props.platform === 'darwin') {
      await navigator.clipboard.writeText(props.content);
    } else {
      await WailsClipboardSetText(props.content);
    }
    copyButtonText.value = 'Copied!';
    emit('copied');
  } catch (err) {
    console.error('Fallback clipboard copy also failed:', err);
    copyButtonText.value = 'Failed!';
  } finally {
    resetCopyLabelLater();
  }
}

function resetCopyLabelLater() {
  setTimeout(() => {
    copyButtonText.value = props.copyButtonLabel;
  }, 2000);
}
</script>
