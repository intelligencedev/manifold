export interface WarppTool {
  name: string
  description?: string
  parameters?: Record<string, any>
}

export interface WarppToolRef {
  name: string
  args?: Record<string, any>
}

export interface WarppNodeLayout {
  x: number
  y: number
  width?: number
  height?: number
}

export interface WarppGroupUIEntry {
  id: string
  label: string
  collapsed?: boolean
  color?: string
}

export interface WarppNoteUIEntry {
  id: string
  label?: string
  color?: string
  note?: string
}

export interface WarppWorkflowUI {
  layout?: Record<string, WarppNodeLayout>
  parents?: Record<string, string>
  groups?: WarppGroupUIEntry[]
  // editor-only notes; persisted client-side, optionally echoed to server ui
  notes?: WarppNoteUIEntry[]
}

export interface WarppStep {
  id: string
  text: string
  guard?: string
  publish_result?: boolean
  publish_mode?: 'immediate' | 'topo'
  continue_on_error?: boolean
  tool?: WarppToolRef
  depends_on?: string[]
}

export interface WarppWorkflow {
  intent: string
  description?: string
  keywords?: string[]
  max_concurrency?: number
  fail_fast?: boolean
  steps: WarppStep[]
  ui?: WarppWorkflowUI
}

export type WarppTraceStatus = 'completed' | 'skipped' | 'noop' | 'error'

export interface WarppStepTrace {
  stepId: string
  text?: string
  renderedArgs?: Record<string, any>
  delta?: Record<string, any>
  payload?: unknown
  status?: WarppTraceStatus
  error?: string
}
