// e.g. WebRetrievalNode.vue.d.ts
export interface WebRetrievalNodeData {
  type: string
  labelStyle?: Record<string, any>
  style?: Record<string, any>
  inputs?: {
    url?: string
  },
  outputs?: {
    // reflect the new property
    result?: {
      output?: string
    }
  },
  hasInputs?: boolean
  hasOutputs?: boolean
  inputHandleColor?: string
  inputHandleShape?: string
  handleColor?: string
  outputHandleShape?: string
  run?: () => Promise<any>
  error?: string
}
