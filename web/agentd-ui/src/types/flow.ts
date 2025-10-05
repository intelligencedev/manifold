import type { WarppStep } from '@/types/warpp'

export interface StepNodeData {
  step: WarppStep
  order: number
  kind?: 'step' | 'utility'
}
