<template>
  <ul class="file-tree">
    <li v-for="node in nodes" :key="node.path" :class="{ 'excluded-node': node.excluded }">
      <div class="node-item" :style="{ 'padding-left': depth * 20 + 'px' }">
        <span v-if="node.isDir" @click="toggleExpand(node)" class="toggler">
          {{ node.expanded ? '▼' : '▶' }}
        </span>
        <span v-else class="item-spacer"></span>
        
        <input 
          type="checkbox" 
          :checked="!node.excluded" 
          @change="handleCheckboxChange(node)"
          class="exclude-checkbox"
        />
        <span @click="node.isDir ? toggleExpand(node) : null" :class="{ 'folder-name': node.isDir }">
          {{ node.name }}
        </span>
      </div>
      <FileTree 
        v-if="node.isDir && node.expanded && node.children" 
        :nodes="node.children" 
        :project-root="projectRoot"
        :depth="depth + 1"
        @toggle-exclude="emitToggleExclude"
      />
    </li>
  </ul>
</template>

<script setup>
import { defineProps, defineEmits } from 'vue';

const props = defineProps({
  nodes: Array,
  projectRoot: String,
  depth: {
    type: Number,
    default: 0
  },
  parentExcluded: { // Whether an ancestor is excluded
    type: Boolean,
    default: false
  }
});

const emit = defineEmits(['toggle-exclude']);

function toggleExpand(node) {
  if (node.isDir) {
    node.expanded = !node.expanded;
  }
}

function handleCheckboxChange(node) {
  // Emit an event with the node to toggle its exclusion status in the parent (App.vue)
  emit('toggle-exclude', node);
}

function emitToggleExclude(node) {
    emit('toggle-exclude', node); // Bubble up the event
}

// A node is effectively excluded if one of its PARENTS is.
// This is mainly for UI state (e.g., disabling checkbox), backend handles true exclusion.
function isEffectivelyExcludedByParent(node) {
    let current = node.parent; 
    while(current) {
        if (current.excluded) return true;
        current = current.parent;
    }
    return false;
}

</script>