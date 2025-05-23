<template>
  <div :class="['fixed top-[64px] bottom-0 w-64 z-[1100] transition-transform duration-300 text-white bg-zinc-900 flex flex-col space-y-2 shadow-[5px_0_10px_-5px_rgba(0,0,0,0.3)] tracking-wider', isOpen ? 'translate-x-0' : '-translate-x-full']">
    <div class="absolute top-1/2 right-0 translate-x-full w-[30px] h-[60px] bg-zinc-900 rounded-r-md flex items-center justify-center cursor-pointer shadow-[5px_0_10px_-5px_rgba(0,0,0,0.3)]" @click="togglePalette">
      <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd" d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
    </div>
    <div class="p-4 h-full box-border">
      <div class="overflow-y-auto h-full pr-2 space-y-4 palette-scroll">
        <div v-for="(nodes, category) in nodeCategories" :key="category">
          <div class="text-base font-bold mb-2 uppercase text-gray-300 cursor-pointer flex justify-between items-center p-2 bg-zinc-800 border border-teal-600 rounded hover:bg-white/20" @click="toggleAccordion(category)">
            <span>{{ category }}</span>
            <svg v-if="category === 'Chat/Agent'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none"><path fill="currentColor" d="m13.087 21.388l.645.382zm.542-.916l-.646-.382zm-3.258 0l-.645.382zm.542.916l.646-.382zm-8.532-5.475l.693-.287zm5.409 3.078l-.013.75zm-2.703-.372l-.287.693zm16.532-2.706l.693.287zm-5.409 3.078l-.012-.75zm2.703-.372l.287.693zm.7-15.882l-.392.64zm1.65 1.65l.64-.391zM4.388 2.738l-.392-.64zm-1.651 1.65l-.64-.391zM9.403 19.21l.377-.649zm4.33 2.56l.541-.916l-1.29-.764l-.543.916zm-4.007-.916l.542.916l1.29-.764l-.541-.916zm2.715.152a.52.52 0 0 1-.882 0l-1.291.764c.773 1.307 2.69 1.307 3.464 0zM10.5 2.75h3v-1.5h-3zm10.75 7.75v1h1.5v-1zm-18.5 1v-1h-1.5v1zm-1.5 0c0 1.155 0 2.058.05 2.787c.05.735.153 1.347.388 1.913l1.386-.574c-.147-.352-.233-.782-.278-1.441c-.046-.666-.046-1.51-.046-2.685zm6.553 6.742c-1.256-.022-1.914-.102-2.43-.316L4.8 19.313c.805.334 1.721.408 2.977.43zM1.688 16.2A5.75 5.75 0 0 0 4.8 19.312l.574-1.386a4.25 4.25 0 0 1-2.3-2.3zm19.562-4.7c0 1.175 0 2.019-.046 2.685c-.045.659-.131 1.089-.277 1.441l1.385.574c.235-.566.338-1.178.389-1.913c.05-.729.049-1.632.049-2.787zm-5.027 8.241c1.256-.021 2.172-.095 2.977-.429l-.574-1.386c-.515.214-1.173.294-2.428.316zm4.704-4.115a4.25 4.25 0 0 1-2.3 2.3l.573 1.386a5.75 5.75 0 0 0 3.112-3.112zM13.5 2.75c1.651 0 2.837 0 3.762.089c.914.087 1.495.253 1.959.537l.783-1.279c-.739-.452-1.577-.654-2.6-.752c-1.012-.096-2.282-.095-3.904-.095zm9.25 7.75c0-1.622 0-2.891-.096-3.904c-.097-1.023-.299-1.862-.751-2.6l-1.28.783c.285.464.451 1.045.538 1.96c.088.924.089 2.11.089 3.761zm-3.53-7.124a4.25 4.25 0 0 1 1.404 1.403l1.279-.783a5.75 5.75 0 0 0-1.899-1.899zM10.5 1.25c-1.622 0-2.891 0-3.904.095c-1.023.098-1.862.3-2.6.752l.783 1.28c.464-.285 1.045-.451 1.96-.538c.924-.088 2.11-.089 3.761-.089zM2.75 10.5c0-1.651 0-2.837.089-3.762c.087-.914.253-1.495.537-1.959l-1.279-.783c-.452.738-.654 1.577-.752 2.6C1.25 7.61 1.25 8.878 1.25 10.5zm1.246-8.403a5.75 5.75 0 0 0-1.899 1.899l1.28.783a4.25 4.25 0 0 1 1.402-1.403zm7.02 17.993c-.202-.343-.38-.646-.554-.884a2.2 2.2 0 0 0-.682-.645l-.754 1.297c.047.028.112.078.224.232c.121.166.258.396.476.764zm-3.24-.349c.44.008.718.014.93.037c.198.022.275.054.32.08l.754-1.297a2.2 2.2 0 0 0-.909-.274c-.298-.033-.657-.038-1.069-.045zm6.498 1.113c.218-.367.355-.598.476-.764c.112-.154.177-.204.224-.232l-.754-1.297c-.29.17-.5.395-.682.645c-.173.238-.352.54-.555.884zm1.924-2.612c-.412.007-.771.012-1.069.045c-.311.035-.616.104-.909.274l.754 1.297c.045-.026.122-.058.32-.08c.212-.023.49-.03.93-.037z"></path><path stroke="currentColor" stroke-linecap="round" stroke-width="1.5" d="M8 9h8m-8 3.5h5.5"></path></g></svg>
            <svg v-if="category === 'Image Gen'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 12c0-4.714 0-7.071 1.464-8.536C4.93 2 7.286 2 12 2s7.071 0 8.535 1.464C22 4.93 22 7.286 22 12s0 7.071-1.465 8.535C19.072 22 16.714 22 12 22s-7.071 0-8.536-1.465C2 19.072 2 16.714 2 12Z"></path><circle cx="16" cy="8" r="2"></circle><path stroke-linecap="round" d="m2 12.5l1.752-1.533a2.3 2.3 0 0 1 3.14.105l4.29 4.29a2 2 0 0 0 2.564.222l.299-.21a3 3 0 0 1 3.731.225L21 18.5"></path></g></svg>
            <svg v-if="category === 'Code'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 12c0-4.714 0-7.071 1.464-8.536C4.93 2 7.286 2 12 2s7.071 0 8.535 1.464C22 4.93 22 7.286 22 12s0 7.071-1.465 8.535C19.072 22 16.714 22 12 22s-7.071 0-8.536-1.465C2 19.072 2 16.714 2 12Z"></path><path stroke-linecap="round" d="M17 15h-5m-5-5l.234.195c1.282 1.068 1.923 1.602 1.923 2.305s-.64 1.237-1.923 2.305L7 15"></path></g></svg>
            <svg v-if="category === 'Web'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"><path d="M6.286 19C3.919 19 2 17.104 2 14.765s1.919-4.236 4.286-4.236q.427.001.83.08m7.265-2.582a5.8 5.8 0 0 1 1.905-.321c.654 0 1.283.109 1.87.309m-11.04 2.594a5.6 5.6 0 0 1-.354-1.962C6.762 5.528 9.32 3 12.476 3c2.94 0 5.361 2.194 5.68 5.015m-11.04 2.594a4.3 4.3 0 0 1 1.55.634m9.49-3.228C20.392 8.78 22 10.881 22 13.353c0 2.707-1.927 4.97-4.5 5.52"></path><path stroke-linejoin="round" d="M12 22v-6m0 6l2-2m-2 2l-2-2"></path></g></svg>
            <svg v-if="category === 'Documents'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none"><path fill="currentColor" d="m15.393 4.054l-.502.557zm3.959 3.563l-.502.557zm2.302 2.537l-.685.305zM3.172 20.828l.53-.53zm17.656 0l-.53-.53zM14 21.25h-4v1.5h4zM2.75 14v-4h-1.5v4zm18.5-.437V14h1.5v-.437zM14.891 4.61l3.959 3.563l1.003-1.115l-3.958-3.563zm7.859 8.952c0-1.689.015-2.758-.41-3.714l-1.371.61c.266.598.281 1.283.281 3.104zm-3.9-5.389c1.353 1.218 1.853 1.688 2.119 2.285l1.37-.61c-.426-.957-1.23-1.66-2.486-2.79zM10.03 2.75c1.582 0 2.179.012 2.71.216l.538-1.4c-.852-.328-1.78-.316-3.248-.316zm5.865.746c-1.086-.977-1.765-1.604-2.617-1.93l-.537 1.4c.532.204.98.592 2.15 1.645zM10 21.25c-1.907 0-3.261-.002-4.29-.14c-1.005-.135-1.585-.389-2.008-.812l-1.06 1.06c.748.75 1.697 1.081 2.869 1.239c1.15.155 2.625.153 4.489.153zM1.25 14c0 1.864-.002 3.338.153 4.489c.158 1.172.49 2.121 1.238 2.87l1.06-1.06c-.422-.424-.676-1.004-.811-2.01c-.138-1.027-.14-2.382-.14-4.289zM14 22.75c1.864 0 3.338.002 4.489-.153c1.172-.158 2.121-.49 2.87-1.238l-1.06-1.06c-.424.422-1.004.676-2.01.811c-1.027.138-2.382.14-4.289.14zM21.25 14c0 1.907-.002 3.262-.14 4.29c-.135 1.005-.389 1.585-.812 2.008l1.06 1.06c.75-.748 1.081-1.697 1.239-2.869c.155-1.15.153-2.625.153-4.489zm-18.5-4c0-1.907.002-3.261.14-4.29c.135-1.005.389-1.585.812-2.008l-1.06-1.06c-.75.748-1.081 1.697-1.239 2.869C1.248 6.661 1.25 8.136 1.25 10zm7.28-8.75c-1.875 0-3.356-.002-4.511.153c-1.177.158-2.129.49-2.878 1.238l1.06 1.06c.424-.422 1.005-.676 2.017-.811c1.033-.138 2.395-.14 4.312-.14z"></path><path stroke="currentColor" stroke-linecap="round" stroke-width="1.5" d="M6 14.5h8M6 18h5.5"></path><path stroke="currentColor" stroke-width="1.5" d="M13 2.5V5c0 2.357 0 3.536.732 4.268S15.643 10 18 10h4"></path></g></svg>
            <svg v-if="category === 'Utilities'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 12c0-4.714 0-7.071 1.464-8.536C4.93 2 7.286 2 12 2s7.071 0 8.535 1.464C22 4.93 22 7.286 22 12s0 7.071-1.465 8.535C19.072 22 16.714 22 12 22s-7.071 0-8.536-1.465C2 19.072 2 16.714 2 12Z"></path><path d="M10 14a2 2 0 1 1 0 4a2 2 0 0 1 0-4Z"></path><circle cx="2" cy="2" r="2" transform="matrix(0 -1 -1 0 16 10)"></circle><path stroke-linecap="round" d="M14 16h5m-9-8H5m0 8h1m13-8h-1"></path></g></svg>
            <svg v-if="category === 'Tools'" xmlns="http://www.w3.org/2000/svg" width="24px" height="24px" viewBox="0 0 24 24" class="ml-auto mr-2"><!-- Icon from Solar by 480 Design - https://creativecommons.org/licenses/by/4.0/ --><g fill="none" stroke="currentColor" stroke-width="1.5"><path d="M7 12H6c-1.886 0-2.828 0-3.414.586S2 14.114 2 16v2c0 1.886 0 2.828.586 3.414S4.114 22 6 22h2c1.886 0 2.828 0 3.414-.586S12 19.886 12 18v-1"></path><path d="M12 7h-1c-1.886 0-2.828 0-3.414.586S7 9.114 7 11v2c0 1.886 0 2.828.586 3.414S9.114 17 11 17h2c1.886 0 2.828 0 3.414-.586S17 14.886 17 13v-1"></path><path d="M12 6c0-1.886 0-2.828.586-3.414S14.114 2 16 2h2c1.886 0 2.828 0 3.414.586S22 4.114 22 6v2c0 1.886 0 2.828-.586 3.414S19.886 12 18 12h-2c-1.886 0-2.828 0-3.414-.586S12 9.886 12 8z"></path></g></svg>
            <svg v-if="isExpanded(category)" xmlns="http://www.w3.org/2000/svg" class="w-4 h-4 transition-transform duration-300 rotate-90" fill="currentColor" viewBox="0 0 16 16">
              <path fill-rule="evenodd" d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" class="w-4 h-4 transition-transform duration-300" fill="currentColor" viewBox="0 0 16 16">
              <path fill-rule="evenodd" d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
            </svg>
          </div>
          <div 
            class="overflow-hidden transition-all duration-300 ease-in-out"
            :style="{ maxHeight: isExpanded(category) ? contentHeights[category] || 'auto' : '0', opacity: isExpanded(category) ? 1 : 0 }"
            :ref="el => { if (el) contentRefs[category] = el }"
          >
            <div class="pl-2 space-y-3 pb-2" :ref="el => { if (el) innerContentRefs[category] = el }">
              <div 
                v-for="node in nodes" 
                :key="node.type" 
                class="p-3 bg-teal-700 border border-teal-600 rounded cursor-grab text-base font-medium flex items-center justify-center hover:bg-white/20" 
                draggable="true" 
                @dragstart="(event) => onDragStart(event, node.type)"
              >
                {{ node.type }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, nextTick, watch } from 'vue'
import useDragAndDrop from '../composables/useDnD.js'
import useNodePalette from '../composables/useNodePalette.js'

const { onDragStart } = useDragAndDrop()
const { isOpen, togglePalette, nodeCategories, toggleAccordion, isExpanded } = useNodePalette()

// Refs to manage the content heights for animations
const contentRefs = reactive({})
const innerContentRefs = reactive({})
const contentHeights = reactive({})

// Update content heights when categories are expanded/collapsed
watch(() => ({ ...nodeCategories }), async () => {
  await nextTick()
  updateContentHeights()
}, { immediate: true, deep: true })

// Initialize content heights after mount
onMounted(async () => {
  await nextTick()
  updateContentHeights()
})

// Watch for changes to expanded state
Object.keys(nodeCategories).forEach(category => {
  watch(() => isExpanded(category), async (expanded) => {
    if (expanded) {
      // First set a fixed height for animation
      if (innerContentRefs[category]) {
        contentHeights[category] = `${innerContentRefs[category].scrollHeight}px`
      }
      
      // Then switch to auto after animation completes
      setTimeout(() => {
        if (isExpanded(category)) {
          contentHeights[category] = 'auto'
        }
      }, 300)
    } else {
      // Set a fixed height first (for animation from auto)
      if (innerContentRefs[category]) {
        contentHeights[category] = `${innerContentRefs[category].scrollHeight}px`
        
        // Force a reflow to ensure the browser registers the change
        if (contentRefs[category]) {
          contentRefs[category].offsetHeight
        }
        
        // Then animate to zero
        contentHeights[category] = '0px'
      }
    }
  })
})

// Update all content heights
const updateContentHeights = () => {
  Object.keys(nodeCategories).forEach(category => {
    if (isExpanded(category) && innerContentRefs[category]) {
      contentHeights[category] = `${innerContentRefs[category].scrollHeight}px`
      
      // Switch to auto for already expanded categories
      setTimeout(() => {
        if (isExpanded(category)) {
          contentHeights[category] = 'auto'
        }
      }, 300)
    }
  })
}
</script>

<style scoped>
.palette-scroll {
  scrollbar-width: thin;
  scrollbar-color: #14b8a6 transparent; /* teal-500 thumb, no track */
}

.palette-scroll::-webkit-scrollbar {
  width: 6px;
}

.palette-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.palette-scroll::-webkit-scrollbar-thumb {
  background-color: #14b8a6;
  border-radius: 9999px;
}
</style>
