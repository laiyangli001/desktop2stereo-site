export type D2SResponse<T> = {
  version?: number
  request_id?: string
  success: boolean
  data?: T
  error?: { code?: string; message?: string }
}

export type D2SLicense = {
  user_id?: number
  region?: string
  id: string
  license_code: string
  kind: string
  status: string
  mode: string
  device_hash?: string
  fingerprint_version: number
  offline_period_days: number
  offline_valid_until: number
  expires_at: number
}

export type D2SOrder = {
  id: string
  user_id?: number
  product: string
  provider: string
  region: string
  currency: string
  amount_minor: number
  balance_minor: number
  gateway_minor: number
  status: string
  created_at: number
  completed_at: number
  expires_at: number
}

export type D2SPaymentEvent = {
  id: string
  provider: string
  provider_event_id: string
  order_id: string
  event_type: string
  amount_minor: number
  currency: string
  processed_at: number
}

export type D2SBalanceAccount = {
  id: string
  user_id?: number
  currency: string
  available_minor: number
  reserved_minor: number
}

export type D2SWalletSummary = {
  accounts: D2SBalanceAccount[]
  transactions: D2SBalanceTransaction[]
  orders: D2SOrder[]
  invite?: D2SInviteInfo
  inviteRecords: D2SInviteReward[]
  minWithdrawalMinor: number
}

export type D2SBalanceTransaction = {
  id: string
  user_id?: number
  currency: string
  kind: string
  amount_minor: number
  order_id?: string
  reference_id?: string
  created_at: number
}

export type D2SInviteInfo = {
  invite_code: string
  rewarded_invitees: number
}

export type D2SInviteReward = {
  id: string
  invitee_user_id: number
  currency: string
  amount_minor: number
  status: string
  created_at: number
}

export type D2SWithdrawal = {
  id: string
  user_id?: number
  currency: string
  amount_minor: number
  status: string
  created_at: number
  updated_at: number
}

export type D2SUnbindRequest = {
  id: string
  license_id: string
  user_id: number
  reason: string
  proof_ref?: string
  status: string
  review_note?: string
  created_at: number
  updated_at: number
}

export type D2SSigningKey = {
  key_id: string
  algorithm: string
  public_jwk: string
  status: string
  created_at: number
  retired_at: number
  is_current?: boolean
}

export type D2SReconciliationReport = {
  start_at: number
  end_at: number
  orders: number
  payment_events: number
  paid_orders: number
  chargeback_orders: number
  pending_expired: number
  mismatches: Array<{
    order_id?: string
    provider_event_id?: string
    kind: string
  }>
}
