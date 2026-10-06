package middleware

import (
	"log/slog"

	"github.com/Michael-Obele/tomoshibi/internal/search/envctx"
	"github.com/gin-gonic/gin"
)

// RequestEnv parses the optional X-Tomoshi-Env header and attaches the
// validated overrides to the request context, where the search engines
// resolve them (envctx.Get) instead of the process env. This is how the MCP
// forwards search API keys from its own env to the backend, so operators
// configure keys in one place.
//
// Failure policy: a malformed header is IGNORED, never a 4xx — the header
// is an optional enhancement, and a proxy or old client mangling it must
// not break the request. The reason is logged (values never are) so
// misconfiguration stays visible.
func RequestEnv(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader(envctx.Header)
		if raw == "" {
			c.Next()
			return
		}
		ov, err := envctx.Parse(raw)
		if err != nil {
			if logger != nil {
				logger.Warn("ignoring malformed env header", "header", envctx.Header, "reason", err.Error())
			}
			c.Next()
			return
		}
		if len(ov) > 0 {
			c.Request = c.Request.WithContext(envctx.With(c.Request.Context(), ov))
		}
		c.Next()
	}
}
