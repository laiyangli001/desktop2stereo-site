package common

import (
	"net"
	"os"
	"strconv"
)

// ClientSourcePort returns the peer port only when the peer is a direct client.
func ClientSourcePort(remoteAddr, remoteIP, clientIP string) *int {
	remotePeerIP := net.ParseIP(remoteIP)
	resolvedClientIP := net.ParseIP(clientIP)
	if remotePeerIP == nil || resolvedClientIP == nil || !remotePeerIP.Equal(resolvedClientIP) {
		return nil
	}

	trustedProxies, _, err := ResolveTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil || isTrustedProxyIP(remotePeerIP, trustedProxies) {
		return nil
	}

	_, portText, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil
	}
	return &port
}

func isTrustedProxyIP(peerIP net.IP, trustedProxies []string) bool {
	for _, trustedProxy := range trustedProxies {
		if _, network, err := net.ParseCIDR(trustedProxy); err == nil {
			if network.Contains(peerIP) {
				return true
			}
			continue
		}
		if trustedIP := net.ParseIP(trustedProxy); trustedIP != nil && trustedIP.Equal(peerIP) {
			return true
		}
	}
	return false
}
