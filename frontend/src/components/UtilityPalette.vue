<template>
    <div class="utility-palette" :class="{ 'is-open': isOpen }">
      <div class="toggle-button" @click="togglePalette">
        <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
          viewBox="0 0 16 16">
          <path fill-rule="evenodd"
            d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
        </svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
          <path fill-rule="evenodd"
            d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708z" />
        </svg>
      </div>
  
      <div class="utility-content">
        <div class="scrollable-content">
          <div class="config-card">
            <h3>Configuration</h3>
            <div v-if="configStore.loading">Loading...</div>
            <div v-else-if="configStore.error">Error: {{ configStore.error }}</div>
            <div v-else>
              <pre>{{ configStore.config }}</pre>
            </div>
          </div>
        </div>
      </div>
    </div>
  </template>
  
  <script setup lang="ts">
  import { ref, onMounted } from 'vue'
  import { useConfigStore } from '@/stores/configStore'
  
  const isOpen = ref(false)
  const configStore = useConfigStore()
  
  // Fetch config when component mounts
  onMounted(() => {
    configStore.fetchConfig()
  })
  
  function togglePalette() {
    isOpen.value = !isOpen.value
  }
  </script>
  
  <style scoped>
  .utility-palette {
    position: fixed;
    top: 50px;
    bottom: 0px;
    right: 0;
    width: 300px;
    background-color: #222;
    color: #eee;
    z-index: 1100;
    transition: transform 0.3s ease-in-out;
    transform: translateX(100%);
  }
  
  .utility-palette.is-open {
    transform: translateX(0);
  }
  
  .toggle-button {
    position: absolute;
    top: 50%;
    left: -30px;
    width: 30px;
    height: 60px;
    background-color: #222;
    border: 1px solid #666;
    border-right: none;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    border-top-left-radius: 8px;
    border-bottom-left-radius: 8px;
  }
  
  .toggle-button svg {
    fill: #eee;
    width: 16px;
    height: 16px;
  }
  
  .utility-content {
    padding: 10px;
    height: 100%;
    box-sizing: border-box;
  }
  
  .scrollable-content {
    overflow-y: auto;
    height: 100%;
    padding-right: 10px;
  }
  
  .scrollable-content::-webkit-scrollbar {
    width: 8px;
  }
  
  .scrollable-content::-webkit-scrollbar-track {
    background: #333;
    border-radius: 4px;
  }
  
  .scrollable-content::-webkit-scrollbar-thumb {
    background-color: #666;
    border-radius: 4px;
    border: 2px solid #333;
  }
  
  .scrollable-content::-webkit-scrollbar-thumb:hover {
    background-color: #888;
  }
  
  .config-card {
    background-color: #333;
    padding: 15px;
    border-radius: 8px;
    box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
    color: #eee;
    font-family: 'Roboto', sans-serif;
  }
  
  .config-card h3 {
    margin-bottom: 10px;
  }
  
  pre {
    background: #222;
    padding: 10px;
    border-radius: 5px;
    overflow-x: auto;
  }
  </style>
  