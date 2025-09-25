import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
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
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
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
      // Support both GET (load) and PUT (save) of a single workflow
      if (!init || !init.method || init.method === 'GET') {
        return new Response(JSON.stringify(workflowsResponse[0]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (init && init.method === 'PUT') {
        // Echo back payload as saved workflow to simulate persistence
        try {
          const body = init.body ? JSON.parse(init.body as string) : workflowsResponse[0]
          return new Response(JSON.stringify(body), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        } catch {
          return new Response('bad request', { status: 400 })
        }
      }
    }

    if (url.endsWith('/api/warpp/run')) {
      return new Response(JSON.stringify({ result: 'ok' }), {
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

  it('enables Run button and posts to /api/warpp/run when clicked', async () => {
    render(FlowView)

    // Wait for workflows to load and Run button to be present
    const runBtn = await screen.findByRole('button', { name: /Run/i })
    expect(runBtn).toBeTruthy()
    expect((runBtn as HTMLButtonElement).disabled).toBe(false)

    // Click Run
    await fireEvent.click(runBtn)

    // Expect that a POST to /api/warpp/run eventually occurs
    await waitFor(() => {
      // @ts-expect-error — fetch is stubbed by vitest
      const calls = (global.fetch as any).mock.calls as Array<[RequestInfo | URL, RequestInit | undefined]>
      expect(calls.some(([u, init]) => String(u).endsWith('/api/warpp/run') && (init?.method ?? 'GET') === 'POST')).toBe(true)
    })
  })
})
