import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { D2SWorkspace } from '..'

const apiMocks = vi.hoisted(() => ({
  changeD2SMode: vi.fn(),
  createD2SManualUnbind: vi.fn(),
  createD2SOrder: vi.fn(),
  createD2SOrderCheckout: vi.fn(),
  createD2SWithdrawal: vi.fn(),
  freeRevokeD2SLicense: vi.fn(),
  getD2SBalance: vi.fn(),
  getD2SCheckoutProviders: vi.fn(),
  getD2SInvite: vi.fn(),
  getD2SLicenses: vi.fn(),
  getD2SManualUnbindRequests: vi.fn(),
  getD2SOrders: vi.fn(),
  getD2SWithdrawals: vi.fn(),
  previewD2SOrder: vi.fn(),
}))

vi.mock('../api', () => apiMocks)

describe('D2SWorkspace data status', () => {
  it('announces unavailable data instead of showing an empty account state', async () => {
    for (const query of Object.values(apiMocks)) {
      query.mockRejectedValue(new Error('D2S API unavailable'))
    }

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <D2SWorkspace />
      </QueryClientProvider>
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load Desktop2Stereo data'
    )
  })
})
