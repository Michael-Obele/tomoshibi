# tomoshi (灯 — lamp light)

Self-hosted web search & scraping MCP for AI agents. One binary, $5/mo.

- **npm:** `tomoshi` (primary) + `tomoshibi` alias
- **MCP:** 3 tools — `cinder_extract`, `cinder_discover`, `cinder_monitor`

## Install

```json
{
  "mcpServers": {
    "tomoshi": {
      "command": "npx",
      "args": ["-y", "tomoshi"],
      "env": {
        "TOMOSHIBI_API_URL": "http://localhost:7431"
      }
    }
  }
}
```

Legacy `CINDER_API_URL` still works.
