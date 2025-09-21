import type { WarppStep, WarppTool } from '@/types/warpp'

export interface StepNodeData {
  step: WarppStep
  order: number
  toolDefinition?: WarppTool | null
}
