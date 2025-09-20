import { render, fireEvent } from '@testing-library/vue'
import { describe, it, expect } from 'vitest'
import ChatView from '@/views/ChatView.vue'

// Smoke test for chat send interaction

describe('ChatView', () => {
  it('sends a message and echoes', async () => {
    const { getByPlaceholderText, getByText } = render(ChatView)

    const input = getByPlaceholderText('Message the agent...') as HTMLInputElement
    await fireEvent.update(input, 'hello')
    await fireEvent.submit(input.form as HTMLFormElement)

    expect(getByText('hello')).toBeTruthy()
  })
})
