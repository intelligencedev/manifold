import { ref, computed } from 'vue'

export function useDocumentsIngest() {
  const mode = ref('documents')
  const ingestion_endpoint = ref('http://localhost:8080/api/sefii/ingest')
  const gitRepoPath = ref('')
  const language = ref('DEFAULT')
  const chunk_size = ref(1000)
  const chunk_overlap = ref(550)
  const selectedFiles = ref([])
  const selectionType = ref('file')

  const selectedFileNames = computed(() => selectedFiles.value.map(file => file.name))
  const currentEndpoint = computed(() => {
    if (mode.value === 'documents') {
      return 'http://localhost:8080/api/sefii/ingest'
    }
    if (mode.value === 'git') {
      return 'http://localhost:8080/api/git-files/ingest'
    }
    return ingestion_endpoint.value
  })

  async function callIngestAPI(text, filePath, fileLanguage) {
    const payload = {
      text,
      language: fileLanguage || language.value,
      chunk_size: chunk_size.value,
      chunk_overlap: chunk_overlap.value,
      file_path: filePath,
      doc_title: filePath
    }
    
    const headers = {
      'Content-Type': 'application/json',
    }
    if (filePath) {
      headers['X-File-Path'] = filePath
    }
    
    const response = await fetch(ingestion_endpoint.value, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }
    return await response.json()
  }

  async function callGitIngestAPI(repoPath) {
    const payload = {
      repo_path: repoPath,
      chunk_size: chunk_size.value,
      chunk_overlap: chunk_overlap.value,
    }

    const response = await fetch(currentEndpoint.value, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }
    return await response.json()
  }

  async function callPathIngestAPI(directory) {
    const payload = {
      directory,
      chunk_size: chunk_size.value,
      chunk_overlap: chunk_overlap.value
    }

    const response = await fetch(currentEndpoint.value, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }
    return await response.json()
  }

  function readFileAsText(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = (e) => resolve(e.target.result)
      reader.onerror = (e) => reject(e)
      reader.readAsText(file)
    })
  }

  function handleFileSelection(files) {
    selectedFiles.value = []
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      if (
        (file.type && file.type.startsWith('text/')) ||
        !file.type ||
        file.name.match(/\.(txt|md|csv|json)$/i)
      ) {
        selectedFiles.value.push(file)
      }
    }
  }

  return {
    mode,
    ingestion_endpoint,
    gitRepoPath,
    language,
    chunk_size,
    chunk_overlap,
    selectedFiles,
    selectionType,
    selectedFileNames,
    currentEndpoint,
    callIngestAPI,
    callGitIngestAPI,
    callPathIngestAPI,
    readFileAsText,
    handleFileSelection
  }
}