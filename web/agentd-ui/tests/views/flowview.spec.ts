import { render } from '@testing-library/vue'
import { describe, it, expect } from 'vitest'
import FlowView from '@/views/FlowView.vue'

// Smoke test to ensure component renders with VueFlow mounted

describe('FlowView', () => {
  it('renders without crashing', async () => {
    const { findByText } = render(FlowView)
    // expect one of node labels to appear
    expect(await findByText('Start')).toBeTruthy()
  })
})
