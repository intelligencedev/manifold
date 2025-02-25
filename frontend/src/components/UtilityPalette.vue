<template>
  <div class="utility-palette" :class="{ 'is-open': isOpen }">
    <div class="toggle-button" @click="togglePalette">
      <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
        viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
        viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708z" />
      </svg>
    </div>

    <div class="utility-content">
      <div class="scrollable-content">

        <!-- Accordion -->
        <div class="accordion">

          <!-- Search Card Accordion Item -->
          <div class="accordion-item">
            <div class="accordion-header" @click="toggleAccordion('search')">
              <h3 class="accordion-title">Model Search</h3>
              <span class="accordion-icon" :class="{ 'is-open': accordionOpen.search }">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-chevron-down"
                  viewBox="0 0 16 16">
                  <path fill-rule="evenodd"
                    d="M1.646 4.646a.5.5 0 0 1 .708 0L8 10.293l5.646-5.647a.5.5 0 0 1 .708.708l-6 6a.5.5 0 0 1-.708 0l-6-6a.5.5 0 0 1 0-.708z" />
                </svg>
              </span>
            </div>
            <div class="accordion-content" v-if="accordionOpen.search">
              <!-- Search Card Content -->
              <div class="search-card">
                <div class="form-group">
                  <label for="searchQuery" class="input-label">Search Query</label>
                  <input type="text" id="searchQuery" v-model="searchQuery" placeholder="Enter search term"
                    class="input-text" />
                </div>
                <div class="form-group">
                  <label for="limit" class="input-label">Results Limit</label>
                  <input type="number" id="limit" v-model.number="limit" min="1" class="input-number" />
                </div>
                <button @click="performSearch" :disabled="isSearching" class="search-button">
                  {{ isSearching ? "Searching..." : "Search" }}
                </button>
                <div v-if="searchError" class="error-message">Error: {{ searchError }}</div>
                <div v-if="results.length > 0" class="results">
                  <ul>
                    <li v-for="model in results" :key="model.id" class="result-item">
                      <!-- Model name is now a link that opens in a new tab -->
                      <a :href="`https://huggingface.co/${model.name}`" target="_blank" rel="noopener noreferrer"
                        class="result-link">
                        <strong>{{ model.name }}</strong>
                      </a>
                    </li>
                  </ul>
                  <div class="pagination">
                    <button @click="prevPage" :disabled="page === 1" class="pagination-button">Previous</button>
                    <span class="page-number">Page {{ page }}</span>
                    <button @click="nextPage" :disabled="results.length < limit" class="pagination-button">Next</button>
                  </div>
                </div>
              </div>
              <!-- End Search Card Content -->
            </div>
          </div>
          <!-- End Search Card Accordion Item -->

          <!-- Configuration Card Accordion Item -->
          <div class="accordion-item">
            <div class="accordion-header" @click="toggleAccordion('config')">
              <h3 class="accordion-title">Configuration</h3>
              <span class="accordion-icon" :class="{ 'is-open': accordionOpen.config }">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-chevron-down"
                  viewBox="0 0 16 16">
                  <path fill-rule="evenodd"
                    d="M1.646 4.646a.5.5 0 0 1 .708 0L8 10.293l5.646-5.647a.5.5 0 0 1 .708.708l-6 6a.5.5 0 0 1-.708 0l-6-6a.5.5 0 0 1 0-.708z" />
                </svg>
              </span>
            </div>
            <div class="accordion-content" v-if="accordionOpen.config">
              <!-- Configuration Card Content -->
              <div class="config-card">
                <div v-if="configStore.loading" class="loading-message">Loading...</div>
                <div v-else-if="configStore.error" class="error-message">Error: {{ configStore.error }}</div>
                <div v-else>
                  <pre class="config-display">{{ configStore.config }}</pre>
                </div>
              </div>
              <!-- End Configuration Card Content -->
            </div>
          </div>
          <!-- End Configuration Card Accordion Item -->

        </div>
        <!-- End Accordion -->

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useConfigStore } from '@/stores/configStore'
import { listModels } from '@huggingface/hub'

const isOpen = ref(false)
const configStore = useConfigStore()

// Accordion State
const accordionOpen = ref({
  search: true,  // Start with search open
  config: false,
})

onMounted(() => {
  configStore.fetchConfig()
})

function togglePalette() {
  isOpen.value = !isOpen.value
}

function toggleAccordion(item: 'search' | 'config') {
  for (const key in accordionOpen.value) {
    // Close all other items
    if (key !== item) {
      accordionOpen.value[key as 'search' | 'config'] = false;
    }
  }
  // Toggle the clicked item
  accordionOpen.value[item] = !accordionOpen.value[item];
}

// --- Search Card Logic ---
const searchQuery = ref('')
const limit = ref(10)  // Default limit
const page = ref(1)
const results = ref<Array<any>>([])
const isSearching = ref(false)
const searchError = ref('')

async function performSearch() {
  if (!searchQuery.value) {
    searchError.value = "Please enter a search query."
    return
  }
  searchError.value = ''
  isSearching.value = true
  try {
    // Retrieve all matching models (ignoring limit/offset on the API call)
    const modelsIterator = listModels({
      search: {
        query: searchQuery.value,
        task: 'text-generation'  //  filter to text-generation models
      },
    })
    const allModels = []
    for await (const model of modelsIterator) {
      allModels.push(model)
    }
    // Manually paginate by slicing the complete result array.
    const offset = (page.value - 1) * limit.value
    results.value = allModels.slice(offset, offset + limit.value)
  } catch (err: any) {
    console.error("Error during search:", err)
    searchError.value = err.message || 'An error occurred during search.'
    results.value = [] // Clear results on error
  } finally {
    isSearching.value = false
  }
}

function nextPage() {
  page.value += 1
  performSearch()  // Re-fetch with new page
}

function prevPage() {
  if (page.value > 1) {
    page.value -= 1
    performSearch() // Re-fetch with new page
  }
}
</script>

<style scoped>
/* Utility Palette Container */
.utility-palette {
  position: fixed;
  top: 50px;
  bottom: 0;
  right: 0;
  width: 300px;
  background-color: #222;
  color: #eee;
  z-index: 1100;
  transition: transform 0.3s ease-in-out;
  transform: translateX(100%);
  box-shadow: -2px 0 5px rgba(0, 0, 0, 0.2);
  border-left: 1px solid #444;
  font-family: 'Roboto', sans-serif; /* Consistent font */
}

.utility-palette.is-open {
  transform: translateX(0);
}

/* Toggle Button */
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
  box-shadow: -2px 0 5px rgba(0, 0, 0, 0.2); /* Add shadow */
}

.toggle-button svg {
  fill: #eee;
  width: 16px;
  height: 16px;
}

/* Utility Content */
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

/* Scrollbar Styles */
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

/* Accordion Styles */
.accordion-item {
  margin-bottom: 10px;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid #444; /* Add border to accordion items */
}

.accordion-header {
  background-color: #333;
  padding: 12px 15px; /* Slightly smaller padding */
  color: #eee;
  font-weight: bold;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #444; /* Add border */
}

.accordion-title {
  margin: 0;
  font-size: 1rem; /* Consistent font size */
}

.accordion-icon {
  transition: transform 0.3s ease;
  display: flex; /* Use flex for alignment */
  align-items: center; /* Center vertically */
}

.accordion-icon.is-open {
  transform: rotate(180deg);
}

.accordion-content {
  background-color: #2a2a2a; /* Slightly lighter content background */
  padding: 15px;
  border-bottom-left-radius: 8px;  /* Round bottom corners */
  border-bottom-right-radius: 8px;
}

/* Configuration Card Styles */
.config-card {
  padding: 15px;
  border-radius: 8px;
  color: #eee;
  margin-bottom: 0; /* Remove bottom margin */
}

.config-display {
  background: #222;
  padding: 10px;
  border-radius: 5px;
  overflow-x: auto;
  white-space: pre-wrap; /* Preserve newlines and spaces */
  font-family: 'Courier New', monospace; /* Monospace font */
  font-size: 0.9rem;
}

.loading-message,
.error-message {
  padding: 10px;
  color: #eee;
}
.error-message {
    color: #f88;
}

/* Search Card Styles */
.search-card {
  padding: 15px;
  border-radius: 8px;
  color: #eee;
  margin-bottom: 0;  /* Remove bottom margin */
}

.form-group {
  margin-bottom: 10px;
}

.input-label {
  display: block;
  margin-bottom: 5px;
  font-size: 0.9rem;
}

.input-text,
.input-number {
  width: 100%;
  padding: 8px;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #222;
  color: #eee;
  box-sizing: border-box;
  font-size: 0.9rem; /* Consistent font size */
}

.search-button {
  padding: 8px 16px;
  background-color: #555;
  color: #eee;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  margin-top: 10px;
  font-size: 0.9rem;
  transition: background-color 0.2s ease; /* Add transition */
}

.search-button:disabled {
  background-color: #777;
  cursor: not-allowed;
}
.search-button:hover:enabled { /* Hover effect */
    background-color: #666;
}

.results {
  margin-top: 15px;
}

.results ul {
  list-style-type: none;
  padding: 0;
}

.result-item {
  padding: 8px 0; /* Slightly more padding */
  border-bottom: 1px solid #444;
}
.result-item:last-child { /* Remove last border */
    border-bottom: none;
}

.result-link {
  color: #eee;
  text-decoration: none; /* Remove underline */
  font-size: 0.9rem;
  transition: color 0.2s ease;
}
.result-link:hover {
    color: #007bff;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 10px;
}

.pagination-button {
  padding: 5px 10px;
  background-color: #555;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9rem;
  color: #eee;
  transition: background-color 0.2s ease;
}

.pagination-button:disabled {
  background-color: #777;
  cursor: not-allowed;
}
.pagination-button:hover:enabled {
    background-color: #666;
}

.page-number {
  color: #eee;
  font-size: 0.9rem;
}
</style>