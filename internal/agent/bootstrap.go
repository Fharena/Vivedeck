package agent

import (
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type HTTPServerConfig struct {
	PublicAgentBaseURL         string
	PublicSignalingBaseURL     string
	BootstrapRecentThreadLimit int
}

type BootstrapResponse struct {
	AgentBaseURL     string                `json:"agentBaseUrl,omitempty"`
	SignalingBaseURL string                `json:"signalingBaseUrl,omitempty"`
	WorkspaceRoot    string                `json:"workspaceRoot,omitempty"`
	Adapter          BootstrapAdapterView  `json:"adapter"`
	CurrentThreadID  string                `json:"currentThreadId,omitempty"`
	RecentThreads    []BootstrapThreadView `json:"recentThreads,omitempty"`
}

type BootstrapAdapterView struct {
	Name     string `json:"name,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Provider string `json:"provider,omitempty"`
	Ready    bool   `json:"ready"`
}

type BootstrapThreadView struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updatedAt"`
	Current   bool   `json:"current"`
}

func normalizeHTTPServerConfig(cfg HTTPServerConfig) HTTPServerConfig {
	cfg.PublicAgentBaseURL = normalizeBaseURL(cfg.PublicAgentBaseURL)
	cfg.PublicSignalingBaseURL = normalizeBaseURL(cfg.PublicSignalingBaseURL)
	if cfg.BootstrapRecentThreadLimit <= 0 {
		cfg.BootstrapRecentThreadLimit = 5
	}
	return cfg
}

func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, "/")
	return value
}

func buildBootstrapResponse(
	adapter WorkspaceAdapter,
	orchestrator *Orchestrator,
	p2pManager *P2PSessionManager,
	cfg HTTPServerConfig,
	agentBaseURL string,
	signalingBaseURL string,
) BootstrapResponse {
	adapterInfo := BasicAdapterRuntimeInfo(adapter)
	if provider, ok := adapter.(AdapterRuntimeInfoProvider); ok {
		adapterInfo = provider.RuntimeInfo()
	}

	currentThreadID, recentThreads := buildBootstrapRecentThreads(orchestrator, cfg.BootstrapRecentThreadLimit)
	return BootstrapResponse{
		AgentBaseURL:     agentBaseURL,
		SignalingBaseURL: signalingBaseURL,
		WorkspaceRoot:    adapterInfo.WorkspaceRoot,
		Adapter: BootstrapAdapterView{
			Name:     adapterInfo.Name,
			Mode:     adapterInfo.Mode,
			Provider: inferProviderName(adapterInfo),
			Ready:    adapterInfo.Ready,
		},
		CurrentThreadID: currentThreadID,
		RecentThreads:   recentThreads,
	}
}

func buildBootstrapRecentThreads(orchestrator *Orchestrator, limit int) (string, []BootstrapThreadView) {
	threads := []ThreadSummary{}
	if orchestrator != nil && orchestrator.ThreadStore() != nil {
		threads = orchestrator.ThreadStore().List()
	}
	if limit > 0 && len(threads) > limit {
		threads = threads[:limit]
	}

	recentThreads := make([]BootstrapThreadView, 0, len(threads))
	currentThreadID := ""
	for i, thread := range threads {
		current := i == 0
		if current {
			currentThreadID = thread.ID
		}
		recentThreads = append(recentThreads, BootstrapThreadView{
			ID:        thread.ID,
			Title:     thread.Title,
			UpdatedAt: thread.UpdatedAt,
			Current:   current,
		})
	}
	return currentThreadID, recentThreads
}

func configuredBootstrapSignalingBaseURL(p2pManager *P2PSessionManager) string {
	if p2pManager == nil {
		return ""
	}
	status := p2pManager.Status()
	if baseURL := normalizeBaseURL(status.SignalingBaseURL); baseURL != "" {
		return baseURL
	}
	return normalizeBaseURL(p2pManager.DefaultSignalingBaseURL())
}

func inferAgentBaseURL(r *http.Request, configured string) string {
	if configured = normalizeBaseURL(configured); configured != "" {
		return configured
	}

	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}

	scheme := forwardedScheme(r)
	return scheme + "://" + host
}

func resolveDiscoveryAgentBaseURL(listenAddr string, publicOverride string) string {
	host := resolveBootstrapHost(publicOverride, listenAddr)
	if host == "" {
		host = "127.0.0.1"
	}

	_, port := splitHostPort(strings.TrimSpace(listenAddr))
	return resolvePublicBaseURL(publicOverride, "http", host, port)
}

func resolveDiscoverySignalingBaseURL(configured string, publicOverride string, fallbackHost string) string {
	configured = normalizeBaseURL(configured)
	publicOverride = normalizeBaseURL(publicOverride)
	base := publicOverride
	if base == "" {
		base = configured
	}
	if base == "" {
		return ""
	}

	port := portFromBaseURL(base)
	return resolvePublicBaseURL(base, "http", fallbackHost, port)
}

func resolveBootstrapHost(configuredBaseURL string, listenAddr string) string {
	if host := hostFromBaseURL(configuredBaseURL); host != "" && !isLoopbackHost(host) && !isWildcardHost(host) {
		return host
	}

	listenHost, _ := splitHostPort(strings.TrimSpace(listenAddr))
	if listenHost != "" && !isLoopbackHost(listenHost) && !isWildcardHost(listenHost) {
		return listenHost
	}

	return pickBootstrapLANHost()
}

func resolvePublicBaseURL(configured string, fallbackScheme string, fallbackHost string, fallbackPort int) string {
	configured = normalizeBaseURL(configured)
	if configured == "" {
		if fallbackHost == "" {
			return ""
		}
		if fallbackPort > 0 {
			return fallbackScheme + "://" + net.JoinHostPort(fallbackHost, strconv.Itoa(fallbackPort))
		}
		return fallbackScheme + "://" + fallbackHost
	}

	u, err := url.Parse(configured)
	if err != nil {
		return configured
	}
	if strings.TrimSpace(u.Scheme) == "" {
		u.Scheme = fallbackScheme
	}

	currentHost := strings.TrimSpace(u.Hostname())
	if currentHost == "" || isLoopbackHost(currentHost) || isWildcardHost(currentHost) {
		replacementHost := strings.TrimSpace(fallbackHost)
		if replacementHost != "" {
			port := u.Port()
			if port == "" && fallbackPort > 0 {
				port = strconv.Itoa(fallbackPort)
			}
			if port != "" {
				u.Host = net.JoinHostPort(replacementHost, port)
			} else {
				u.Host = replacementHost
			}
		}
	}
	return normalizeBaseURL(u.String())
}

func resolveBootstrapSignalingBaseURL(r *http.Request, configured string, publicOverride string) string {
	base := normalizeBaseURL(publicOverride)
	if base == "" {
		base = normalizeBaseURL(configured)
	}
	if base == "" {
		return ""
	}

	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if u.Scheme == "" {
		u.Scheme = forwardedScheme(r)
	}

	requestHost, _ := splitHostPort(strings.TrimSpace(r.Host))
	if requestHost == "" || isLoopbackHost(requestHost) || !isLoopbackHost(u.Hostname()) {
		return u.String()
	}

	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(requestHost, port)
	} else {
		u.Host = requestHost
	}
	return u.String()
}

func forwardedScheme(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		return forwarded
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isWildcardHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	return strings.EqualFold(host, "0.0.0.0") || strings.EqualFold(host, "::")
}

func splitHostPort(value string) (string, int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0
	}

	host, portText, err := net.SplitHostPort(value)
	if err == nil {
		port, _ := strconv.Atoi(portText)
		return strings.Trim(host, "[]"), port
	}

	return strings.Trim(value, "[]"), 0
}

func hostFromBaseURL(value string) string {
	u, err := url.Parse(normalizeBaseURL(value))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(u.Hostname(), "[]"))
}

func portFromBaseURL(value string) int {
	u, err := url.Parse(normalizeBaseURL(value))
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(strings.TrimSpace(u.Port()))
	return port
}

func pickBootstrapLANHost() string {
	candidates := make([]string, 0, 8)
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addresses {
			var ip net.IP
			switch typed := addr.(type) {
			case *net.IPNet:
				ip = typed.IP
			case *net.IPAddr:
				ip = typed.IP
			default:
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			text := ip.String()
			if strings.HasPrefix(text, "169.254.") {
				continue
			}
			candidates = append(candidates, text)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := rankBootstrapLANHost(candidates[i])
		right := rankBootstrapLANHost(candidates[j])
		if left == right {
			return candidates[i] < candidates[j]
		}
		return left < right
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func rankBootstrapLANHost(host string) int {
	switch {
	case strings.HasPrefix(host, "192.168."):
		return 0
	case strings.HasPrefix(host, "10."):
		return 1
	case strings.HasPrefix(host, "172."):
		return 2
	default:
		return 3
	}
}

func inferProviderName(info AdapterRuntimeInfo) string {
	joined := strings.ToLower(strings.Join([]string{info.Mode, info.Name}, " "))
	switch {
	case strings.Contains(joined, "cursor"):
		return "cursor"
	case strings.Contains(joined, "codex"):
		return "codex"
	case strings.Contains(joined, "claude"):
		return "claude_code"
	case strings.Contains(joined, "antigravity"):
		return "antigravity"
	default:
		return "unknown"
	}
}
