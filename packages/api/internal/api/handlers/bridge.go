package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Michael-Obele/tomoshibi/internal/bridge"
	"github.com/gin-gonic/gin"
)

// BridgeHandler exposes the browser bridge's pairing state over HTTP so a
// client that is not on this machine (the MCP) can read and fix it without the
// CLI. The WebSocket still lives outside /v1; this is only the admin surface.
type BridgeHandler struct {
	server *bridge.Server
}

// NewBridgeHandler builds the handler. A nil server (bridge disabled) is not
// an error: status reports enabled=false and mutations answer 503.
func NewBridgeHandler(server *bridge.Server) *BridgeHandler {
	return &BridgeHandler{server: server}
}

// BridgeDevice is one paired browser, minus its token hash.
type BridgeDevice struct {
	Name     string `json:"name"`
	PairedAt string `json:"paired_at"`
	LastSeen string `json:"last_seen,omitempty"`
	// TokenHint is the first few hex chars of the stored hash: enough to tell
	// two rows apart, not enough to be a credential.
	TokenHint string `json:"token_hint,omitempty"`
}

// BridgeStatusView is the GET /v1/bridge payload.
type BridgeStatusView struct {
	Enabled        bool           `json:"enabled"`
	Port           string         `json:"port,omitempty"`
	Paired         []BridgeDevice `json:"paired"`
	AllowedOrigins []string       `json:"allowed_origins"`
	PairingCode    string         `json:"pairing_code,omitempty"`
	Hint           string         `json:"hint,omitempty"`
}

// BridgeHostsRequest is the body for allow/deny.
type BridgeHostsRequest struct {
	Hosts []string `json:"hosts"`
}

// BridgeUnpairRequest is the body for unpair; an empty name means "all".
type BridgeUnpairRequest struct {
	Name string `json:"name"`
}

// Status godoc
// @Summary      Browser bridge status
// @Description  Reports whether the bridge is enabled, its port, the paired browsers, and the recipe-free origin allowlist. Pass ?code=1 to include the current 6-digit pairing code.
// @Tags         bridge
// @Produce      json
// @Param        code  query     string  false  "1 includes the pairing code"
// @Success      200   {object}  BridgeStatusView
// @Router       /bridge [get]
func (h *BridgeHandler) Status(c *gin.Context) {
	if h.server == nil {
		c.JSON(http.StatusOK, BridgeStatusView{
			Enabled:        false,
			Paired:         []BridgeDevice{},
			AllowedOrigins: []string{},
			Hint:           "bridge is disabled on this API (no writable state dir); nothing to pair",
		})
		return
	}
	st := h.server.State()
	view := BridgeStatusView{
		Enabled:        true,
		Port:           st.Port(),
		Paired:         bridgeDevices(st.Devices()),
		AllowedOrigins: st.AllowedOrigins(),
	}
	if includeCode, _ := strconv.ParseBool(c.Query("code")); includeCode {
		view.PairingCode = h.server.Code()
	}
	c.JSON(http.StatusOK, view)
}

// Allow godoc
// @Summary      Allow origins on the browser bridge
// @Description  Adds host patterns (example.com or *.example.com) to the recipe-free bridge allowlist.
// @Tags         bridge
// @Accept       json
// @Produce      json
// @Param        body  body      BridgeHostsRequest  true  "Hosts to allow"
// @Success      200   {object}  map[string]interface{}
// @Router       /bridge/allow [post]
func (h *BridgeHandler) Allow(c *gin.Context) { h.mutate(c, true) }

// Deny godoc
// @Summary      Remove origins from the browser bridge allowlist
// @Description  Removes host patterns from the recipe-free bridge allowlist.
// @Tags         bridge
// @Accept       json
// @Produce      json
// @Param        body  body      BridgeHostsRequest  true  "Hosts to deny"
// @Success      200   {object}  map[string]interface{}
// @Router       /bridge/deny [post]
func (h *BridgeHandler) Deny(c *gin.Context) { h.mutate(c, false) }

// Unpair godoc
// @Summary      Revoke paired browsers
// @Description  Removes one named browser, or every browser when name is empty or "all".
// @Tags         bridge
// @Accept       json
// @Produce      json
// @Param        body  body      BridgeUnpairRequest  false  "Device to revoke"
// @Success      200   {object}  map[string]interface{}
// @Router       /bridge/unpair [post]
func (h *BridgeHandler) Unpair(c *gin.Context) {
	if h.server == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bridge is disabled on this API"})
		return
	}
	var req BridgeUnpairRequest
	// An empty body means "revoke everything", so a bind error is not fatal.
	_ = c.ShouldBindJSON(&req)
	st := h.server.State()
	removed := st.Unpair(req.Name)
	c.JSON(http.StatusOK, gin.H{"removed": removed, "paired": bridgeDevices(st.Devices())})
}

func (h *BridgeHandler) mutate(c *gin.Context, add bool) {
	if h.server == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bridge is disabled on this API"})
		return
	}
	var req BridgeHostsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Hosts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": `hosts is required (e.g. ["example.com"])`})
		return
	}
	hosts := make([]string, 0, len(req.Hosts))
	for _, host := range req.Hosts {
		if strings.TrimSpace(host) != "" {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": `hosts is required (e.g. ["example.com"])`})
		return
	}
	st := h.server.State()
	added := []string{}
	already := []string{}
	removed := []string{}
	invalid := []string{}
	for _, host := range hosts {
		pattern := bridge.NormalizeHostPattern(host)
		if add {
			ok, err := st.Allow(host)
			switch {
			case err != nil:
				invalid = append(invalid, host)
			case ok:
				added = append(added, pattern)
			default:
				already = append(already, pattern)
			}
			continue
		}
		ok, err := st.Deny(host)
		switch {
		case err != nil:
			invalid = append(invalid, host)
		case ok:
			removed = append(removed, pattern)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"added":           added,
		"already":         already,
		"removed":         removed,
		"invalid":         invalid,
		"allowed_origins": st.AllowedOrigins(),
	})
}

func bridgeDevices(devices []bridge.Device) []BridgeDevice {
	out := make([]BridgeDevice, 0, len(devices))
	for _, d := range devices {
		view := BridgeDevice{
			Name:     d.Name,
			PairedAt: d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if !d.LastSeen.IsZero() {
			view.LastSeen = d.LastSeen.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		if len(d.TokenSHA) >= 8 {
			view.TokenHint = d.TokenSHA[:8]
		}
		out = append(out, view)
	}
	return out
}
