<template>
  <div class="flex h-full overflow-hidden border-t border-gray-200">
    <!-- Sidebar: History List -->
    <div class="w-72 bg-gray-50 dark:bg-slate-900 border-r border-gray-200 dark:border-slate-700 flex flex-col flex-shrink-0">
      <div class="p-3 border-b border-gray-200 dark:border-slate-700 flex justify-between items-center bg-gray-100 dark:bg-slate-800">
        <h3 class="font-semibold text-gray-700 dark:text-gray-300 text-sm">Prompt History</h3>
        <button 
            @click="loadHistory" 
            title="Refresh History"
            class="text-gray-500 hover:text-blue-600 transition-colors"
        >
            ↻
        </button>
      </div>
      
      <div v-if="isLoading" class="p-4 text-center text-gray-500 dark:text-gray-400 text-xs">
        Loading...
      </div>
      
      <div v-else-if="historyItems.length === 0" class="p-4 text-center text-gray-500 dark:text-gray-400 text-xs">
        No history yet. Execute a prompt in Step 2.
      </div>

      <div v-else class="overflow-y-auto flex-1">
        <ul>
          <li 
            v-for="item in historyItems" 
            :key="item.id" 
            @click="selectItem(item)" 
            class="p-3 border-b border-gray-100 cursor-pointer hover:bg-white dark:hover:bg-slate-800 transition-colors"
            :class="{'bg-blue-50 dark:bg-blue-900 border-l-4 border-l-blue-500': selectedItem && selectedItem.id === item.id, 'border-l-4 border-l-transparent': !selectedItem || selectedItem.id !== item.id}"
          >
            <div class="text-sm font-medium text-gray-800 dark:text-gray-200 truncate mb-1" :title="item.userTask">
                {{ item.userTask || 'No task description' }}
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400 flex justify-between">
                <span>{{ formatTime(item.timestamp) }}</span>
                <span>{{ formatDate(item.timestamp) }}</span>
            </div>
          </li>
        </ul>
      </div>
      
      <div class="p-2 border-t border-gray-200 dark:border-slate-700 bg-gray-100 dark:bg-slate-800 text-center">
         <button @click="clearHistory" class="text-xs text-red-500 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300">Clear History</button>
      </div>
    </div>

    <!-- Main Content: Split Pane -->
    <div class="flex-1 flex flex-col h-full overflow-hidden bg-white dark:bg-slate-800 relative" v-if="selectedItem">
        <div class="flex-1 flex flex-row overflow-hidden">
             <!-- Request Pane -->
             <div class="w-1/2 flex flex-col border-r border-gray-200">
                 <div class="p-2 border-b border-gray-200 dark:border-slate-700 flex justify-between items-center bg-gray-50 dark:bg-slate-800">
                     <span class="font-bold text-gray-700 dark:text-gray-300 text-xs uppercase tracking-wider">Raw Request</span>
                     <div class="flex items-center space-x-2">
                       <button @click="copyText(selectedItem.constructedPrompt, 'req')" class="text-xs text-blue-600 hover:text-blue-800 font-medium">
                          {{ copyReqBtnText }}
                       </button>
                       <button
                         v-if="selectedItem && selectedItem.apiCall"
                         @click="openApiCallModal"
                         class="text-xs text-gray-500 hover:text-gray-800 underline"
                       >
                         view api call
                       </button>
                     </div>
                 </div>
                 <div class="relative flex-grow">
                    <textarea 
                        readonly 
                      class="absolute inset-0 w-full h-full p-3 text-xs font-mono resize-none focus:outline-none bg-white dark:bg-slate-800" 
                        :value="selectedItem.constructedPrompt"
                    ></textarea>
                 </div>
             </div>
             
             <!-- Response Pane -->
             <div class="w-1/2 flex flex-col">
                 <div class="p-2 border-b border-gray-200 dark:border-slate-700 flex justify-between items-center bg-gray-50 dark:bg-slate-800">
                     <span class="font-bold text-gray-700 dark:text-gray-300 text-xs uppercase tracking-wider">Response</span>
                     <div class="flex items-center space-x-3">
                        <button @click="copyText(selectedItem.response, 'res')" class="text-xs text-blue-600 hover:text-blue-800 font-medium">
                            {{ copyResBtnText }}
                        </button>
                     </div>
                 </div>
                 <div class="relative flex-grow">
                    <textarea 
                        readonly 
                      class="absolute inset-0 w-full h-full p-3 text-xs font-mono resize-none focus:outline-none bg-white dark:bg-slate-800" 
                        :value="selectedItem.response"
                    ></textarea>
                 </div>
             </div>
        </div>
    </div>
    
    <!-- Empty State -->
    <div v-else class="flex-1 flex flex-col items-center justify-center text-gray-400 dark:text-gray-400 bg-gray-50 dark:bg-slate-900">
        <div class="text-4xl mb-2">🗂️</div>
        <p class="text-gray-500 dark:text-gray-400">Select an item from history to view details</p>
    </div>
    
    <!-- API Call Debug Modal -->
    <div
      v-if="isApiCallModalVisible"
      class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"
    >
      <div class="bg-white dark:bg-slate-800 rounded-lg shadow-xl w-[90%] h-[90%] flex flex-col">
        <div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 dark:border-slate-700">
          <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">LLM API Call</h3>
          <button
            @click="closeApiCallModal"
            class="text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200 text-xl leading-none"
          >
            &times;
          </button>
        </div>
        <textarea
          readonly
          class="flex-1 p-4 font-mono text-xs border-none outline-none resize-none bg-white dark:bg-slate-800"
          :value="currentApiCall"
        ></textarea>
        <div class="flex justify-end space-x-3 px-4 py-3 border-t border-gray-200 bg-gray-50 dark:bg-slate-800">
          <button
            @click="copyApiCall"
            class="text-xs text-blue-600 hover:text-blue-800 font-medium"
          >
            {{ copyApiCallBtnText }}
          </button>
          <button
            @click="closeApiCallModal"
            class="text-xs text-gray-600 hover:text-gray-900"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { GetPromptHistory, ClearPromptHistory } from '../../../wailsjs/go/main/App';
import { LogInfo, LogError } from '../../../wailsjs/runtime/runtime';

const historyItems = ref([]);
const selectedItem = ref(null);
const isLoading = ref(false);

const copyReqBtnText = ref('Copy All');
const copyResBtnText = ref('Copy All');

const isApiCallModalVisible = ref(false);
const currentApiCall = ref('');
const copyApiCallBtnText = ref('Copy All');

onMounted(() => {
    loadHistory();
});

// Refresh history when this component becomes visible (if parent keeps it alive) or via prop changes if needed.
// Since the parent uses v-if, mounted is sufficient.

async function loadHistory() {
    isLoading.value = true;
    try {
        const items = await GetPromptHistory();
        // Items are expected to be sorted newest first by backend
        historyItems.value = items || [];
        if (historyItems.value.length > 0 && !selectedItem.value) {
            selectedItem.value = historyItems.value[0];
        }
    } catch (err) {
        console.error("Failed to load history:", err);
        LogError(`Failed to load history: ${err}`);
    } finally {
        isLoading.value = false;
    }
}

async function clearHistory() {
    if (!confirm("Are you sure you want to clear the prompt history?")) return;
    try {
        await ClearPromptHistory();
        historyItems.value = [];
        selectedItem.value = null;
        LogInfo("History cleared.");
    } catch (err) {
        LogError(`Failed to clear history: ${err}`);
    }
}

function selectItem(item) {
    selectedItem.value = item;
}

function formatTime(ts) {
    if (!ts) return '';
    return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatDate(ts) {
    if (!ts) return '';
    return new Date(ts).toLocaleDateString();
}

async function copyText(text, type) {
    if (!text) return;
    try {
        await navigator.clipboard.writeText(text);
        if (type === 'req') {
            copyReqBtnText.value = 'Copied!';
            setTimeout(() => copyReqBtnText.value = 'Copy All', 2000);
        } else {
            copyResBtnText.value = 'Copied!';
            setTimeout(() => copyResBtnText.value = 'Copy All', 2000);
        }
    } catch (err) {
        console.error('Copy failed:', err);
    }
}

function openApiCallModal() {
    if (!selectedItem.value || !selectedItem.value.apiCall) {
        return;
    }
    currentApiCall.value = selectedItem.value.apiCall;
    isApiCallModalVisible.value = true;
}

function closeApiCallModal() {
    isApiCallModalVisible.value = false;
}

async function copyApiCall() {
    if (!currentApiCall.value) return;
    try {
        await navigator.clipboard.writeText(currentApiCall.value);
        copyApiCallBtnText.value = 'Copied!';
        setTimeout(() => copyApiCallBtnText.value = 'Copy All', 2000);
    } catch (err) {
        console.error('Copy failed:', err);
    }
}

// Expose methods for parent if needed (e.g. to refresh when Step 2 completes)
defineExpose({
    loadHistory
});
</script>
