import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({ trackD2SDownload: vi.fn() }))
const toastMocks = vi.hoisted(() => ({ error: vi.fn() }))

vi.mock('@/features/d2s/api', () => apiMocks)
vi.mock('sonner', () => ({ toast: toastMocks }))

import { Desktop2StereoHome } from '../desktop2stereo-home'

describe('Desktop2StereoHome download tracking', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  it('records an anonymous download click before opening the external release', async () => {
    const replace = vi.fn()
    const close = vi.fn()
    const popup = { opener: window, location: { replace }, close }
    vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window)
    apiMocks.trackD2SDownload.mockResolvedValue({
      success: true,
      data: { target: 'release', logged: true },
    })
    const user = userEvent.setup()

    render(<Desktop2StereoHome isAuthenticated={false} />)
    await user.click(screen.getByRole('button', { name: 'Download v2.5' }))

    await waitFor(() =>
      expect(apiMocks.trackD2SDownload).toHaveBeenCalledWith('release')
    )
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith(
        'https://github.com/lc700x/desktop2stereo/releases/tag/v2.5.0'
      )
    )
    expect(popup.opener).toBeNull()
    expect(close).not.toHaveBeenCalled()
  })

  it('does not open the external release when the click log cannot be recorded', async () => {
    const close = vi.fn()
    vi.spyOn(window, 'open').mockReturnValue({
      close,
      opener: null,
    } as unknown as Window)
    apiMocks.trackD2SDownload.mockRejectedValue(new Error('audit unavailable'))
    const user = userEvent.setup()

    render(<Desktop2StereoHome isAuthenticated={false} />)
    await user.click(screen.getByRole('button', { name: 'Download v2.5' }))

    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(toastMocks.error).toHaveBeenCalledOnce()
  })
})
