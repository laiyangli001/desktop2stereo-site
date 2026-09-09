import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UpdateCheckerSection } from '../update-checker-section'

const apiMocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/lib/api', () => ({ api: apiMocks }))

const oldSHA = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
const newSHA = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'

function axiosResponse<T>(data: T) {
  return { data: { success: true, data } }
}

function status(runtime: Record<string, string>, currentSHA = oldSHA) {
  return {
    repository: 'laiyangli001/desktop2stereo-site',
    branch: 'main',
    enabled: true,
    configured: true,
    script: '/usr/local/sbin/desktop2stereo-docker-update',
    version: oldSHA,
    current_sha: currentSHA,
    runtime,
  }
}

function commit() {
  return {
    sha: newSHA,
    commit: { message: 'fix: test project updater' },
  }
}

function renderChecker() {
  return render(<UpdateCheckerSection currentVersion={oldSHA} />)
}

describe('UpdateCheckerSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMocks.post.mockImplementation((path: string) => {
      if (path.endsWith('/check'))
        return Promise.resolve(axiosResponse({ commit: commit() }))
      return Promise.resolve({ data: { success: true, message: 'queued' } })
    })
  })

  it('disables the update button while polling and refreshes the version after success', async () => {
    apiMocks.get
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'idle', phase: 'completed', message: '' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'idle', phase: 'completed', message: '' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'running', phase: 'backup', message: 'Backing up' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status(
            { state: 'succeeded', phase: 'completed', message: 'Done' },
            newSHA
          )
        )
      )

    renderChecker()
    fireEvent.click(screen.getByRole('button', { name: 'Check for updates' }))
    const updateButton = await screen.findByRole('button', {
      name: 'Update server to this commit',
    })
    fireEvent.click(updateButton)

    expect(
      await screen.findByRole('button', { name: 'Updating...' })
    ).toBeDisabled()
    await waitFor(
      () =>
        expect(
          screen.getByText(
            'The server is already running this commit. No update is needed.'
          )
        ).toBeInTheDocument(),
      { timeout: 5000 }
    )
    expect(
      screen.queryByRole('button', { name: 'Updating...' })
    ).not.toBeInTheDocument()
  })

  it('shows the failure and restores the update button after polling', async () => {
    apiMocks.get
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'idle', phase: 'completed', message: '' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'idle', phase: 'completed', message: '' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status({ state: 'running', phase: 'build', message: 'Building' })
        )
      )
      .mockResolvedValueOnce(
        axiosResponse(
          status({
            state: 'failed',
            phase: 'rollback',
            message: 'Update failed',
            error: 'health check failed',
          })
        )
      )

    renderChecker()
    fireEvent.click(screen.getByRole('button', { name: 'Check for updates' }))
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Update server to this commit',
      })
    )

    expect(
      await screen.findByText('health check failed', {}, { timeout: 5000 })
    ).toBeInTheDocument()
    expect(
      await screen.findByRole('button', {
        name: 'Update server to this commit',
      })
    ).toBeEnabled()
  })

  it('restores an in-progress update when returning to the maintenance page', async () => {
    apiMocks.get.mockResolvedValue(
      axiosResponse(
        status({
          state: 'running',
          phase: 'build',
          message: 'Building',
          sha: newSHA,
        })
      )
    )

    renderChecker()

    expect(
      await screen.findByRole('button', { name: 'Updating...' })
    ).toBeDisabled()
    expect(await screen.findByText('Building')).toBeInTheDocument()
    expect(await screen.findByText('Build Docker image')).toBeInTheDocument()
  })
})
