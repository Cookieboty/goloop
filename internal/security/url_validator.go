// internal/security/url_validator.go
package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// testMode allows bypassing validation for testing
var testMode = false

// SetTestMode enables/disables test mode (for unit tests only)
func SetTestMode(enabled bool) {
	testMode = enabled
}

var (
	blockedDomains = []string{
		"localhost",
		"metadata.google.internal",
		"169.254.169.254", // AWS/GCP/Azure metadata
		"metadata.azure.com",
		"metadata",
		"169.254.169",
		"fd00:ec2::254", // AWS IMDSv2 IPv6
	}
)

// ValidateImageURL 验证图片 URL 是否安全
func ValidateImageURL(rawURL string) error {
	// Skip validation in test mode
	if testMode {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// 1. 仅允许 HTTPS
	if u.Scheme != "https" {
		return fmt.Errorf("only https:// URLs are allowed, got: %s", u.Scheme)
	}

	host := u.Hostname()

	// 2. 禁止特殊域名（精确匹配或后缀匹配）
	hostLower := strings.ToLower(host)
	for _, blocked := range blockedDomains {
		// 精确匹配或作为后缀（如 .metadata.google.internal）
		if hostLower == blocked || strings.HasSuffix(hostLower, "."+blocked) {
			return fmt.Errorf("blocked domain: %s", host)
		}
	}

	// 3. 解析 IP 地址（如果是 IP）
	ip := net.ParseIP(host)
	if ip != nil {
		if err := validateIP(ip); err != nil {
			return err
		}
	} else {
		// 4. 域名解析检查（防止 DNS rebinding）
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("DNS lookup failed: %w", err)
		}
		for _, ip := range ips {
			if err := validateIP(ip); err != nil {
				return fmt.Errorf("resolved IP is blocked: %w", err)
			}
		}
	}

	return nil
}

func validateIP(ip net.IP) error {
	// 禁止内网地址
	if ip.IsLoopback() {
		return fmt.Errorf("loopback addresses not allowed")
	}
	if ip.IsPrivate() {
		return fmt.Errorf("private IP addresses not allowed")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local addresses not allowed")
	}

	// 禁止特殊 IP 段
	if ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("invalid IP address")
	}

	return nil
}
