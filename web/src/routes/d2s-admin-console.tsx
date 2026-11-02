/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute } from '@tanstack/react-router'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from 'react'

import { SkipToMain } from '@/components/skip-to-main'

import { downloadBlob } from '../features/d2s/download'

const MAX_AUDIT_RETENTION_DAYS = 2_147_483_647

type AdminSession = { id: number; email: string; role: string }
type ApiEnvelope<T> = { success: boolean; data: T; code?: string }
type RecordRow = Record<string, unknown>

async function adminRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    cache: 'no-store',
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  const payload = (await response.json().catch(() => ({}))) as ApiEnvelope<T>
  if (!response.ok || payload.success === false) {
    throw new Error(payload.code || `HTTP_${response.status}`)
  }
  return payload.data
}

function stringifyCell(value: unknown) {
  if (value === null || value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

type AdminSection =
  | 'logs'
  | 'registrations'
  | 'purchases'
  | 'accounts'
  | 'identity'
  | 'retention'

function defaultAdminSection(role: string): AdminSection | null {
  if (role === 'super_admin' || role === 'audit_reader') return 'logs'
  if (role === 'account_operator') return 'accounts'
  if (role === 'identity_reviewer') return 'identity'
  return null
}

const adminErrorMessages: Record<string, string> = {
  RETENTION_REJECTED: '保留天数不能低于 180 天。',
  RETENTION_UNAVAILABLE:
    '独立审计清理数据库未配置或当前不可用，请联系数据库管理员。',
  RETENTION_FAILED:
    '清理操作失败，未确认清理结果；请检查后台审计记录和数据库状态。',
}

export function D2SAdminConsole() {
  const [admin, setAdmin] = useState<AdminSession | null>(null)
  const [checking, setChecking] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [section, setSection] = useState<AdminSection | null>(null)
  const [filters, setFilters] = useState({
    start_time: '',
    end_time: '',
    user_id: '',
    email: '',
    source_ip: '',
    action_type: '',
  })
  const [rows, setRows] = useState<RecordRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [registrations, setRegistrations] = useState<RecordRow[]>([])
  const [purchases, setPurchases] = useState<RecordRow[]>([])
  const [userID, setUserID] = useState('')
  const [loginBlocked, setLoginBlocked] = useState(false)
  const [purchaseBlocked, setPurchaseBlocked] = useState(false)
  const [forceLogout, setForceLogout] = useState(false)
  const [reason, setReason] = useState('')
  const [batchCIDR, setBatchCIDR] = useState('')
  const [batchDomain, setBatchDomain] = useState('')
  const [batchFrom, setBatchFrom] = useState('')
  const [batchUntil, setBatchUntil] = useState('')
  const [batchPreview, setBatchPreview] = useState<{
    preview_id: string
    match_count: number
  } | null>(null)
  const [identity, setIdentity] = useState<RecordRow | null>(null)
  const [identityType, setIdentityType] = useState('PASSPORT')
  const [identityNumber, setIdentityNumber] = useState('')
  const [identityCountry, setIdentityCountry] = useState('')
  const [identityStatus, setIdentityStatus] = useState('PENDING')
  const [retentionDays, setRetentionDays] = useState('180')

  const isAllowed = useCallback(
    (...roles: string[]) =>
      admin?.role === 'super_admin' || roles.includes(admin?.role || ''),
    [admin]
  )

  useEffect(() => {
    let active = true
    adminRequest<{ admin: AdminSession }>('/api/v1/admin/auth/session')
      .then((data) => {
        if (active) {
          setAdmin(data.admin)
          setSection(defaultAdminSection(data.admin.role))
        }
      })
      .catch(() => {
        if (active) setAdmin(null)
      })
      .finally(() => {
        if (active) setChecking(false)
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [])

  const run = useCallback(async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (err) {
      const code = err instanceof Error ? err.message : 'REQUEST_FAILED'
      setError(adminErrorMessages[code] ?? code)
    } finally {
      setBusy(false)
    }
  }, [])

  const login = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    void run(async () => {
      const data = await adminRequest<{ admin: AdminSession }>(
        '/api/v1/admin/auth/login',
        {
          method: 'POST',
          body: JSON.stringify({ email, password }),
        }
      )
      setAdmin(data.admin)
      setSection(defaultAdminSection(data.admin.role))
      setPassword('')
    })
  }

  const logout = () =>
    run(async () => {
      await adminRequest('/api/v1/admin/auth/logout', { method: 'POST' })
      setAdmin(null)
      setRows([])
    })

  const queryString = useMemo(() => {
    const query = new URLSearchParams({ page: String(page), page_size: '50' })
    Object.entries(filters).forEach(([key, value]) => {
      if (value.trim()) query.set(key, value.trim())
    })
    return query.toString()
  }, [filters, page])

  const loadLogs = (targetPage = page) =>
    run(async () => {
      const query = new URLSearchParams(queryString)
      query.set('page', String(targetPage))
      const data = await adminRequest<{ items: RecordRow[]; total: number }>(
        `/api/v1/admin/audit/user-events?${query}`
      )
      setRows(data.items)
      setTotal(data.total)
      setPage(targetPage)
    })

  const exportLogs = () =>
    run(async () => {
      const query = new URLSearchParams()
      Object.entries(filters).forEach(([key, value]) => {
        if (value.trim()) query.set(key, value.trim())
      })
      const response = await fetch(
        `/api/v1/admin/audit/user-events/export.csv?${query}`,
        { credentials: 'include', cache: 'no-store' }
      )
      if (!response.ok) throw new Error('LOG_EXPORT_FAILED')
      const disposition = response.headers.get('Content-Disposition') || ''
      const filename =
        disposition.match(/filename="?([^";]+)"?/i)?.[1] || 'access_log.csv'
      downloadBlob(await response.blob(), filename)
    })

  const loadRegistrations = (targetPage = page) =>
    run(async () => {
      const query = new URLSearchParams({
        page: String(targetPage),
        page_size: '50',
      })
      if (filters.start_time) query.set('start_time', filters.start_time)
      if (filters.end_time) query.set('end_time', filters.end_time)
      const data = await adminRequest<{
        items: RecordRow[]
        total: number
      }>(`/api/v1/admin/audit/registrations?${query}`)
      setRegistrations(data.items)
      setTotal(data.total)
      setPage(targetPage)
    })

  const exportRegistrations = () =>
    run(async () => {
      const query = new URLSearchParams()
      if (filters.start_time) query.set('start_time', filters.start_time)
      if (filters.end_time) query.set('end_time', filters.end_time)
      const response = await fetch(
        `/api/v1/admin/audit/registrations/export.csv?${query}`,
        { credentials: 'include', cache: 'no-store' }
      )
      if (!response.ok) throw new Error('REGISTRATION_EXPORT_FAILED')
      const filename =
        response.headers
          .get('Content-Disposition')
          ?.match(/filename="?([^";]+)"?/i)?.[1] || 'registration_records.csv'
      downloadBlob(await response.blob(), filename)
    })

  const loadPurchases = (targetPage = page) =>
    run(async () => {
      const query = new URLSearchParams({
        page: String(targetPage),
        page_size: '50',
      })
      const data = await adminRequest<{
        items: RecordRow[]
        total: number
      }>(`/api/v1/admin/purchases?${query}`)
      setPurchases(data.items)
      setTotal(data.total)
      setPage(targetPage)
    })

  const goToPage = (targetPage: number) => {
    if (targetPage < 1 || targetPage > Math.ceil(total / 50)) return
    if (section === 'logs') void loadLogs(targetPage)
    if (section === 'registrations') void loadRegistrations(targetPage)
    if (section === 'purchases') void loadPurchases(targetPage)
  }

  const setEmergency = () =>
    run(async () => {
      if (!userID || !window.confirm(`确认对账户 ${userID} 执行紧急限制？`)) {
        return
      }
      await adminRequest(
        `/api/v1/admin/users/${encodeURIComponent(userID)}/emergency`,
        {
          method: 'PUT',
          body: JSON.stringify({
            login_blocked: loginBlocked,
            purchase_blocked: purchaseBlocked,
            force_logout: forceLogout,
            reason,
          }),
        }
      )
      setError('紧急操作已记录')
    })

  const previewBatch = () => {
    setBatchPreview(null)
    return run(async () => {
      const data = await adminRequest<{
        preview_id: string
        match_count: number
      }>('/api/v1/admin/users/batch/preview', {
        method: 'POST',
        body: JSON.stringify({
          selector: {
            ip_cidr: batchCIDR,
            email_domain: batchDomain,
            register_from: batchFrom ? Number(batchFrom) : 0,
            register_until: batchUntil ? Number(batchUntil) : 0,
          },
          actions: {
            login_blocked: loginBlocked,
            purchase_blocked: purchaseBlocked,
            reason,
          },
        }),
      })
      setBatchPreview(data)
    })
  }

  const applyBatch = () =>
    run(async () => {
      if (
        !batchPreview ||
        !window.confirm(
          `将对 ${batchPreview.match_count} 个账户应用限制，确认继续？`
        )
      ) {
        return
      }
      await adminRequest('/api/v1/admin/users/batch/apply', {
        method: 'POST',
        body: JSON.stringify({
          preview_id: batchPreview.preview_id,
          confirmed: true,
          confirmation_count: batchPreview.match_count,
        }),
      })
      setBatchPreview(null)
      setError('批量操作已记录')
    })

  const loadIdentity = () =>
    run(async () => {
      if (!userID) throw new Error('请输入账户 ID')
      const data = await adminRequest<RecordRow>(
        `/api/v1/admin/users/${encodeURIComponent(userID)}/identity`
      )
      setIdentity(data)
      setIdentityType(stringifyCell(data.id_document_type) || 'PASSPORT')
      setIdentityCountry(stringifyCell(data.id_document_country))
      setIdentityStatus(
        stringifyCell(data.id_verification_status) || 'NOT_REQUIRED'
      )
    })

  const saveIdentity = () =>
    run(async () => {
      if (!userID) throw new Error('请输入账户 ID')
      const data = await adminRequest<RecordRow>(
        `/api/v1/admin/users/${encodeURIComponent(userID)}/identity`,
        {
          method: 'PUT',
          body: JSON.stringify({
            id_document_type: identityType,
            id_document_number: identityNumber,
            id_document_country: identityCountry,
            id_verification_status: identityStatus,
          }),
        }
      )
      setIdentity(data)
      setIdentityNumber('')
    })

  const runRetention = () =>
    run(async () => {
      if (
        !window.confirm(
          `确认清理早于 ${retentionDays} 天的数据？此操作不可撤回。`
        )
      ) {
        return
      }
      await adminRequest('/api/v1/admin/audit/retention/execute', {
        method: 'POST',
        body: JSON.stringify({ retention_days: Number(retentionDays) }),
      })
      setError('保留期清理已完成并写入审计')
    })

  if (checking) {
    return (
      <>
        <SkipToMain />
        <main id='content' tabIndex={-1} className='mx-auto max-w-4xl p-8'>
          <p aria-live='polite' role='status'>
            正在检查后台会话…
          </p>
        </main>
      </>
    )
  }

  if (!admin) {
    return (
      <>
        <SkipToMain />
        <main id='content' tabIndex={-1} className='mx-auto max-w-md p-8'>
          <form className='space-y-5' onSubmit={login}>
            <h1 className='text-2xl font-semibold'>Desktop2Stereo 后台登录</h1>
            <p className='text-muted-foreground text-sm'>
              使用独立后台管理员账号登录。
            </p>
            <label className='block space-y-1'>
              <span>管理员邮箱</span>
              <input
                autoComplete='username'
                className='w-full rounded border p-2'
                name='email'
                spellCheck={false}
                onChange={(event) => setEmail(event.target.value)}
                required
                type='email'
                value={email}
              />
            </label>
            <label className='block space-y-1'>
              <span>密码</span>
              <input
                autoComplete='current-password'
                className='w-full rounded border p-2'
                name='password'
                onChange={(event) => setPassword(event.target.value)}
                required
                type='password'
                value={password}
              />
            </label>
            <button
              className='bg-primary text-primary-foreground hover:bg-primary/90 focus-visible:ring-ring rounded px-4 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:opacity-50'
              disabled={busy}
              type='submit'
            >
              {busy ? '正在登录…' : '登录'}
            </button>
            {error && (
              <p aria-live='polite' className='text-destructive text-sm'>
                {error}
              </p>
            )}
          </form>
        </main>
      </>
    )
  }

  const logColumns = [
    'id',
    'user_id',
    'email',
    'action_type',
    'event_time',
    'source_ip',
    'source_port',
    'destination_ip',
    'destination_port',
    'user_agent',
    'device_fingerprint',
    'request_path',
    'result_status',
    'record_hash',
  ]

  return (
    <>
      <SkipToMain />
      <main
        id='content'
        tabIndex={-1}
        className='mx-auto max-w-7xl space-y-6 p-6'
      >
        <header className='flex flex-wrap items-center justify-between gap-3'>
          <div>
            <h1 className='text-2xl font-semibold'>账户审计与治理</h1>
            <p className='text-muted-foreground text-sm'>
              {admin.email} · {admin.role}
            </p>
          </div>
          <button
            className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
            disabled={busy}
            onClick={logout}
            type='button'
          >
            退出后台
          </button>
        </header>
        <nav aria-label='后台功能' className='flex flex-wrap gap-2'>
          {[
            ['logs', '行为日志', 'audit_reader'],
            ['registrations', '注册记录', 'audit_reader'],
            ['purchases', '购买记录', 'account_operator'],
            ['accounts', '紧急限制', 'account_operator'],
            ['identity', '身份核验', 'identity_reviewer'],
            ['retention', '日志留存', 'super_admin'],
          ]
            .filter(([, , role]) => isAllowed(role as string))
            .map(([key, label]) => (
              <button
                aria-pressed={section === key}
                className={
                  section === key
                    ? 'bg-primary text-primary-foreground hover:bg-primary/90 focus-visible:ring-ring rounded px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                    : 'hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                }
                key={key}
                onClick={() => {
                  setSection(key as AdminSection)
                  setPage(1)
                  setTotal(0)
                  setRows([])
                  setRegistrations([])
                  setPurchases([])
                }}
                type='button'
              >
                {label}
              </button>
            ))}
        </nav>
        {error && (
          <p aria-live='polite' className='rounded border p-3 text-sm'>
            {error}
          </p>
        )}

        {section === null && (
          <p
            aria-live='polite'
            className='rounded border p-3 text-sm'
            role='status'
          >
            当前管理员角色没有可访问的后台模块。
          </p>
        )}

        {(section === 'logs' || section === 'registrations') && (
          <section className='space-y-4 rounded border p-4'>
            <h2 className='text-lg font-medium'>
              {section === 'logs' ? '用户行为日志' : '注册信息'}
            </h2>
            <div className='grid gap-3 md:grid-cols-3'>
              {[
                ['start_time', '开始时间（Unix 秒或 RFC3339）'],
                ['end_time', '结束时间（Unix 秒或 RFC3339）'],
                ...(section === 'logs'
                  ? [
                      ['user_id', '用户 ID'],
                      ['email', '邮箱'],
                      ['source_ip', '来源 IP'],
                      ['action_type', '行为类型'],
                    ]
                  : []),
              ].map(([key, label]) => (
                <label className='space-y-1 text-sm' key={key}>
                  <span>{label}</span>
                  <input
                    autoComplete='off'
                    className='w-full rounded border p-2'
                    name={key}
                    spellCheck={key !== 'email'}
                    type={key === 'email' ? 'email' : 'text'}
                    onChange={(event) => {
                      setPage(1)
                      setTotal(0)
                      setRows([])
                      setRegistrations([])
                      setFilters((current) => ({
                        ...current,
                        [key]: event.target.value,
                      }))
                    }}
                    value={filters[key as keyof typeof filters]}
                  />
                </label>
              ))}
            </div>
            <div className='flex flex-wrap gap-2'>
              <button
                className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                disabled={busy}
                onClick={() =>
                  section === 'logs' ? loadLogs() : loadRegistrations()
                }
                type='button'
              >
                查询
              </button>
              <button
                className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                disabled={busy}
                onClick={section === 'logs' ? exportLogs : exportRegistrations}
                type='button'
              >
                导出 CSV
              </button>
            </div>
            {section === 'logs' ? (
              <>
                <div
                  className='focus-visible:ring-ring overflow-auto rounded focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                  role='region'
                  aria-label='可横向滚动的用户行为日志'
                  tabIndex={0}
                >
                  <table
                    aria-label='用户行为日志结果'
                    className='min-w-full border-collapse text-left text-xs'
                  >
                    <thead>
                      <tr>
                        {logColumns.map((column) => (
                          <th className='border p-2' key={column} scope='col'>
                            {column}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {rows.length === 0 && (
                        <tr>
                          <td
                            className='border p-3 text-center'
                            colSpan={logColumns.length}
                          >
                            <span aria-live='polite' role='status'>
                              尚无日志记录，请调整筛选条件或执行查询。
                            </span>
                          </td>
                        </tr>
                      )}
                      {rows.map((row) => (
                        <tr key={stringifyCell(row.id)}>
                          {logColumns.map((column) => (
                            <td
                              className='max-w-64 border p-2 break-all'
                              key={column}
                            >
                              {stringifyCell(row[column])}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className='flex flex-wrap items-center gap-3 text-sm'>
                  <PaginationControls
                    busy={busy}
                    page={page}
                    total={total}
                    onPageChange={goToPage}
                  />
                  <button
                    className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-1 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                    disabled={!rows.length || busy}
                    onClick={() =>
                      run(async () => {
                        const id = stringifyCell(rows[0].id)
                        const data = await adminRequest<{
                          consistent: boolean
                        }>(
                          `/api/v1/admin/audit/user-events/${encodeURIComponent(id)}/verify-hash`,
                          { method: 'POST' }
                        )
                        setError(`日志 ${id} 哈希一致：${data.consistent}`)
                      })
                    }
                    type='button'
                  >
                    校验本页首条哈希
                  </button>
                </div>
              </>
            ) : (
              <>
                <RecordTable rows={registrations} />
                <PaginationControls
                  busy={busy}
                  page={page}
                  total={total}
                  onPageChange={goToPage}
                />
              </>
            )}
          </section>
        )}

        {section === 'purchases' && (
          <section className='space-y-4 rounded border p-4'>
            <h2 className='text-lg font-medium'>购买记录</h2>
            <button
              className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
              disabled={busy}
              onClick={() => loadPurchases()}
              type='button'
            >
              刷新
            </button>
            <RecordTable rows={purchases} />
            <PaginationControls
              busy={busy}
              page={page}
              total={total}
              onPageChange={goToPage}
            />
          </section>
        )}

        {section === 'accounts' && (
          <section className='space-y-5 rounded border p-4'>
            <h2 className='text-lg font-medium'>账户紧急限制</h2>
            <div className='flex flex-wrap gap-3'>
              <input
                aria-label='账户 ID'
                autoComplete='off'
                className='rounded border p-2'
                inputMode='numeric'
                name='user_id'
                onChange={(event) => setUserID(event.target.value)}
                placeholder='账户 ID'
                value={userID}
              />
              <input
                aria-label='处置原因'
                autoComplete='off'
                className='min-w-64 rounded border p-2'
                name='reason'
                disabled={busy}
                onChange={(event) => {
                  setBatchPreview(null)
                  setReason(event.target.value)
                }}
                placeholder='处置原因'
                value={reason}
              />
            </div>
            <div className='flex flex-wrap gap-4 text-sm'>
              <label>
                <input
                  name='login_blocked'
                  checked={loginBlocked}
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setLoginBlocked(event.target.checked)
                  }}
                  type='checkbox'
                />{' '}
                禁止登录
              </label>
              <label>
                <input
                  name='purchase_blocked'
                  checked={purchaseBlocked}
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setPurchaseBlocked(event.target.checked)
                  }}
                  type='checkbox'
                />{' '}
                禁止购买
              </label>
              <label>
                <input
                  name='force_logout'
                  checked={forceLogout}
                  onChange={(event) => setForceLogout(event.target.checked)}
                  type='checkbox'
                />{' '}
                强制下线
              </label>
            </div>
            <button
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90 focus-visible:ring-ring rounded px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
              disabled={busy || !userID}
              onClick={setEmergency}
              type='button'
            >
              应用账户限制
            </button>
            <hr />
            <h3 className='font-medium'>批量处置</h3>
            <div className='grid gap-3 md:grid-cols-2'>
              <label className='grid gap-1 text-sm'>
                <span>注册 IP CIDR</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  name='ip_cidr'
                  spellCheck={false}
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setBatchCIDR(event.target.value)
                  }}
                  value={batchCIDR}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>邮箱域名</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  name='email_domain'
                  spellCheck={false}
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setBatchDomain(event.target.value)
                  }}
                  value={batchDomain}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>注册开始时间（Unix 秒）</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  inputMode='numeric'
                  name='register_from'
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setBatchFrom(event.target.value)
                  }}
                  value={batchFrom}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>注册结束时间（Unix 秒）</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  inputMode='numeric'
                  name='register_until'
                  disabled={busy}
                  onChange={(event) => {
                    setBatchPreview(null)
                    setBatchUntil(event.target.value)
                  }}
                  value={batchUntil}
                />
              </label>
            </div>
            <div className='flex flex-wrap gap-2'>
              <button
                className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                disabled={busy}
                onClick={previewBatch}
                type='button'
              >
                预览影响账户
              </button>
              {batchPreview && (
                <>
                  <p aria-live='polite' className='text-sm' role='status'>
                    预览匹配 {batchPreview.match_count}{' '}
                    个账户；请核对筛选条件后确认。
                  </p>
                  <button
                    className='bg-destructive text-destructive-foreground hover:bg-destructive/90 focus-visible:ring-ring rounded px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                    disabled={busy}
                    onClick={applyBatch}
                    type='button'
                  >
                    确认处置 {batchPreview.match_count} 个账户
                  </button>
                </>
              )}
            </div>
          </section>
        )}

        {section === 'identity' && (
          <section className='space-y-4 rounded border p-4'>
            <h2 className='text-lg font-medium'>触发式身份核验</h2>
            <div className='flex flex-wrap gap-2'>
              <input
                aria-label='账户 ID'
                autoComplete='off'
                className='rounded border p-2'
                inputMode='numeric'
                name='user_id'
                onChange={(event) => setUserID(event.target.value)}
                placeholder='账户 ID'
                value={userID}
              />
              <button
                className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                disabled={busy}
                onClick={loadIdentity}
                type='button'
              >
                查询状态
              </button>
            </div>
            {identity && (
              <pre className='bg-muted overflow-auto rounded p-3 text-xs'>
                {JSON.stringify(identity, null, 2)}
              </pre>
            )}
            <div className='grid gap-3 md:grid-cols-2'>
              <label className='grid gap-1 text-sm'>
                <span>证件类型</span>
                <select
                  autoComplete='off'
                  className='rounded border p-2'
                  name='id_document_type'
                  onChange={(event) => setIdentityType(event.target.value)}
                  value={identityType}
                >
                  <option value='PASSPORT'>Passport</option>
                  <option value='FOREIGN_PERMANENT_RESIDENT_ID'>
                    Foreign permanent resident ID
                  </option>
                  <option value='OTHER'>Other</option>
                </select>
              </label>
              <label className='grid gap-1 text-sm'>
                <span>签发国/地区代码</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  name='id_document_country'
                  spellCheck={false}
                  onChange={(event) => setIdentityCountry(event.target.value)}
                  value={identityCountry}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>证件号码（仅提交哈希）</span>
                <input
                  autoComplete='off'
                  className='rounded border p-2'
                  name='id_document_number'
                  spellCheck={false}
                  onChange={(event) => setIdentityNumber(event.target.value)}
                  value={identityNumber}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>核验状态</span>
                <select
                  autoComplete='off'
                  className='rounded border p-2'
                  name='id_verification_status'
                  onChange={(event) => setIdentityStatus(event.target.value)}
                  value={identityStatus}
                >
                  <option value='NOT_REQUIRED'>NOT_REQUIRED</option>
                  <option value='PENDING'>PENDING</option>
                  <option value='VERIFIED'>VERIFIED</option>
                  <option value='REJECTED'>REJECTED</option>
                </select>
              </label>
            </div>
            <button
              className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
              disabled={busy || !userID}
              onClick={saveIdentity}
              type='button'
            >
              保存核验结果
            </button>
          </section>
        )}

        {section === 'retention' && (
          <section className='space-y-4 rounded border p-4'>
            <h2 className='text-lg font-medium'>受控日志清理</h2>
            <p className='text-muted-foreground text-sm'>
              清理期必须在 180 至 {MAX_AUDIT_RETENTION_DAYS}{' '}
              天之间。执行前会记录清理范围与数量。
            </p>
            <div className='flex flex-wrap items-end gap-2'>
              <label className='grid gap-1 text-sm'>
                <span>保留天数（最少 180 天）</span>
                <input
                  autoComplete='off'
                  className='w-40 rounded border p-2'
                  name='retention_days'
                  max={MAX_AUDIT_RETENTION_DAYS}
                  min='180'
                  onChange={(event) => setRetentionDays(event.target.value)}
                  step='1'
                  type='number'
                  value={retentionDays}
                />
              </label>
              <button
                className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                disabled={
                  busy ||
                  !Number.isSafeInteger(Number(retentionDays)) ||
                  Number(retentionDays) < 180 ||
                  Number(retentionDays) > MAX_AUDIT_RETENTION_DAYS
                }
                onClick={runRetention}
                type='button'
              >
                执行受控清理
              </button>
            </div>
          </section>
        )}
      </main>
    </>
  )
}

function PaginationControls(props: {
  page: number
  total: number
  busy: boolean
  onPageChange: (page: number) => void
}) {
  const pageCount = Math.ceil(props.total / 50)
  return (
    <nav aria-label='记录分页' className='flex items-center gap-3 text-sm'>
      <button
        className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-1 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
        disabled={props.page <= 1 || props.busy}
        onClick={() => props.onPageChange(props.page - 1)}
        type='button'
      >
        上一页
      </button>
      <span aria-live='polite' role='status'>
        第 {props.page} 页 · 共 {props.total} 条
      </span>
      <button
        className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-1 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
        disabled={props.page >= pageCount || props.busy}
        onClick={() => props.onPageChange(props.page + 1)}
        type='button'
      >
        下一页
      </button>
    </nav>
  )
}

function RecordTable({ rows }: { rows: RecordRow[] }) {
  const columns = rows.length ? Object.keys(rows[0]) : []
  if (rows.length === 0) {
    return (
      <p
        aria-live='polite'
        className='text-muted-foreground p-3 text-sm'
        role='status'
      >
        暂无记录。
      </p>
    )
  }
  return (
    <div
      className='focus-visible:ring-ring overflow-auto rounded focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
      role='region'
      aria-label='可横向滚动的记录结果'
      tabIndex={0}
    >
      <table
        aria-label='记录结果'
        className='min-w-full border-collapse text-left text-xs'
      >
        <thead>
          <tr>
            {columns.map((column) => (
              <th className='border p-2' key={column} scope='col'>
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={stringifyCell(row.id) || index}>
              {columns.map((column) => (
                <td className='max-w-64 border p-2 break-all' key={column}>
                  {stringifyCell(row[column])}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export const Route = createFileRoute('/d2s-admin-console')({
  component: D2SAdminConsole,
})
