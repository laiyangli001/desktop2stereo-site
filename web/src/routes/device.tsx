import { createFileRoute, redirect } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { approveD2SDevice } from '@/features/d2s/api'
import { resolveAuthentication } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/device')({
  beforeLoad: async () => {
    await resolveAuthentication()
    if (!useAuthStore.getState().auth.user) {
      throw redirect({ to: '/sign-in', search: { redirect: '/device' } })
    }
  },
  component: D2SDeviceApprovalPage,
})

function D2SDeviceApprovalPage() {
  const { t } = useTranslation()
  const [userCode, setUserCode] = useState('')
  const [approved, setApproved] = useState(false)
  const [pending, setPending] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalized = userCode.trim().toUpperCase()
    if (!normalized) return
    setPending(true)
    try {
      const result = await approveD2SDevice(normalized)
      if (!result.success) {
        toast.error(result.error?.message || t('Unable to approve device'))
        return
      }
      setApproved(true)
      toast.success(t('Device approved'))
    } catch {
      toast.error(t('Unable to approve device'))
    } finally {
      setPending(false)
    }
  }

  return (
    <main className='flex min-h-svh items-center justify-center px-4 py-8'>
      <Card className='w-full max-w-md'>
        <CardHeader>
          <CardTitle>{t('Approve Desktop2Stereo device')}</CardTitle>
          <CardDescription>
            {t('Enter the user code shown by the Desktop2Stereo launcher.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {approved ? (
            <p role='status' className='text-sm'>
              {t('Device approved. You can return to the launcher.')}
            </p>
          ) : (
            <form className='space-y-4' onSubmit={submit}>
              <label
                className='grid gap-2 text-sm font-medium'
                htmlFor='d2s-user-code'
              >
                {t('User code')}
                <Input
                  id='d2s-user-code'
                  autoComplete='one-time-code'
                  autoFocus
                  inputMode='text'
                  maxLength={16}
                  pattern='[A-Za-z0-9]{4}-[A-Za-z0-9]{4}'
                  placeholder='ABCD-EFGH'
                  required
                  value={userCode}
                  onChange={(event) => setUserCode(event.target.value)}
                />
              </label>
              <Button type='submit' disabled={pending} className='w-full'>
                {pending ? t('Approving…') : t('Approve device')}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  )
}
