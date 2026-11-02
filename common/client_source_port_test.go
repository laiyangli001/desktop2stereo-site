package common

import "testing"

func TestClientSourcePortOnlyRecordsDirectClientPort(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "none")
	tests := []struct {
		name       string
		remoteAddr string
		remoteIP   string
		clientIP   string
		want       int
	}{
		{
			name:       "direct IPv4 client",
			remoteAddr: "192.0.2.10:54321",
			remoteIP:   "192.0.2.10",
			clientIP:   "192.0.2.10",
			want:       54321,
		},
		{
			name:       "direct IPv6 client",
			remoteAddr: "[2001:db8::10]:43210",
			remoteIP:   "2001:db8::10",
			clientIP:   "2001:db8::10",
			want:       43210,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClientSourcePort(test.remoteAddr, test.remoteIP, test.clientIP)
			if got == nil || *got != test.want {
				t.Fatalf("ClientSourcePort() = %v, want %d", got, test.want)
			}
		})
	}
}

func TestClientSourcePortOmitsProxyOrUnknownPorts(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "none")
	tests := []struct {
		name       string
		remoteAddr string
		remoteIP   string
		clientIP   string
	}{
		{
			name:       "forwarded client through proxy",
			remoteAddr: "10.0.0.5:37890",
			remoteIP:   "10.0.0.5",
			clientIP:   "203.0.113.4",
		},
		{
			name:       "invalid remote address",
			remoteAddr: "not-a-host-port",
			remoteIP:   "192.0.2.10",
			clientIP:   "192.0.2.10",
		},
		{
			name:       "invalid client address",
			remoteAddr: "192.0.2.10:54321",
			remoteIP:   "192.0.2.10",
			clientIP:   "unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClientSourcePort(test.remoteAddr, test.remoteIP, test.clientIP)
			if got != nil {
				t.Fatalf("ClientSourcePort() = %d, want nil", *got)
			}
		})
	}
}

func TestClientSourcePortOmitsTrustedProxyFallback(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
	got := ClientSourcePort("10.0.0.5:37890", "10.0.0.5", "10.0.0.5")
	if got != nil {
		t.Fatalf("ClientSourcePort() = %d, want nil for trusted proxy", *got)
	}
}

func TestClientSourcePortOmitsDefaultTrustedProxyFallback(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	got := ClientSourcePort("192.168.0.10:54321", "192.168.0.10", "192.168.0.10")
	if got != nil {
		t.Fatalf("ClientSourcePort() = %d, want nil for default trusted proxy", *got)
	}
}

func TestClientSourcePortOmitsInvalidPortValues(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "none")
	for _, remoteAddr := range []string{"192.0.2.10:0", "192.0.2.10:65536", "192.0.2.10:bad"} {
		t.Run(remoteAddr, func(t *testing.T) {
			got := ClientSourcePort(remoteAddr, "192.0.2.10", "192.0.2.10")
			if got != nil {
				t.Fatalf("ClientSourcePort() = %d, want nil", *got)
			}
		})
	}
}
