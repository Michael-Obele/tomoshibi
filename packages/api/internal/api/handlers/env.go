package handlers

import (
	"net/http"

	"github.com/Michael-Obele/tomoshibi/internal/search/envctx"
	"github.com/gin-gonic/gin"
)

// Env godoc
// @Summary      List env var names a client may supply per request
// @Description  Returns the allowlisted env var names accepted in the X-Tomoshi-Env request header (JSON object of NAME to value). The MCP calls this once and forwards the matching keys from its own env, so search API keys are configured in a single place. Values are never echoed back.
// @Tags         search
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Router       /env [get]
func Env(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"header": envctx.Header,
		"format": "JSON object of env var name to value",
		"env":    envctx.Allowed(),
	})
}
