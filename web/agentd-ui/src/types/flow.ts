import type { WarppStep } from '@/types/warpp'

export interface StepNodeData {
  step: WarppStep
  order: number
  kind?: 'step' | 'utility'
  // UI-only: whether the node card is collapsed to its header
  collapsed?: boolean
  groupId?: string
}

export interface GroupNodeData {
  kind: 'group'
  label: string
  collapsed?: boolean
  color?: string
}
