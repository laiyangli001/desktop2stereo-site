import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { PurchaseSettingsPanel } from '../integrations/purchase-settings-panel'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))
afterEach(() => {
  cleanup()
  vi.resetAllMocks()
})

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PurchaseSettingsPanel />
    </QueryClientProvider>
  )
}

it('saves the master switch immediately and keeps the confirmed state after a failed save', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { enabled: false } },
  })
  vi.mocked(api.put)
    .mockResolvedValueOnce({ data: { success: true, data: { enabled: true } } })
    .mockRejectedValueOnce(new Error('network failure'))
  const user = userEvent.setup()
  renderPanel()
  const toggle = await screen.findByRole('switch', {
    name: 'Desktop2Stereo purchase master switch',
  })
  await waitFor(() =>
    expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
  )
  expect(toggle).not.toBeChecked()
  await user.click(toggle)
  await waitFor(() => expect(toggle).toBeChecked())
  expect(api.put).toHaveBeenCalledWith('/api/option/d2s-purchases', {
    enabled: true,
  })
  await user.click(toggle)
  await waitFor(() =>
    expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
  )
  expect(toggle).toBeChecked()
})

it('keeps purchasing disabled when settings fail to load and supports retry', async () => {
  vi.mocked(api.get)
    .mockRejectedValueOnce(new Error('unavailable'))
    .mockResolvedValueOnce({
      data: { success: true, data: { enabled: false } },
    })
  const user = userEvent.setup()
  renderPanel()
  await screen.findByRole('alert')
  const toggle = screen.getByRole('switch', {
    name: 'Desktop2Stereo purchase master switch',
  })
  expect(toggle).toHaveAttribute('aria-disabled', 'true')
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
  )
  expect(toggle).not.toBeChecked()
})
