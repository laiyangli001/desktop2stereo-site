/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
the Free Software Foundation's terms.
*/

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BehaviorCaptcha, type BehaviorCaptchaValue } from '../behavior-captcha'

const apiMocks = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/lib/api', () => ({ api: apiMocks }))

const captcha = {
  id: 'captcha-id',
  master_image: 'data:image/jpeg;base64,master',
  thumb_image: 'data:image/png;base64,thumb',
  width: 320,
  height: 160,
  required_clicks: 2,
}

describe('BehaviorCaptcha', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockResolvedValue({ data: { success: true, data: captcha } })
  })

  it('loads a challenge and emits all coordinates after clicking the targets', async () => {
    const onChange = vi.fn<(value?: BehaviorCaptchaValue) => void>()
    render(<BehaviorCaptcha onChange={onChange} />)

    const imageButton = await screen.findByRole('button', {
      name: '点击图片中的目标，已完成 0/2',
    })
    vi.spyOn(imageButton, 'getBoundingClientRect').mockReturnValue({
      x: 10,
      y: 20,
      top: 20,
      left: 10,
      right: 330,
      bottom: 180,
      width: 320,
      height: 160,
      toJSON: () => ({}),
    })

    fireEvent.click(imageButton, { clientX: 110, clientY: 50 })
    fireEvent.click(imageButton, { clientX: 210, clientY: 100 })

    expect(onChange).toHaveBeenLastCalledWith({
      captcha_id: 'captcha-id',
      captcha_clicks: [
        { x: 100, y: 30 },
        { x: 200, y: 80 },
      ],
    })
  })

  it('clears the previous value and requests a new challenge when refreshed', async () => {
    const onChange = vi.fn<(value?: BehaviorCaptchaValue) => void>()
    render(
      <BehaviorCaptcha
        value={{ captcha_id: 'old', captcha_clicks: [{ x: 1, y: 2 }] }}
        onChange={onChange}
      />
    )

    await screen.findByRole('button', {
      name: '点击图片中的目标，已完成 0/2',
    })
    fireEvent.click(screen.getByRole('button', { name: '刷新验证码' }))

    await waitFor(() => expect(apiMocks.get).toHaveBeenCalledTimes(2))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })
})
