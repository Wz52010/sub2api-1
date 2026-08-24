// Package model 定义服务层使用的数据模型。
package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// TLSFingerprintProfile TLS 指纹配置模板
// 包含完整的 ClientHello 参数，用于模拟特定客户端的 TLS 握手特征
type TLSFingerprintProfile struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	Description         *string  `json:"description"`
	EnableGREASE        bool     `json:"enable_grease"`
	CipherSuites        []uint16 `json:"cipher_suites"`
	Curves              []uint16 `json:"curves"`
	PointFormats        []uint16 `json:"point_formats"`
	SignatureAlgorithms []uint16 `json:"signature_algorithms"`
	ALPNProtocols       []string `json:"alpn_protocols"`
	SupportedVersions   []uint16 `json:"supported_versions"`
	KeyShareGroups      []uint16 `json:"key_share_groups"`
	PSKModes            []uint16 `json:"psk_modes"`
	Extensions          []uint16 `json:"extensions"`
	// ---- 档2 HTTP/2 帧级指纹（可后台编辑；为空回退按客户端类型的默认 spec）----
	H2Settings          [][]uint32 `json:"h2_settings"`
	H2ConnectionFlow    uint32     `json:"h2_connection_flow"`
	H2PseudoHeaderOrder []string   `json:"h2_pseudo_header_order"`
	H2HeaderOrder       []string   `json:"h2_header_order"`
	H2AkamaiExpected    *string    `json:"h2_akamai_expected"`
	H2Source            *string    `json:"h2_source"`
	// 档2 TLS 扩展顺序随机化（模拟 rustls/reqwest 的 JA3 每连接变化；JA4 稳定）
	ShuffleExtensions bool                          `json:"shuffle_extensions"`
	CreatedAt         time.Time                     `json:"created_at"`
	UpdatedAt         time.Time                     `json:"updated_at"`
	Metadata          TLSFingerprintProfileMetadata `json:"metadata"`
}

// TLSFingerprintProfileMetadata describes the effective profile capabilities
// without claiming that the profile is an official client attestation.
type TLSFingerprintProfileMetadata struct {
	ClientType         string `json:"client_type"`
	ClientVersionRange string `json:"client_version_range"`
	TLSVersionRange    string `json:"tls_version_range"`
	ALPNPreference     string `json:"alpn_preference"`
	FingerprintKey     string `json:"fingerprint_key"`
}

var profileVersionPattern = regexp.MustCompile(`(?i)(?:node(?:\.js)?|claude\s*code|chrome|firefox|safari)\s*v?([0-9]+)`)

// Validate 验证模板配置的有效性
func (p *TLSFingerprintProfile) Validate() error {
	if p.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	return nil
}

// ToTLSProfile 将领域模型转换为运行时使用的 tlsfingerprint.Profile
// 空切片字段会在 dialer 中 fallback 到内置默认值
func (p *TLSFingerprintProfile) ToTLSProfile() *tlsfingerprint.Profile {
	clientOS, clientArch := parseClientOSArch(strings.ToLower(p.Name + " " + valueOrEmpty(p.Description)))
	return &tlsfingerprint.Profile{
		Name:                p.Name,
		EnableGREASE:        p.EnableGREASE,
		CipherSuites:        p.CipherSuites,
		Curves:              p.Curves,
		PointFormats:        p.PointFormats,
		SignatureAlgorithms: p.SignatureAlgorithms,
		ALPNProtocols:       p.ALPNProtocols,
		SupportedVersions:   p.SupportedVersions,
		KeyShareGroups:      p.KeyShareGroups,
		PSKModes:            p.PSKModes,
		Extensions:          p.Extensions,
		H2Settings:          p.H2Settings,
		H2ConnectionFlow:    p.H2ConnectionFlow,
		H2PseudoHeaderOrder: p.H2PseudoHeaderOrder,
		H2HeaderOrder:       p.H2HeaderOrder,
		ShuffleExtensions:   p.ShuffleExtensions,
		ClientOS:            clientOS,
		ClientArch:          clientArch,
	}
}

// parseClientOSArch 从 Profile 名称/描述解析目标系统与架构,输出 Anthropic SDK(stainless)所用取值:
// OS "MacOS"/"Windows"/"Linux";Arch "arm64"/"x64"。无法判定返回空串(表示不联动 OS 头)。
// text 应为已 ToLower 的 "名称 + 描述"。
func parseClientOSArch(text string) (clientOS string, clientArch string) {
	switch {
	case strings.Contains(text, "macos"), strings.Contains(text, "mac os"),
		strings.Contains(text, "darwin"), strings.Contains(text, "osx"):
		clientOS = "MacOS"
	case strings.Contains(text, "windows"), strings.Contains(text, "win32"), strings.Contains(text, "win64"):
		clientOS = "Windows"
	case strings.Contains(text, "linux"):
		clientOS = "Linux"
	}
	switch {
	case strings.Contains(text, "arm64"), strings.Contains(text, "aarch64"):
		clientArch = "arm64"
	case strings.Contains(text, "x64"), strings.Contains(text, "x86_64"),
		strings.Contains(text, "amd64"), strings.Contains(text, "x86-64"):
		clientArch = "x64"
	}
	return clientOS, clientArch
}

// BuildMetadata derives display-only compatibility information from the
// effective ClientHello fields and profile naming. It intentionally avoids
// treating a user-supplied name as proof of a real client version.
func (p *TLSFingerprintProfile) BuildMetadata() TLSFingerprintProfileMetadata {
	if p == nil {
		return TLSFingerprintProfileMetadata{
			ClientType:         "Custom",
			ClientVersionRange: "Unspecified",
			TLSVersionRange:    "Default",
			ALPNPreference:     "Default",
			FingerprintKey:     (&tlsfingerprint.Profile{}).FingerprintKey(),
		}
	}

	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{p.Name, valueOrEmpty(p.Description)}, " ")))
	clientType := "Custom"
	switch {
	// Rust(reqwest)优先于 node 判定:Codex CLI 为 Rust 客户端,已校准的 Codex Profile
	// 名称/描述可能残留 "node" 文案,若不先判 rust 会被误标成 "Node.js / Claude Code"。
	case strings.Contains(text, "rust") || strings.Contains(text, "reqwest"):
		clientType = "Codex CLI / Rust (reqwest)"
	case strings.Contains(text, "claude") || strings.Contains(text, "node"):
		clientType = "Node.js / Claude Code"
	case strings.Contains(text, "chrome"):
		clientType = "Chrome"
	case strings.Contains(text, "firefox"):
		clientType = "Firefox"
	case strings.Contains(text, "safari"):
		clientType = "Safari"
	case strings.Contains(text, "browser"):
		clientType = "Browser"
	}

	versionRange := "Unspecified"
	if match := profileVersionPattern.FindStringSubmatch(text); len(match) > 1 {
		versionRange = match[1] + ".x"
	}

	return TLSFingerprintProfileMetadata{
		ClientType:         clientType,
		ClientVersionRange: versionRange,
		TLSVersionRange:    formatTLSVersions(p.SupportedVersions),
		ALPNPreference:     formatALPN(p.ALPNProtocols),
		FingerprintKey:     p.ToTLSProfile().FingerprintKey(),
	}
}

// RefreshMetadata updates the response-only metadata after a profile changes.
func (p *TLSFingerprintProfile) RefreshMetadata() {
	if p != nil {
		p.Metadata = p.BuildMetadata()
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatTLSVersions(versions []uint16) string {
	if len(versions) == 0 {
		return "Default"
	}
	labels := make([]string, 0, len(versions))
	for _, version := range versions {
		switch version {
		case 0x0304:
			labels = append(labels, "TLS 1.3")
		case 0x0303:
			labels = append(labels, "TLS 1.2")
		default:
			labels = append(labels, fmt.Sprintf("0x%04x", version))
		}
	}
	return strings.Join(labels, ", ")
}

func formatALPN(protocols []string) string {
	if len(protocols) == 0 {
		return "Default"
	}
	return strings.Join(protocols, ", ")
}
