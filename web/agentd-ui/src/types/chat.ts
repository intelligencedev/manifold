export type ChatRole = 'user' | 'assistant' | 'tool' | 'system' | 'status'

export interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  createdAt: string
  streaming?: boolean
  title?: string
  error?: string
  toolArgs?: string
  audioUrl?: string
  audioFilePath?: string
}

export interface ChatSessionMeta {
  id: string
  name: string
  createdAt: string
  updatedAt: string
  lastMessagePreview?: string
  model?: string
}
