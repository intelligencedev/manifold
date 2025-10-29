import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { WarppStepTrace } from '@/types/warpp'
import { runWarppWorkflow, type WarppRunResponse } from '@/api/warpp'
import { useProjectsStore } from '@/stores/projects'

export const useWarppRunStore = defineStore('warpp-run', () => {
  const running = ref(false)
  const error = ref('')
  const runOutput = ref('')
  const runLogs = ref<string[]>([])
  const runTrace = ref<Record<string, WarppStepTrace>>({})
  let runAbort: AbortController | null = null

  function reset() {
    error.value = ''
    runOutput.value = ''
    runLogs.value = []
    runTrace.value = {}
  }

  async function startRun(intent: string, prompt?: string): Promise<WarppRunResponse> {
    if (running.value) throw new Error('A run is already in progress')
    running.value = true
    reset()
    runLogs.value.push(`▶ Starting run for intent "${intent}"`)
    runAbort?.abort()
    runAbort = new AbortController()
    try {
      runLogs.value.push('→ POST /api/warpp/run')
      const projStore = useProjectsStore()
      const projectId = projStore.currentProjectId || undefined
      const res = await runWarppWorkflow(intent, prompt ?? `Run workflow: ${intent}`, runAbort.signal, projectId)
      runOutput.value = res.result || ''
      // materialize trace into record for UI access
      const rec: Record<string, WarppStepTrace> = {}
      for (const t of res.trace ?? []) {
        rec[t.stepId] = t
      }
      runTrace.value = rec
      runLogs.value.push('✓ Run finished')
      if (runOutput.value) {
        const snippet = runOutput.value.slice(0, 160) + (runOutput.value.length > 160 ? '…' : '')
        runLogs.value.push('Result snippet: ' + snippet)
      }
      return { result: res.result, trace: res.trace }
    } catch (err: any) {
      if (err?.name === 'AbortError') {
        error.value = 'Run cancelled'
        runLogs.value.push('⚠ Run cancelled by user')
        return { result: '', trace: [] }
      }
      const msg = err?.message ?? 'Failed to run workflow'
      error.value = msg
      runLogs.value.push('✗ Error: ' + msg)
      throw err
    } finally {
      running.value = false
    }
  }

  function cancelRun() {
    if (running.value && runAbort) runAbort.abort()
  }

  return { running, error, runOutput, runLogs, runTrace, startRun, cancelRun }
})

