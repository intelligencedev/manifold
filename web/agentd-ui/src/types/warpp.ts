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
}

export interface WarppWorkflowUI {
  layout?: Record<string, WarppNodeLayout>
}

export interface WarppStep {
  id: string
  text: string
  guard?: string
  publish_result?: boolean
  tool?: WarppToolRef
}

export interface WarppWorkflow {
  intent: string
  description?: string
  keywords?: string[]
  steps: WarppStep[]
  ui?: WarppWorkflowUI
}
