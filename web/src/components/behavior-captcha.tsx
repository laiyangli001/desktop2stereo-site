/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

import { RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useRef, useState, type PointerEvent } from 'react'

import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'

export interface BehaviorCaptchaValue {
  captcha_id: string
  captcha_x: number
  captcha_y: number
}

interface CaptchaData {
  id: string
  master_image: string
  tile_image: string
  width: number
  height: number
  tile_width: number
  tile_height: number
  tile_start_y: number
}

interface BehaviorCaptchaProps {
  value?: BehaviorCaptchaValue
  onChange: (value?: BehaviorCaptchaValue) => void
}

export function BehaviorCaptcha({ value, onChange }: BehaviorCaptchaProps) {
  const [data, setData] = useState<CaptchaData>()
  const [tileX, setTileX] = useState(0)
  const [loading, setLoading] = useState(false)
  const [dragging, setDragging] = useState(false)
  const dragRef = useRef({ pointerX: 0, tileX: 0 })

  const refresh = useCallback(async () => {
    setLoading(true)
    onChange(undefined)
    try {
      const response = await api.get('/api/captcha')
      if (response.data?.success) {
        setData(response.data.data)
        setTileX(0)
      }
    } finally {
      setLoading(false)
    }
  }, [onChange])

  useEffect(() => {
    void refresh()
  }, [refresh])

  function handlePointerDown(event: PointerEvent<HTMLImageElement>) {
    if (!data || loading) return
    if (typeof event.currentTarget.setPointerCapture === 'function') {
      event.currentTarget.setPointerCapture(event.pointerId)
    }
    dragRef.current = { pointerX: event.clientX, tileX }
    setDragging(true)
  }

  function handlePointerMove(event: PointerEvent<HTMLImageElement>) {
    if (!dragging || !data) return
    const maxX = Math.max(0, data.width - data.tile_width)
    const nextX = Math.min(
      maxX,
      Math.max(0, dragRef.current.tileX + event.clientX - dragRef.current.pointerX)
    )
    dragRef.current.tileX = nextX
    setTileX(nextX)
  }

  function handlePointerUp() {
    if (!dragging || !data) return
    setDragging(false)
    onChange({ captcha_id: data.id, captcha_x: Math.round(dragRef.current.tileX), captcha_y: data.tile_start_y })
  }

  return (
    <div className='grid gap-2' aria-label='Drag verification'>
      <div className='text-muted-foreground flex items-center justify-between text-sm'>
        <span>拖动拼图完成验证</span>
        <Button type='button' variant='ghost' size='icon' onClick={() => void refresh()} disabled={loading}>
          <RefreshCw className={loading ? 'animate-spin' : ''} />
          <span className='sr-only'>刷新验证码</span>
        </Button>
      </div>
      {data ? (
        <div
          className='relative select-none overflow-hidden rounded-md border bg-muted'
          style={{ width: data.width, height: data.height, touchAction: 'none' }}
        >
          <img src={data.master_image} alt='验证码背景' className='absolute inset-0 h-full w-full' draggable={false} />
          <img
            src={data.tile_image}
            alt='可拖动拼图'
            className='absolute cursor-grab active:cursor-grabbing'
            style={{ left: tileX, top: data.tile_start_y, width: data.tile_width, height: data.tile_height }}
            draggable={false}
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            onPointerCancel={handlePointerUp}
          />
        </div>
      ) : (
        <div className='text-muted-foreground flex h-20 items-center justify-center text-sm'>验证码加载中…</div>
      )}
      {value ? <span className='text-xs text-green-600'>已完成拖动，请提交表单</span> : null}
    </div>
  )
}
