package handler

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/analytics"
)

type AppConfig struct {
	CdnDomain string `json:"cdn_domain"`
	// CdnSigned tells clients that the CDN domain above serves PRIVATE
	// content through time-bounded signed URLs (CloudFront signing is
	// enabled). When true, a raw storage URL on the CDN domain is NOT
	// publicly fetchable — renderers must not pick it as a native
	// <img>/<video> source and should fall back to the per-attachment
	// API endpoint or a freshly signed download_url instead (MUL-3254).
	// Omitted when false so older clients see the previous shape.
	CdnSigned bool `json:"cdn_signed,omitempty"`
	// Public auth config consumed by the web app at runtime so self-hosted
	// deployments do not need to rebuild the frontend image when operators
	// toggle signup or wire Google OAuth.
	AllowSignup    bool   `json:"allow_signup"`
	GoogleClientID string `json:"google_client_id,omitempty"`
	// WorkspaceCreationDisabled mirrors the server-side
	// DISABLE_WORKSPACE_CREATION env var so the UI can hide every
	// "Create workspace" affordance on self-hosted instances. Omitted
	// from the JSON when false to keep responses identical to the
	// previous shape for the common managed-cloud case (#3433).
	WorkspaceCreationDisabled bool `json:"workspace_creation_disabled,omitempty"`
	// Public daemon setup config consumed by the web app at runtime so
	// self-hosted instances can show `multica setup self-host` commands
	// with the operator's own domains instead of Multica Cloud defaults.
	DaemonServerURL string `json:"daemon_server_url,omitempty"`
	DaemonAppURL    string `json:"daemon_app_url,omitempty"`

	// PostHog public config for the frontend. The key is the same Project
	// API Key the backend uses; returning it here (instead of baking it
	// into the frontend bundle via NEXT_PUBLIC_*) means self-hosted
	// instances — whose server returns an empty key — automatically
	// disable frontend event shipping too.
	PosthogKey           string `json:"posthog_key"`
	PosthogHost          string `json:"posthog_host"`
	AnalyticsEnvironment string `json:"analytics_environment"`
}

// GetConfig is mounted on the public (unauthenticated) route group because
// the web app calls it before login to decide whether to render the Google
// sign-in button and signup UI. Only add fields here that are safe to expose
// to anonymous callers — never user- or tenant-scoped data.
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config := AppConfig{
		AllowSignup:               os.Getenv("ALLOW_SIGNUP") != "false",
		GoogleClientID:            os.Getenv("GOOGLE_CLIENT_ID"),
		WorkspaceCreationDisabled: os.Getenv("DISABLE_WORKSPACE_CREATION") == "true",
	}
	if h.Storage != nil {
		config.CdnDomain = h.Storage.CdnDomain()
	}
	config.CdnSigned = h.CFSigner != nil
	config.DaemonServerURL, config.DaemonAppURL = daemonSetupURLsFromEnv()

	// Re-read from env on every request so operators can rotate keys via
	// secret refresh without a server restart.
	if v := os.Getenv("ANALYTICS_DISABLED"); v != "true" && v != "1" {
		config.PosthogKey = os.Getenv("POSTHOG_API_KEY")
		config.PosthogHost = os.Getenv("POSTHOG_HOST")
		config.AnalyticsEnvironment = analytics.EnvironmentFromEnv()
		if config.PosthogHost == "" && config.PosthogKey != "" {
			config.PosthogHost = "https://us.i.posthog.com"
		}
	}

	writeJSON(w, http.StatusOK, config)
}

func daemonSetupURLsFromEnv() (string, string) {
	serverURL := normalizePublicURL(os.Getenv("MULTICA_PUBLIC_URL"))
	appURL := normalizePublicURL(os.Getenv("MULTICA_APP_URL"))
	if appURL == "" {
		appURL = normalizePublicURL(os.Getenv("FRONTEND_ORIGIN"))
	}
	if appURL == "" {
		return "", ""
	}

	if serverURL == "" {
		serverURL = inferDaemonServerURL(appURL)
	}
	if isOfficialCloudDaemonConfig(appURL) {
		return "", ""
	}
	return serverURL, appURL
}

// inferDaemonServerURL derives the backend server URL the daemon should talk
// to when MULTICA_PUBLIC_URL is not set. The historical behavior assumed a
// reverse-proxy / same-origin topology and reused the app URL verbatim. That
// breaks the common self-hosted split-port topology where the frontend is
// served on a non-default port (e.g. :3001) while the Go backend listens on
// its own PORT (default 8080): the daemon command would point at the frontend
// port and never reach /health or the WebSocket proxy. When the app URL
// carries an explicit non-default port we therefore rebuild the URL with the
// backend's listen port instead.
//
// Heuristic boundaries:
//   - No explicit port, or the scheme's default port (http :80 / https :443)
//     → same-origin; reuse the app URL verbatim (behavior unchanged).
//   - Explicit non-default port → swap in the backend port from PORT env
//     (the same single source of truth used in cmd/server/main.go), default
//     8080. PORT must be a non-empty pure number in 1–65535 or it falls back
//     to 8080. The scheme and host are preserved; the host is reassembled
//     with net.JoinHostPort so IPv6 literals stay safe.
//   - Parse failure (missing scheme/host) → fall back to the app URL verbatim
//     so we are never worse than today.
//
// This is a heuristic for the common self-hosted topology. Non-standard setups
// (PaaS-random PORT, custom reverse-proxy ports such as :8443) may infer
// incorrectly; operators escape by setting MULTICA_PUBLIC_URL explicitly.
func inferDaemonServerURL(appURL string) string {
	u, err := url.Parse(appURL)
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return appURL
	}
	port := u.Port()
	if port == "" {
		return appURL
	}
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		return appURL
	}
	u.Host = net.JoinHostPort(u.Hostname(), backendListenPort())
	return u.String()
}

// backendListenPort returns the port the Go backend listens on, mirroring the
// resolution in cmd/server/main.go: PORT is the single source of truth and
// defaults to 8080 when unset or invalid. A non-numeric or out-of-range value
// falls back to the default rather than corrupting the constructed URL.
func backendListenPort() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "8080"
	}
	return port
}

func normalizePublicURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// isOfficialCloudDaemonConfig reports whether this deployment is the official
// Multica Cloud, identified by its frontend host alone (multica.ai /
// app.multica.ai). The daemon setup for the managed cloud is always
// `multica setup` (which hardcodes api.multica.ai), so the per-deployment URLs
// must be omitted from /api/config even when MULTICA_PUBLIC_URL is unset or
// misconfigured. Previously this also required serverURL==api.multica.ai, so a
// cloud deployment that forgot MULTICA_PUBLIC_URL fell through and emitted a
// `setup self-host --server-url https://multica.ai` command — pointing the
// daemon's backend at the frontend (no /health, no WebSocket proxy).
func isOfficialCloudDaemonConfig(appURL string) bool {
	return urlHostEquals(appURL, "multica.ai") || urlHostEquals(appURL, "app.multica.ai")
}

func urlHostEquals(raw, want string) bool {
	host := canonicalURLHost(raw)
	if host == "" {
		return false
	}
	want = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(want)), ".")
	return host == want
}

func canonicalURLHost(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" && !strings.Contains(raw, "://") {
		u, err = url.Parse("https://" + raw)
		if err != nil {
			return ""
		}
		host = u.Hostname()
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
