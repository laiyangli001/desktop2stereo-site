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

import { AlipayPCSettingsPanel } from '../features/d2s/alipay-settings-panel'
import { downloadBlob } from '../features/d2s/download'

const MAX_AUDIT_RETENTION_DAYS = 2_147_483_647

type AdminSession = { id: number; email: string; role: string }
type ApiEnvelope<T> = { success: boolean; data: T; code?: string }
type RecordRow = Record<string, unknown>
type ExportApproval = {
  id: string
  export_type: string
  request_reason: string
  status: string
  expires_at: number
  filter_snapshot: string
  requested_by_email: string
}

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
  | 'support'
  | 'accounts'
  | 'retention'
  | 'alipay'

function defaultAdminSection(role: string): AdminSection | null {
  if (role === 'super_admin' || role === 'audit_reader') return 'logs'
  if (role === 'account_operator') return 'accounts'
  return null
}

const adminErrorMessages: Record<string, string> = {
  RETENTION_REJECTED: '保留天数不能低于 366 天。',
  RETENTION_UNAVAILABLE:
    '独立审计清理数据库未配置或当前不可用，请联系数据库管理员。',
  RETENTION_FAILED:
    '清理操作失败，未确认清理结果；请检查后台审计记录和数据库状态。',
  EXPORT_APPROVAL_REQUIRED: '请先申请导出审批，并由另一名超级管理员批准。',
  EXPORT_DOWNLOAD_FAILED: '导出失败；请检查审批状态和筛选条件。',
  REVIEW_REASON_REQUIRED: '请填写审批意见。',
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
  const [supportTickets, setSupportTickets] = useState<RecordRow[]>([])
  const [supportStatus, setSupportStatus] = useState('')
  const [supportReplies, setSupportReplies] = useState<Record<string, string>>(
    {}
  )
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
  const [retentionDays, setRetentionDays] = useState('366')
  const [exportApproval, setExportApproval] = useState<ExportApproval | null>(
    null
  )
  const [exportReason, setExportReason] = useState('')
  const [pendingApprovals, setPendingApprovals] = useState<ExportApproval[]>([])
  const [reviewReason, setReviewReason] = useState('')

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

  const exportQuery = (target: AdminSection) => {
    const query = new URLSearchParams()
    if (target === 'logs') {
      Object.entries(filters).forEach(([key, value]) => {
        if (value.trim()) query.set(key, value.trim())
      })
    } else {
      if (filters.start_time.trim()) {
        query.set('start_time', filters.start_time.trim())
      }
      if (filters.end_time.trim()) {
        query.set('end_time', filters.end_time.trim())
      }
    }
    return query
  }

  const requestExportApproval = (target: AdminSection) =>
    run(async () => {
      const exportType =
        target === 'logs' ? 'USER_ACTION_LOGS' : 'REGISTRATIONS'
      const query = exportQuery(target)
      query.set('export_type', exportType)
      const approval = await adminRequest<ExportApproval>(
        `/api/v1/admin/audit/export-approvals?${query}`,
        { method: 'POST', body: JSON.stringify({ reason: exportReason }) }
      )
      setExportApproval(approval)
    })

  const refreshExportApproval = () =>
    run(async () => {
      if (!exportApproval) return
      const approval = await adminRequest<ExportApproval>(
        `/api/v1/admin/audit/export-approvals/${encodeURIComponent(exportApproval.id)}`
      )
      setExportApproval(approval)
    })

  const downloadApprovedExport = (target: AdminSection) =>
    run(async () => {
      if (!exportApproval || exportApproval.status !== 'APPROVED') {
        throw new Error('EXPORT_APPROVAL_REQUIRED')
      }
      const query = exportQuery(target)
      query.set('approval_id', exportApproval.id)
      const exportPath =
        target === 'logs'
          ? '/api/v1/admin/audit/user-events/export.csv'
          : '/api/v1/admin/audit/registrations/export.csv'
      const response = await fetch(`${exportPath}?${query}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!response.ok) throw new Error('EXPORT_DOWNLOAD_FAILED')
      const filename =
        response.headers
          .get('Content-Disposition')
          ?.match(/filename="?([^";]+)"?/i)?.[1] || 'audit_export.csv'
      downloadBlob(await response.blob(), filename)
      setExportApproval(null)
      setExportReason('')
    })

  const loadPendingApprovals = () =>
    run(async () => {
      const data = await adminRequest<{ items: ExportApproval[] }>(
        '/api/v1/admin/audit/export-approvals?page=1&page_size=50'
      )
      setPendingApprovals(data.items)
    })

  const reviewExportApproval = (
    approval: ExportApproval,
    decision: 'APPROVED' | 'REJECTED'
  ) =>
    run(async () => {
      if (!reviewReason.trim()) throw new Error('REVIEW_REASON_REQUIRED')
      await adminRequest(
        `/api/v1/admin/audit/export-approvals/${encodeURIComponent(approval.id)}`,
        {
          method: 'PUT',
          body: JSON.stringify({ decision, reason: reviewReason }),
        }
      )
      setReviewReason('')
      const data = await adminRequest<{ items: ExportApproval[] }>(
        '/api/v1/admin/audit/export-approvals?page=1&page_size=50'
      )
      setPendingApprovals(data.items)
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

  const loadSupportTickets = (targetPage = page) =>
    run(async () => {
      const query = new URLSearchParams({
        page: String(targetPage),
        page_size: '50',
      })
      if (supportStatus) query.set('status', supportStatus)
      const data = await adminRequest<{
        items: RecordRow[]
        total: number
      }>(`/api/v1/admin/support/tickets?${query}`)
      setSupportTickets(data.items)
      setTotal(data.total)
      setPage(targetPage)
    })

  const updateSupportTicket = (ticket: RecordRow) =>
    run(async () => {
      const id = stringifyCell(ticket.id)
      const updated = await adminRequest<RecordRow>(
        `/api/v1/admin/support/tickets/${encodeURIComponent(id)}`,
        {
          method: 'PUT',
          body: JSON.stringify({
            status: stringifyCell(ticket.status),
            admin_reply:
              supportReplies[id] ?? stringifyCell(ticket.admin_reply),
          }),
        }
      )
      setSupportTickets((current) =>
        current.map((row) => (stringifyCell(row.id) === id ? updated : row))
      )
      setSupportReplies((current) => {
        const next = { ...current }
        delete next[id]
        return next
      })
      setError('工单处理结果已保存并写入审计')
    })

  const goToPage = (targetPage: number) => {
    if (targetPage < 1 || targetPage > Math.ceil(total / 50)) return
    if (section === 'logs') void loadLogs(targetPage)
    if (section === 'registrations') void loadRegistrations(targetPage)
    if (section === 'purchases') void loadPurchases(targetPage)
    if (section === 'support') void loadSupportTickets(targetPage)
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

  const runRetention = () =>
    run(async () => {
      if (
        !window.confirm(
          `按至少 ${retentionDays} 天期限清理已到期的用户行为日志、注册快照和管理员审计记录。此操作不可撤回。`
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
            ['support', '投诉与咨询', 'account_operator'],
            ['accounts', '紧急限制', 'account_operator'],
            ['retention', '日志留存', 'super_admin'],
            ['alipay', '支付宝支付', 'super_admin'],
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
                  setExportApproval(null)
                  setPage(1)
                  setTotal(0)
                  setRows([])
                  setRegistrations([])
                  setPurchases([])
                  setSupportTickets([])
                  if (key === 'support') void loadSupportTickets(1)
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
                      setExportApproval(null)
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
                onClick={() => requestExportApproval(section)}
                type='button'
              >
                申请导出审批
              </button>
            </div>
            <div className='space-y-3 rounded border p-3'>
              <label className='block space-y-1 text-sm'>
                <span>导出用途（审批必填）</span>
                <textarea
                  className='w-full rounded border p-2'
                  maxLength={500}
                  onChange={(event) => setExportReason(event.target.value)}
                  value={exportReason}
                  rows={2}
                />
              </label>
              {exportApproval && (
                <div
                  aria-live='polite'
                  className='space-y-2 text-sm'
                  role='status'
                >
                  <p>
                    审批编号：{exportApproval.id} · 状态：
                    {exportApproval.status}
                  </p>
                  <p className='break-all'>
                    申请范围：{exportApproval.filter_snapshot}
                  </p>
                  <div className='flex flex-wrap gap-2'>
                    <button
                      className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                      disabled={busy}
                      onClick={refreshExportApproval}
                      type='button'
                    >
                      刷新审批状态
                    </button>
                    <button
                      className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                      disabled={busy || exportApproval.status !== 'APPROVED'}
                      onClick={() => downloadApprovedExport(section)}
                      type='button'
                    >
                      下载已批准 CSV
                    </button>
                  </div>
                </div>
              )}
            </div>
            {admin.role === 'super_admin' && (
              <section
                aria-label='导出审批队列'
                className='space-y-3 rounded border p-3'
              >
                <div className='flex items-center justify-between gap-3'>
                  <h3 className='font-medium'>待审批导出</h3>
                  <button
                    className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                    disabled={busy}
                    onClick={loadPendingApprovals}
                    type='button'
                  >
                    刷新待审批
                  </button>
                </div>
                <label className='block space-y-1 text-sm'>
                  <span>审批意见（批准或拒绝均必填）</span>
                  <textarea
                    className='w-full rounded border p-2'
                    maxLength={500}
                    onChange={(event) => setReviewReason(event.target.value)}
                    value={reviewReason}
                    rows={2}
                  />
                </label>
                {pendingApprovals.length === 0 ? (
                  <p role='status'>当前没有待审批申请。</p>
                ) : (
                  pendingApprovals.map((approval) => (
                    <article
                      className='space-y-2 rounded border p-3 text-sm'
                      key={approval.id}
                    >
                      <p>
                        申请人：{approval.requested_by_email} · 类型：
                        {approval.export_type}
                      </p>
                      <p>用途：{approval.request_reason}</p>
                      <p className='break-all'>
                        筛选范围：{approval.filter_snapshot}
                      </p>
                      <div className='flex flex-wrap gap-2'>
                        <button
                          className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                          disabled={
                            busy ||
                            approval.requested_by_email.toLowerCase() ===
                              admin.email.toLowerCase()
                          }
                          onClick={() =>
                            reviewExportApproval(approval, 'APPROVED')
                          }
                          type='button'
                        >
                          批准
                        </button>
                        <button
                          className='hover:bg-muted/70 focus-visible:ring-ring rounded border px-3 py-2 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
                          disabled={
                            busy ||
                            approval.requested_by_email.toLowerCase() ===
                              admin.email.toLowerCase()
                          }
                          onClick={() =>
                            reviewExportApproval(approval, 'REJECTED')
                          }
                          type='button'
                        >
                          拒绝
                        </button>
                      </div>
                    </article>
                  ))
                )}
              </section>
            )}
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

        {section === 'support' && (
          <section className='space-y-4 rounded border p-4'>
            <div className='flex flex-wrap items-end justify-between gap-3'>
              <div>
                <h2 className='text-lg font-medium'>投诉与咨询工单</h2>
                <p className='text-muted-foreground text-sm'>
                  联系邮箱、订单号、问题描述和处理回复仅供授权管理员处理。
                </p>
              </div>
              <label className='grid gap-1 text-sm'>
                <span>处理状态</span>
                <select
                  className='bg-background rounded border p-2'
                  value={supportStatus}
                  onChange={(event) => setSupportStatus(event.target.value)}
                >
                  <option value=''>全部</option>
                  <option value='OPEN'>待处理</option>
                  <option value='IN_PROGRESS'>处理中</option>
                  <option value='RESOLVED'>已解决</option>
                  <option value='CLOSED'>已关闭</option>
                </select>
              </label>
              <button
                className='hover:bg-muted/70 rounded border px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:outline-none'
                disabled={busy}
                onClick={() => loadSupportTickets(1)}
                type='button'
              >
                刷新工单
              </button>
            </div>
            {supportTickets.length === 0 ? (
              <p aria-live='polite' className='p-3 text-sm' role='status'>
                暂无工单。
              </p>
            ) : (
              <div className='space-y-4'>
                {supportTickets.map((ticket) => {
                  const id = stringifyCell(ticket.id)
                  return (
                    <article
                      className='space-y-3 rounded-lg border p-4'
                      key={id}
                    >
                      <div className='flex flex-wrap justify-between gap-3'>
                        <div>
                          <h3 className='font-medium'>
                            {stringifyCell(ticket.subject)}
                          </h3>
                          <p className='text-muted-foreground text-xs'>
                            {id} · 用户 {stringifyCell(ticket.user_id)} ·{' '}
                            {stringifyCell(ticket.category)} ·{' '}
                            {stringifyCell(ticket.created_at)}
                          </p>
                        </div>
                        <span className='text-sm'>
                          {stringifyCell(ticket.status)}
                        </span>
                      </div>
                      <dl className='grid gap-2 text-sm sm:grid-cols-2'>
                        <div>
                          <dt className='text-muted-foreground'>联系邮箱</dt>
                          <dd className='break-all'>
                            {stringifyCell(ticket.contact_email)}
                          </dd>
                        </div>
                        <div>
                          <dt className='text-muted-foreground'>订单号</dt>
                          <dd>{stringifyCell(ticket.order_id) || '未提供'}</dd>
                        </div>
                      </dl>
                      <p className='text-sm break-words whitespace-pre-wrap'>
                        {stringifyCell(ticket.message)}
                      </p>
                      <label className='grid gap-1 text-sm'>
                        <span>内部处理备注</span>
                        <textarea
                          className='min-h-20 rounded border p-2'
                          maxLength={5000}
                          value={
                            supportReplies[id] ??
                            stringifyCell(ticket.admin_reply)
                          }
                          onChange={(event) =>
                            setSupportReplies((current) => ({
                              ...current,
                              [id]: event.target.value,
                            }))
                          }
                        />
                      </label>
                      <div className='flex flex-wrap items-center gap-2'>
                        <label className='flex items-center gap-2 text-sm'>
                          <span>更新状态</span>
                          <select
                            className='bg-background rounded border p-2'
                            value={stringifyCell(ticket.status)}
                            onChange={(event) =>
                              setSupportTickets((current) =>
                                current.map((row) =>
                                  stringifyCell(row.id) === id
                                    ? { ...row, status: event.target.value }
                                    : row
                                )
                              )
                            }
                          >
                            <option value='OPEN'>待处理</option>
                            <option value='IN_PROGRESS'>处理中</option>
                            <option value='RESOLVED'>已解决</option>
                            <option value='CLOSED'>已关闭</option>
                          </select>
                        </label>
                        <button
                          className='bg-primary text-primary-foreground hover:bg-primary/90 rounded px-3 py-2 text-sm focus-visible:ring-2 focus-visible:outline-none'
                          disabled={busy}
                          onClick={() => updateSupportTicket(ticket)}
                          type='button'
                        >
                          保存处理状态与备注
                        </button>
                      </div>
                    </article>
                  )
                })}
              </div>
            )}
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

        {section === 'alipay' && admin.role === 'super_admin' && (
          <AlipayPCSettingsPanel />
        )}

        {section === 'retention' && (
          <section className='space-y-4 rounded border p-4'>
            <h2 className='text-lg font-medium'>受控日志清理</h2>
            <p className='text-muted-foreground text-sm'>
              设置的清理期必须在 366 至 {MAX_AUDIT_RETENTION_DAYS}{' '}
              天之间。用户行为日志、注册快照和管理员操作审计均至少保留 366
              天。执行前会分别记录各表的截止时间、范围与数量。
            </p>
            <div className='flex flex-wrap items-end gap-2'>
              <label className='grid gap-1 text-sm'>
                <span>保留天数（最少 366 天）</span>
                <input
                  autoComplete='off'
                  className='w-40 rounded border p-2'
                  name='retention_days'
                  max={MAX_AUDIT_RETENTION_DAYS}
                  min='366'
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
                  Number(retentionDays) < 366 ||
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
