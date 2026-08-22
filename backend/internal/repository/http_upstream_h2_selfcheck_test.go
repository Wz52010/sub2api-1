package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// 纯函数:Profile 自带 H2 时用之,否则回退按客户端类型默认。
func TestH2SpecFromProfile(t *testing.T) {
	require.Equal(t, "undici", h2SpecFromProfile(&tlsfingerprint.Profile{Name: "Claude Code - Node.js 24.x"}).Name, "无 H2 字段回退 undici")
	require.Equal(t, "reqwest", h2SpecFromProfile(&tlsfingerprint.Profile{Name: "Codex CLI"}).Name, "无 H2 字段回退 reqwest")

	p := &tlsfingerprint.Profile{
		Name:                "custom",
		H2Settings:          [][]uint32{{1, 65536}, {2, 0}, {4, 6291456}, {6, 262144}},
		H2ConnectionFlow:    15663105,
		H2PseudoHeaderOrder: []string{":method", ":authority", ":scheme", ":path"},
	}
	spec := h2SpecFromProfile(p)
	require.Equal(t, "profile:custom", spec.Name, "有 H2 字段用 Profile 自带")
	require.Equal(t, "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p", spec.akamaiString())
	require.Equal(t, spec.akamaiString(), ExpectedProfileH2Akamai(p))

	// 不完整(缺伪头序)→ 回退,避免半配置发出畸形指纹。
	require.Equal(t, "undici", h2SpecFromProfile(&tlsfingerprint.Profile{Name: "x", H2Settings: [][]uint32{{1, 1}}}).Name)
}

// 网络自校验 / 漂移守卫:默认跳过,设 FP_SELFCHECK=1 时对 tls.peet.ws 实测,断言实际发出的
// Akamai 串 == 配置目标(字节级)。用于校准新抓包、以及依赖升级后的回归。
func TestH2SelfCheckReproducesSpec(t *testing.T) {
	if os.Getenv("FP_SELFCHECK") == "" {
		t.Skip("set FP_SELFCHECK=1 to run the network self-check against tls.peet.ws")
	}
	cases := []*tlsfingerprint.Profile{
		{Name: "Claude Code - Node.js 24.x", ALPNProtocols: []string{"h2", "http/1.1"}},
		{Name: "Codex CLI - Node.js 24.x", ALPNProtocols: []string{"h2", "http/1.1"}},
		{
			Name: "custom-db", ALPNProtocols: []string{"h2", "http/1.1"},
			H2Settings:          [][]uint32{{1, 65536}, {2, 0}, {4, 6291456}, {6, 262144}},
			H2ConnectionFlow:    15663105,
			H2PseudoHeaderOrder: []string{":method", ":authority", ":scheme", ":path"},
		},
	}
	for _, p := range cases {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			got, err := VerifyProfileH2Akamai(ctx, p, "")
			require.NoError(t, err)
			require.Equal(t, ExpectedProfileH2Akamai(p), got, "emitted Akamai must match configured spec byte-for-byte")
		})
	}
}
