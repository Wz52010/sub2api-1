package repository

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	fhttp2 "github.com/bogdanfinn/fhttp/http2"
	butls "github.com/bogdanfinn/utls"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// 档2:出站 HTTP/2 帧级指纹。
//
// 档1 已让指纹链路协商到 h2,但 H2 SETTINGS/WINDOW_UPDATE/PRIORITY/伪头序仍是 Go
// x/net/http2 的运行时特征(伪头序 a,m,p,s、SETTINGS 4:4194304;5:16384;6:10485760…),
// 与真实 undici(Node→Claude)/reqwest(Rust→Codex)不一致。档2 改用 bogdanfinn/fhttp
// 的 http2.Transport,它允许精确设定 SETTINGS 的内容与顺序、连接级 WINDOW_UPDATE、伪头
// 顺序与普通 header 顺序,从而把 Akamai H2 指纹调成目标客户端形状。
//
// TLS 层(ClientHello/JA3/JA4)继续复用既有 refraction/utls 拨号器,不改动——档1 已验证
// 其 JA3 与 Node profile 一致;档2 只在其上叠加 H2 帧层。
//
// 全部行为在配置开关 GATEWAY_TLS_FINGERPRINT_HTTP2_FRAME_FINGERPRINT_ENABLED 之后,默认关闭。

const fingerprintH2FrameDialTimeout = 30 * time.Second

// h2Setting 是一条 HTTP/2 SETTINGS(id:value),按发送顺序排列。
type h2Setting struct {
	ID  uint16
	Val uint32
}

// h2FingerprintSpec 描述一个客户端的 H2 帧级指纹。用平凡类型表达,不泄露 fhttp 类型,
// 便于纯函数单测;构建 Transport 时再转换为 fhttp2 类型。
type h2FingerprintSpec struct {
	Name string
	// Settings 按发送顺序;可包含 id 1(HEADER_TABLE_SIZE)与 4(INITIAL_WINDOW_SIZE)——
	// bogdanfinn/fhttp 此分支直接按 SettingsOrder 逐条写帧,并从 Settings 读 IWS/HTS 派生流控。
	Settings []h2Setting
	// ConnectionFlow 为连接级 WINDOW_UPDATE 增量(0 表示用库默认 ~15MB)。
	ConnectionFlow uint32
	// PseudoHeaderOrder 伪头顺序,全名形式,如 [":method",":authority",":scheme",":path"]。
	PseudoHeaderOrder []string
	// HeaderOrder 普通 header 顺序(小写);为空则不强制顺序。
	HeaderOrder []string
}

// akamaiString 以 Akamai 记法拼出该 spec 的 H2 指纹(SETTINGS|WINDOW_UPDATE|PRIORITY|伪头序),
// PRIORITY 段固定为 0(本项目暂不发独立 PRIORITY 帧)。用于日志/诊断与探针断言。
func (s h2FingerprintSpec) akamaiString() string {
	parts := make([]string, 0, len(s.Settings))
	for _, st := range s.Settings {
		parts = append(parts, fmt.Sprintf("%d:%d", st.ID, st.Val))
	}
	abbr := make([]string, 0, len(s.PseudoHeaderOrder))
	for _, p := range s.PseudoHeaderOrder {
		switch strings.ToLower(strings.TrimPrefix(p, ":")) {
		case "method":
			abbr = append(abbr, "m")
		case "authority":
			abbr = append(abbr, "a")
		case "scheme":
			abbr = append(abbr, "s")
		case "path":
			abbr = append(abbr, "p")
		}
	}
	return fmt.Sprintf("%s|%d|0|%s", strings.Join(parts, ";"), s.ConnectionFlow, strings.Join(abbr, ","))
}

// h2SpecUndici 是 Node.js/undici(Claude Code 链路)的种子指纹。
//
// 注意(calibrate):以下数值为基于 nghttp2/Node 常见形态的合理种子,并非从真实 Claude Code
// 抓包确证的字节值。上生产前应对真实客户端在 tls.peet.ws 抓 http2.akamai_fingerprint 校准。
// 探针(cmd/fpprobe)会断言实际发出的 Akamai 串等于本 spec 的 akamaiString()。
var h2SpecUndici = h2FingerprintSpec{
	Name:              "undici",
	Settings:          []h2Setting{{1, 65536}, {2, 0}, {4, 6291456}, {6, 262144}},
	ConnectionFlow:    15663105,
	PseudoHeaderOrder: []string{":method", ":authority", ":scheme", ":path"},
	HeaderOrder:       nil,
}

// h2SpecReqwest 是 Rust/reqwest+h2(真实 Codex CLI 链路)的种子指纹。
//
// 注意(calibrate):同样为种子值,需以真实 codex_cli_rs 抓包校准。默认不会被启用——Codex/
// OpenAI 走原生转发(Do + 内置 h2),不进本指纹链路;仅当把 Codex 账号显式绑定到指纹 Profile
// 并开启档2 时才会用到,以保持 Codex 现有稳定路径不被改动。
var h2SpecReqwest = h2FingerprintSpec{
	Name: "reqwest",
	// 校准来源:真实 codex-cli 0.148.0-alpha.15 / macOS 15.5 / arm64,tls.peet.ws 抓包。
	// H2 Akamai(稳定):2:0;4:2097152;5:16384;6:16384|5177345|0|m,s,a,p
	Settings:          []h2Setting{{2, 0}, {4, 2097152}, {5, 16384}, {6, 16384}},
	ConnectionFlow:    5177345,
	PseudoHeaderOrder: []string{":method", ":scheme", ":authority", ":path"},
	HeaderOrder:       nil,
}

// selectH2Fingerprint 依据 Profile 选择 H2 指纹 spec(客户端类型感知)。
//
// 真实 Codex CLI 是 Rust(reqwest),即便 Profile 名字写着 Node.js 也应用 reqwest 形态;
// 其余(Claude Code / Node 通用)用 undici。无法判断时回退 undici(当前唯一激活的 Claude 链路)。
func selectH2Fingerprint(profile *tlsfingerprint.Profile) h2FingerprintSpec {
	if profile != nil && strings.Contains(strings.ToLower(profile.Name), "codex") {
		return h2SpecReqwest
	}
	return h2SpecUndici
}

// h2SpecFromProfile 优先使用 Profile 自带的 H2 帧级指纹(DB 存储、后台可编辑);未配置时
// 回退到按客户端类型选择的内置默认 spec。这样"跟随 Claude/GPT 更新指纹"变成改数据
// (Profile)而非改代码——配合服务端自校验(VerifyH2Fingerprint)即可安全同步。
func h2SpecFromProfile(profile *tlsfingerprint.Profile) h2FingerprintSpec {
	if profile != nil && len(profile.H2Settings) > 0 && len(profile.H2PseudoHeaderOrder) > 0 {
		settings := make([]h2Setting, 0, len(profile.H2Settings))
		for _, pair := range profile.H2Settings {
			if len(pair) == 2 {
				settings = append(settings, h2Setting{ID: uint16(pair[0]), Val: pair[1]})
			}
		}
		if len(settings) > 0 {
			return h2FingerprintSpec{
				Name:              "profile:" + profile.Name,
				Settings:          settings,
				ConnectionFlow:    profile.H2ConnectionFlow,
				PseudoHeaderOrder: profile.H2PseudoHeaderOrder,
				HeaderOrder:       profile.H2HeaderOrder,
			}
		}
	}
	return selectH2Fingerprint(profile)
}

// buildFingerprintH2FrameTransport 构建档2 的 H2 帧级指纹 Transport。
//
// 复用 fingerprintDialTLSContext(与档1/h1 口径一致:直连/socks5/http 支持,https 代理/未知
// 类型不支持)拿到 utls 拨号器,交给 fhttp2.Transport;并按 spec 设定 SETTINGS/顺序/
// WINDOW_UPDATE/伪头序。返回一个把 net/http 请求桥接到 fhttp 的 RoundTripper。
func buildFingerprintH2FrameTransport(settings poolSettings, proxyURL *url.URL, profile *tlsfingerprint.Profile) (http.RoundTripper, error) {
	dial, ok := fingerprintDialTLSContext(proxyURL, profile)
	if !ok {
		scheme := ""
		if proxyURL != nil {
			scheme = proxyURL.Scheme
		}
		return nil, fmt.Errorf("tls fingerprint http2-frame unsupported for proxy scheme %q", scheme)
	}
	spec := h2SpecFromProfile(profile)

	tr := &fhttp2.Transport{
		AllowHTTP:          false,
		DisableCompression: false, // 与既有 utls/h1 及档1 一致:不自行改动压缩语义,交给 decompressResponseBody
		ReadIdleTimeout:    openAIHTTP2ReadIdleTimeout,
		PingTimeout:        openAIHTTP2PingTimeout,
		ConnectionFlow:     spec.ConnectionFlow,
		Settings:           make(map[fhttp2.SettingID]uint32, len(spec.Settings)),
		SettingsOrder:      make([]fhttp2.SettingID, 0, len(spec.Settings)),
		PseudoHeaderOrder:  append([]string(nil), spec.PseudoHeaderOrder...),
		DialTLS: func(network, addr string, _ *butls.Config) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), fingerprintH2FrameDialTimeout)
			defer cancel()
			return dial(ctx, network, addr)
		},
	}
	for _, st := range spec.Settings {
		id := fhttp2.SettingID(st.ID)
		tr.Settings[id] = st.Val
		tr.SettingsOrder = append(tr.SettingsOrder, id)
	}
	_ = settings // fhttp2.Transport 单连接多路复用,不复用 http.Transport 连接池尺寸参数。

	return &h2FrameRoundTripper{tr: tr, headerOrder: append([]string(nil), spec.HeaderOrder...), spec: spec}, nil
}

// h2FrameRoundTripper 把 net/http 的 RoundTrip 桥接到 fhttp 的 h2 Transport。
// body 直接透传(不缓冲),保证 SSE 流式与大响应不被破坏;请求 ctx 透传以支持取消。
type h2FrameRoundTripper struct {
	tr          *fhttp2.Transport
	headerOrder []string
	spec        h2FingerprintSpec
}

func (rt *h2FrameRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	freq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	freq.Header = stdHeaderToFhttp(req.Header)
	if len(rt.headerOrder) > 0 {
		freq.Header[fhttp.HeaderOrderKey] = append([]string(nil), rt.headerOrder...)
	}
	if req.ContentLength != 0 {
		freq.ContentLength = req.ContentLength
	}
	if req.Host != "" {
		freq.Host = req.Host
	}
	if len(req.Trailer) > 0 {
		freq.Trailer = stdHeaderToFhttp(req.Trailer)
	}

	fresp, err := rt.tr.RoundTrip(freq)
	if err != nil {
		return nil, err
	}
	return fhttpRespToStd(fresp, req), nil
}

// CloseIdleConnections 让 http.Client 的清理路径(通过类型断言调用)能关闭底层 fhttp 连接。
func (rt *h2FrameRoundTripper) CloseIdleConnections() {
	rt.tr.CloseIdleConnections()
}

func stdHeaderToFhttp(h http.Header) fhttp.Header {
	out := make(fhttp.Header, len(h))
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func fhttpRespToStd(fresp *fhttp.Response, origReq *http.Request) *http.Response {
	resp := &http.Response{
		Status:           fresp.Status,
		StatusCode:       fresp.StatusCode,
		Proto:            fresp.Proto,
		ProtoMajor:       fresp.ProtoMajor,
		ProtoMinor:       fresp.ProtoMinor,
		Header:           make(http.Header, len(fresp.Header)),
		Body:             fresp.Body,
		ContentLength:    fresp.ContentLength,
		TransferEncoding: fresp.TransferEncoding,
		Close:            fresp.Close,
		Uncompressed:     fresp.Uncompressed,
		Request:          origReq,
	}
	if resp.Body == nil {
		resp.Body = io.NopCloser(strings.NewReader(""))
	}
	for k, v := range fresp.Header {
		if k == fhttp.HeaderOrderKey || k == fhttp.PHeaderOrderKey {
			continue
		}
		resp.Header[k] = append([]string(nil), v...)
	}
	if len(fresp.Trailer) > 0 {
		resp.Trailer = make(http.Header, len(fresp.Trailer))
		for k, v := range fresp.Trailer {
			resp.Trailer[k] = append([]string(nil), v...)
		}
	}
	return resp
}
