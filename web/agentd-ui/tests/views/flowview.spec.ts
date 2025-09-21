import { render } from '@testing-library/vue'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

import FlowView from '@/views/FlowView.vue'

const toolsResponse = [
  {
    name: 'web-search',
    description: 'Search the web',
    parameters: {
      type: 'object',
      properties: {
        query: { type: 'string', description: 'Search query' },
        limit: { type: 'integer', minimum: 1 },
      },
      required: ['query'],
    },
  },
]

const workflowsResponse = [
  {
    intent: 'default',
    description: 'Sample workflow',
    steps: [
      {
        id: 'step-1',
        text: 'Start',
        publish_result: true,
        tool: {
          name: 'web-search',
          args: { query: 'hello', limit: 3 },
        },
      },
    ],
  },
]

beforeEach(() => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.url

    if (url.endsWith('/api/warpp/tools')) {
      return new Response(JSON.stringify(toolsResponse), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    if (url.endsWith('/api/warpp/workflows')) {
      return new Response(JSON.stringify(workflowsResponse), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    if (url.includes('/api/warpp/workflows/')) {
      return new Response(JSON.stringify(workflowsResponse[0]), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    return new Response('not found', { status: 404 })
  })

  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('FlowView', () => {
  it('shows tool palette and renders node with editable parameters', async () => {
    const { findByText, findByLabelText, queryByText } = render(FlowView)

    expect(await findByText('Tool Palette')).toBeTruthy()
    expect(await findByText('web-search')).toBeTruthy()

    const stepInput = await findByLabelText('Step Text')
    expect(stepInput).toBeTruthy()

    const queryField = await findByLabelText(/query/i)
    expect(queryField).toBeTruthy()

    expect(queryByText(/Select a node to edit step details/)).toBeNull()
  })
})
