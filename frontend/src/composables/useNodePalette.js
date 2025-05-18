import { ref, reactive } from 'vue'
import { getNodeCategories } from '../components/nodes/nodeRegistry.ts'

const isOpen = ref(false)
const nodeCategories = getNodeCategories()
const expandedCategories = reactive({})
Object.keys(nodeCategories).forEach((category) => {
  expandedCategories[category] = category === 'Text Completions'
})

function togglePalette() {
  isOpen.value = !isOpen.value
}

function toggleAccordion(category) {
  expandedCategories[category] = !expandedCategories[category]
}

function isExpanded(category) {
  return expandedCategories[category]
}

export default function useNodePalette() {
  return {
    isOpen,
    togglePalette,
    nodeCategories,
    toggleAccordion,
    isExpanded,
  }
}
