/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { RefreshCcwIcon, RocketIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'

import { SettingsSection } from '../components/settings-section'

type ProjectUpdateStatus = {
  repository: string
  branch: string
  enabled: boolean
  configured: boolean
  script: string
  version: string
  current_sha?: string
  runtime?: {
    state: 'idle' | 'running' | 'succeeded' | 'failed'
    phase: string
    message: string
    error?: string
    sha?: string
    updated_at?: string
  }
}

type ProjectUpdateCommit = {
  sha: string
  html_url?: string
  commit: {
    message: string
    author?: { date?: string }
  }
}

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

export function UpdateCheckerSection({
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [applying, setApplying] = useState(false)
  const [updateStatus, setUpdateStatus] = useState<ProjectUpdateStatus | null>(null)
  const [latestCommit, setLatestCommit] = useState<ProjectUpdateCommit | null>(null)
  const [displayVersion, setDisplayVersion] = useState(currentVersion || '')
  const [activeSHA, setActiveSHA] = useState<string | null>(null)

  useEffect(() => {
    setDisplayVersion(currentVersion || '')
  }, [currentVersion])

  useEffect(() => {
    if (!applying) return

    let stopped = false
    let finished = false
    const poll = async () => {
      try {
        const response = await api.get<{
          success: boolean
          data: ProjectUpdateStatus
        }>('/api/option/project-update/status')
        if (stopped) return
        const status = response.data.data
        setUpdateStatus(status)
        const runtime = status.runtime
        if (!runtime || runtime.state === 'running') return
        if (activeSHA && runtime.sha && runtime.sha.toLowerCase() !== activeSHA.toLowerCase()) {
          return
        }
        if (runtime.state === 'succeeded') {
          finished = true
          setApplying(false)
          setDisplayVersion(status.current_sha || runtime.sha || status.version)
          toast.success(t('Project update completed successfully'))
        } else if (runtime.state === 'failed') {
          finished = true
          setApplying(false)
          toast.error(runtime.error || runtime.message || t('Project update failed'))
        }
      } catch {
        // The application may be restarting. Keep polling until the status file reports a result.
      }
    }

    void poll()
    const timer = window.setInterval(() => {
      if (!finished) void poll()
    }, 2000)
    return () => {
      stopped = true
      window.clearInterval(timer)
    }
  }, [activeSHA, applying, t])

  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const version = displayVersion || updateStatus?.current_sha || t('Unknown')
  const updatePhases = [
    ['backup', t('Backup database')],
    ['download', t('Download project')],
    ['build', t('Build Docker image')],
    ['restart', t('Restart application')],
    ['health', t('Health check')],
  ] as const
  const activePhase = updateStatus?.runtime?.phase
  const activePhaseIndex = updatePhases.findIndex(([phase]) => phase === activePhase)

  const handleCheckUpdates = async () => {
    setChecking(true)
    try {
      const [statusResponse, commitResponse] = await Promise.all([
        api.get<{ success: boolean; data: ProjectUpdateStatus }>('/api/option/project-update/status'),
        api.post<{ success: boolean; data: { commit: ProjectUpdateCommit } }>('/api/option/project-update/check'),
      ])
      setUpdateStatus(statusResponse.data.data)
      setLatestCommit(commitResponse.data.data.commit)
      toast.success(t('Project update source checked successfully'))
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to check for updates')
      toast.error(message)
    } finally {
      setChecking(false)
    }
  }

  const handleApplyUpdate = async () => {
    if (!latestCommit) {
      toast.error(t('Check for updates first'))
      return
    }
    if (!updateStatus?.configured) {
      toast.error(t('Server update is not configured'))
      return
    }
    if (!window.confirm(t('Start the project update now? The service will restart after backup and health checks.'))) {
      return
    }
    setActiveSHA(latestCommit.sha)
    setApplying(true)
    try {
      const response = await api.post<{ success: boolean; message: string }>(
        '/api/option/project-update/apply',
        { sha: latestCommit.sha }
      )
      if (response.data.success) toast.success(response.data.message)
      else {
        setApplying(false)
        toast.error(response.data.message)
      }
    } catch (error) {
      setApplying(false)
      toast.error(error instanceof Error ? error.message : t('Failed to start project update'))
    }
  }

  return (
    <SettingsSection title={t('System maintenance')}>
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Current version')}
              </div>
              <div className='text-lg font-semibold'>{version}</div>
            </div>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Uptime since')}
              </div>
              <div className='text-lg font-semibold'>{uptime}</div>
            </div>
          </div>

          <Button onClick={handleCheckUpdates} disabled={checking}>
            {checking ? (
              t('Checking updates...')
            ) : (
              <>
                <RefreshCcwIcon className='me-2 h-4 w-4' />
                {t('Check for updates')}
              </>
            )}
          </Button>
          <div className='rounded-lg border p-4 text-sm'>
            <div className='font-semibold'>{t('Project update source')}</div>
            <div className='text-muted-foreground mt-2'>
              {updateStatus?.repository ?? 'laiyangli001/desktop2stereo-site'}:{updateStatus?.branch ?? 'main'}
            </div>
            <div className='text-muted-foreground mt-1'>
              {updateStatus?.configured
                ? t('Server-side update is ready. Database and shared files stay outside the release directory.')
                : t('Server-side update is disabled or not initialized. Install the fixed update script and enable D2S_UPDATE_ENABLED first.')}
            </div>
            {latestCommit && (
              <div className='mt-3 space-y-1'>
                <div className='font-mono text-xs'>{latestCommit.sha}</div>
                <div>{latestCommit.commit.message.split('\n')[0]}</div>
                <Button type='button' className='mt-2' onClick={handleApplyUpdate} disabled={applying || checking}>
                  <RocketIcon className='me-2 h-4 w-4' />
                  {applying ? t('Updating...') : t('Update server to this commit')}
                </Button>
              </div>
            )}
            {applying && updateStatus?.runtime && (
              <div className='mt-4 rounded-md bg-muted p-3'>
                <div className='font-medium'>{updateStatus.runtime.message}</div>
                <div className='text-muted-foreground mt-1 text-xs'>
                  {updateStatus.runtime.phase} · {t('Status is refreshed every 2 seconds')}
                </div>
                <div className='mt-3 grid gap-2 sm:grid-cols-5'>
                  {updatePhases.map(([phase, label], index) => (
                    <div
                      key={phase}
                      className={
                        index <= activePhaseIndex
                          ? 'rounded border border-primary bg-primary/10 px-2 py-1 text-xs'
                          : 'rounded border px-2 py-1 text-xs text-muted-foreground'
                      }
                    >
                      {label}
                    </div>
                  ))}
                </div>
              </div>
            )}
            {!applying && updateStatus?.runtime?.state === 'failed' && (
              <div className='mt-4 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm'>
                {updateStatus.runtime.error || updateStatus.runtime.message}
              </div>
            )}
          </div>
        </div>
    </SettingsSection>
  )
}
