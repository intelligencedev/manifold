import { ref } from 'vue'

/**
 * Global state management for the Code Editor component
 * Allows other components to update code in the editor
 */
const code = ref('console.log("Hello from Wasm!");\n// Try accessing window or document - it should fail\n// Example: console.log(window.location.href);\n')
const isEditorOpen = ref(false)

export function useCodeEditor() {
  /**
   * Set new code in the editor and open it
   * @param {string} newCode - The code to set in the editor
   * @param {boolean} openEditor - Whether to automatically open the editor
   */
  const setEditorCode = (newCode, openEditor = true) => {
    code.value = newCode
    if (openEditor) {
      isEditorOpen.value = true
    }
  }

  /**
   * Open the code editor panel
   */
  const openEditor = () => {
    isEditorOpen.value = true
  }

  /**
   * Close the code editor panel
   */
  const closeEditor = () => {
    isEditorOpen.value = false
  }

  return {
    code,
    isEditorOpen,
    setEditorCode,
    openEditor,
    closeEditor
  }
}