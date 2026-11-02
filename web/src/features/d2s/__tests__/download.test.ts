import { afterEach, describe, expect, it, vi } from 'vitest'

import { downloadBlob } from '../download'

describe('downloadBlob', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('clicks an attached download link and revokes the blob URL after a delay', () => {
    vi.useFakeTimers()
    const blobUrl = 'blob:http://localhost/access-log'
    const urlApi = {
      createObjectURL: vi.fn(() => blobUrl),
      revokeObjectURL: vi.fn(),
    }
    vi.stubGlobal('URL', urlApi)

    let wasAttachedWhenClicked = false
    let clickedAnchor: HTMLAnchorElement | undefined
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {
      clickedAnchor = document.body.querySelector('a') ?? undefined
      wasAttachedWhenClicked = clickedAnchor?.isConnected ?? false
    })

    downloadBlob(new Blob(['id,email\n1,a@example.com']), 'access_log.csv')

    expect(urlApi.createObjectURL).toHaveBeenCalledOnce()
    expect(clickedAnchor?.href).toBe(blobUrl)
    expect(clickedAnchor?.download).toBe('access_log.csv')
    expect(wasAttachedWhenClicked).toBe(true)
    expect(document.body.querySelector('a')).toBeNull()
    expect(urlApi.revokeObjectURL).not.toHaveBeenCalled()

    vi.advanceTimersByTime(999)
    expect(urlApi.revokeObjectURL).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    expect(urlApi.revokeObjectURL).toHaveBeenCalledTimes(1)
    expect(urlApi.revokeObjectURL).toHaveBeenCalledWith(blobUrl)
  })
})
