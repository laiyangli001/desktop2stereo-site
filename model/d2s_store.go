package model

// D2SProductPrice is shared by the catalog and authoritative order quotes.
func D2SProductPrice(product, currency string, revokeNumber int) (int64, error) {
	if currency != "CNY" && currency != "USD" {
		return 0, ErrD2SOrderInvalid
	}
	switch product {
	case D2SOrderProductLicense:
		if currency == "CNY" {
			return 9900, nil
		}
		return 2990, nil
	case D2SOrderProductPaidRevoke:
		if revokeNumber < 1 || revokeNumber > 3 {
			return 0, ErrD2SPaidRevokeLimit
		}
		if currency == "CNY" {
			return []int64{1990, 3490, 6990}[revokeNumber-1], nil
		}
		return []int64{299, 499, 999}[revokeNumber-1], nil
	case D2SOrderProductOfflineExtension:
		if currency == "CNY" {
			return d2sConfiguredPrice("D2S_OFFLINE_EXTENSION_CNY_MINOR")
		}
		return d2sConfiguredPrice("D2S_OFFLINE_EXTENSION_USD_MINOR")
	default:
		return 0, ErrD2SOrderInvalid
	}
}
