import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Desktop2StereoHome } from '../desktop2stereo-home'

const apiMocks = vi.hoisted(() => ({ trackD2SDownload: vi.fn() }))
const toastMocks = vi.hoisted(() => ({ error: vi.fn() }))

vi.mock('@/features/d2s/api', () => apiMocks)
vi.mock('sonner', () => ({ toast: toastMocks }))

function renderHome() {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ tag_name: 'v2.6.0' }),
    })
  )
  const rootRoute = createRootRoute({ component: Outlet })
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: () => <Desktop2StereoHome isAuthenticated={false} />,
  })
  const licenseRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/business-license',
    component: () => <div>Business license</div>,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([homeRoute, licenseRoute]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })

  return render(<RouterProvider router={router} />)
}

describe('Desktop2StereoHome download tracking', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
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

    renderHome()
    expect(
      await screen.findByRole('heading', { name: 'Desktop2Stereo v2.6.0' })
    ).toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: 'Download' }))

    await waitFor(() =>
      expect(apiMocks.trackD2SDownload).toHaveBeenCalledWith('release')
    )
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith(
        'https://github.com/lc700x/desktop2stereo/releases/latest'
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

    renderHome()
    await user.click(await screen.findByRole('button', { name: 'Download' }))

    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(toastMocks.error).toHaveBeenCalledOnce()
  })
})
