<template>
  <div class="flex flex-col w-full">
    <div v-if="showHeader && (label || tokensLabel)" class="flex items-center justify-between mb-1">
      <label v-if="label" class="block text-sm font-medium text-gray-700">{{ label }}</label>
      <span :class="['text-xs font-medium', tokenCountColorClass]" v-if="tokensLabel">{{ tokensLabel }}</span>
    </div>

    <div
      class="w-full p-2 border border-gray-300 rounded-md shadow-sm bg-gray-50 font-mono text-xs text-gray-800 whitespace-pre-wrap break-words overflow-auto"
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
      <p class="text-xs" :class="isTruncated ? 'text-amber-600' : 'text-gray-500'">
        <template v-if="isTruncated">
          Showing {{ previewTokensLabel }} ({{ displayedCharactersLabel }}) out of {{ tokensLabel }} ({{ totalCharactersLabel }})
        </template>
        <template v-else>
          {{ tokensLabel }} ({{ totalCharactersLabel }})
        </template>
      </p>
      <button
        v-if="showCopyButton"
        type="button"
        @click="copyFullContent"
        class="px-3 py-1 bg-gray-200 text-gray-700 text-xs font-semibold rounded-md hover:bg-gray-300 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-opacity-50"
      >
        {{ copyButtonText }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { ClipboardSetText as WailsClipboardSetText } from '../../../wailsjs/runtime/runtime';
import { CountTokens } from '../../../wailsjs/go/main/App';

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
  tokensExact: {
    type: Number,
    default: null
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
const shouldShowTokens = computed(() => props.showHeader || props.showFooter);

const totalTokens = ref(null);
const previewTokens = ref(null);
let totalTokensRequestId = 0;
let previewTokensRequestId = 0;
let totalTokensTimer = null;
let previewTokensTimer = null;

function formatTokenCount(value) {
  return value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

const tokensLabel = computed(() => {
  if (!shouldShowTokens.value) {
    return null;
  }
  if (totalTokens.value === null) {
    return 'n/a';
  }
  return `${formatTokenCount(totalTokens.value)} tokens`;
});

const tokenCountColorClass = computed(() => {
  const count = totalTokens.value;
  if (typeof count !== 'number' || count <= 0) {
    return 'text-gray-500';
  }
  if (count < 1000000) {
    return 'text-green-600';
  } else if (count <= 4000000) {
    return 'text-yellow-500';
  }
  return 'text-red-600';
});

const previewTokensLabel = computed(() => {
  if (!shouldShowTokens.value) {
    return null;
  }
  if (previewTokens.value === null) {
    return 'n/a';
  }
  return `${formatTokenCount(previewTokens.value)} tokens`;
});
const displayedCharactersLabel = computed(() => `${displayedCharacters.value.toLocaleString()} chars`);
const totalCharactersLabel = computed(() => `${totalCharacters.value.toLocaleString()} chars`);

async function computeTotalTokens(text) {
  const requestId = ++totalTokensRequestId;
  if (!text) {
    totalTokens.value = 0;
    return;
  }
  try {
    const count = await CountTokens(text);
    if (requestId === totalTokensRequestId) {
      totalTokens.value = count;
    }
  } catch (err) {
    console.error('Failed to count tokens:', err);
    if (requestId === totalTokensRequestId) {
      totalTokens.value = null;
    }
  }
}

async function computePreviewTokens(text) {
  const requestId = ++previewTokensRequestId;
  if (!text) {
    previewTokens.value = 0;
    return;
  }
  try {
    const count = await CountTokens(text);
    if (requestId === previewTokensRequestId) {
      previewTokens.value = count;
    }
  } catch (err) {
    console.error('Failed to count preview tokens:', err);
    if (requestId === previewTokensRequestId) {
      previewTokens.value = null;
    }
  }
}

function scheduleTotalTokens(text) {
  if (totalTokensTimer) {
    clearTimeout(totalTokensTimer);
  }
  totalTokensTimer = setTimeout(() => {
    void computeTotalTokens(text);
  }, 200);
}

function schedulePreviewTokens(text) {
  if (previewTokensTimer) {
    clearTimeout(previewTokensTimer);
  }
  previewTokensTimer = setTimeout(() => {
    void computePreviewTokens(text);
  }, 200);
}

watch([() => props.content, () => props.tokensExact, shouldShowTokens], () => {
  if (!shouldShowTokens.value) {
    totalTokens.value = null;
    previewTokens.value = null;
    return;
  }
  if (typeof props.tokensExact === 'number' && props.tokensExact >= 0) {
    totalTokens.value = props.tokensExact;
    return;
  }
  scheduleTotalTokens(props.content);
}, { immediate: true });

watch([displayContent, isTruncated, () => totalTokens.value, shouldShowTokens], () => {
  if (!shouldShowTokens.value) {
    previewTokens.value = null;
    return;
  }
  if (!isTruncated.value) {
    previewTokens.value = totalTokens.value;
    return;
  }
  schedulePreviewTokens(displayContent.value);
}, { immediate: true });

async function copyFullContent() {
  if (!props.content) return;

  // Use navigator.clipboard.writeText as primary on darwin (WailsClipboardSetText has UTF-8 encoding issues with box-drawing chars)
  try {
    if (props.platform === 'darwin') {
      await navigator.clipboard.writeText(props.content);
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

  // Fallback to Wails clipboard API
  try {
    await WailsClipboardSetText(props.content);
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
