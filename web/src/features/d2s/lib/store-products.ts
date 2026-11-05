export const STORE_PRODUCT_CONTENT = {
  license: {
    title: 'Full license',
    description: 'A new Desktop2Stereo software license for your account.',
    unit: 'One-time purchase',
    features: [
      'One device per license',
      'Online and offline modes',
      'Manage binding in authorization management',
    ],
  },
  paid_revoke: {
    title: 'Paid revoke',
    description: 'Release the device binding of an existing eligible license.',
    unit: 'Per service',
    features: [
      'Requires a bound, non-permanent license',
      'Up to three paid revokes per UTC month',
      'Tiered pricing; review the exact quote before payment',
    ],
  },
  offline_extension: {
    title: 'Offline extension',
    description: 'Extend offline access for an existing eligible license.',
    unit: '30-day extension',
    features: [
      'Requires an active, non-permanent license',
      'Adds up to 30 days of offline access',
      'Extension cannot exceed the license expiry date',
    ],
  },
} as const
