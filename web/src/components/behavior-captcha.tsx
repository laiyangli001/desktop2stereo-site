/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
the Free Software Foundation's terms.
*/

import { RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useState, type MouseEvent } from 'react'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

export interface BehaviorCaptchaClick {
  x: number
  y: number
}

export interface BehaviorCaptchaValue {
  captcha_id: string
  captcha_clicks: BehaviorCaptchaClick[]
}

interface CaptchaData {
  id: string
  master_image: string
  thumb_image: string
  width: number
  height: number
  required_clicks: number
}

interface BehaviorCaptchaProps {
  value?: BehaviorCaptchaValue
  onChange: (value?: BehaviorCaptchaValue) => void
}

export function BehaviorCaptcha({ value, onChange }: BehaviorCaptchaProps) {
  const [data, setData] = useState<CaptchaData>()
  const [clicks, setClicks] = useState<BehaviorCaptchaClick[]>([])
  const [loading, setLoading] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setClicks([])
    onChange(undefined)
    try {
      const response = await api.get('/api/captcha')
      if (response.data?.success) {
        setData(response.data.data)
      }
    } finally {
      setLoading(false)
    }
  }, [onChange])

  useEffect(() => {
    void refresh()
  }, [refresh])

  function handleImageClick(event: MouseEvent<HTMLButtonElement>) {
    if (!data || loading || clicks.length >= data.required_clicks) return

    const bounds = event.currentTarget.getBoundingClientRect()
    const scaleX = data.width / bounds.width
    const scaleY = data.height / bounds.height
    const nextClick = {
      x: Math.max(
        0,
        Math.min(data.width, Math.round((event.clientX - bounds.left) * scaleX))
      ),
      y: Math.max(
        0,
        Math.min(data.height, Math.round((event.clientY - bounds.top) * scaleY))
      ),
    }
    const nextClicks = [...clicks, nextClick]
    setClicks(nextClicks)

    if (nextClicks.length === data.required_clicks) {
      onChange({ captcha_id: data.id, captcha_clicks: nextClicks })
    }
  }

  return (
    <div className='grid gap-2' aria-label='Click verification'>
      <div className='text-muted-foreground flex items-center justify-between text-sm'>
        <span>请按下方提示点击图片中的目标</span>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          onClick={() => void refresh()}
          disabled={loading}
          aria-label='刷新验证码'
        >
          <RefreshCw
            className={loading ? 'animate-spin' : ''}
            aria-hidden='true'
          />
          <span className='sr-only'>刷新验证码</span>
        </Button>
      </div>
      {data ? (
        <>
          <div className='flex items-center gap-2 text-sm'>
            <span className='text-muted-foreground'>请点击：</span>
            <img
              src={data.thumb_image}
              alt='需要点击的目标'
              className='h-10 w-auto rounded border'
            />
            <span className='text-muted-foreground'>
              ({clicks.length}/{data.required_clicks})
            </span>
          </div>
          <button
            type='button'
            className='bg-muted focus-visible:ring-ring relative block overflow-hidden rounded-md border p-0 text-left focus-visible:ring-2 focus-visible:outline-none disabled:cursor-default'
            style={{ width: data.width, height: data.height, maxWidth: '100%' }}
            onClick={handleImageClick}
            disabled={loading || clicks.length >= data.required_clicks}
            aria-label={`点击图片中的目标，已完成 ${clicks.length}/${data.required_clicks}`}
          >
            <img
              src={data.master_image}
              alt='点击验证码图片'
              className='pointer-events-none h-full w-full object-fill select-none'
              draggable={false}
            />
            {clicks.map((point, index) => (
              <span
                key={`${point.x}-${point.y}-${index}`}
                className='pointer-events-none absolute flex h-6 w-6 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border-2 border-white bg-blue-600/80 text-xs font-semibold text-white shadow'
                style={{
                  left: `${(point.x / data.width) * 100}%`,
                  top: `${(point.y / data.height) * 100}%`,
                }}
                aria-hidden='true'
              >
                {index + 1}
              </span>
            ))}
          </button>
        </>
      ) : (
        <div className='text-muted-foreground flex h-20 items-center justify-center text-sm'>
          验证码加载中…
        </div>
      )}
      {value ? (
        <span className='text-xs text-green-600'>已完成点击，请提交表单</span>
      ) : null}
    </div>
  )
}
