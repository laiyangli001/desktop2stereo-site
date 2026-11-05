package model

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestD2SStorePricesMatchExistingPurchaseRules(t *testing.T) {
	t.Setenv("D2S_OFFLINE_EXTENSION_CNY_MINOR", "1990")
	t.Setenv("D2S_OFFLINE_EXTENSION_USD_MINOR", "299")
	for _, test := range []struct {
		product, currency string
		number            int
		amount            int64
	}{
		{D2SOrderProductLicense, "CNY", 0, 9900}, {D2SOrderProductLicense, "USD", 0, 2990},
		{D2SOrderProductPaidRevoke, "CNY", 1, 1990}, {D2SOrderProductPaidRevoke, "CNY", 2, 3490}, {D2SOrderProductPaidRevoke, "CNY", 3, 6990},
		{D2SOrderProductPaidRevoke, "USD", 1, 299}, {D2SOrderProductPaidRevoke, "USD", 2, 499}, {D2SOrderProductPaidRevoke, "USD", 3, 999},
		{D2SOrderProductOfflineExtension, "CNY", 0, 1990}, {D2SOrderProductOfflineExtension, "USD", 0, 299},
	} {
		amount, err := D2SProductPrice(test.product, test.currency, test.number)
		require.NoError(t, err)
		require.Equal(t, test.amount, amount)
	}
	_, err := D2SProductPrice(D2SOrderProductPaidRevoke, "CNY", 4)
	require.ErrorIs(t, err, ErrD2SPaidRevokeLimit)
	_, err = D2SProductPrice(D2SOrderProductLicense, "EUR", 0)
	require.ErrorIs(t, err, ErrD2SOrderInvalid)
	t.Setenv("D2S_OFFLINE_EXTENSION_CNY_MINOR", "")
	_, err = D2SProductPrice(D2SOrderProductOfflineExtension, "CNY", 0)
	require.ErrorIs(t, err, ErrD2SPriceNotConfigured)
}
