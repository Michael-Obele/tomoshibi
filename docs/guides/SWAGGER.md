# Interactive API Documentation (Swagger)

Tomoshibi provides interactive API documentation out-of-the-box using Swagger (via Swaggo). This allows you to view the API schema and test endpoints directly from your browser.

## Accessing the Swagger UI

1. Start the Tomoshibi API server:
   ```bash
   go run cmd/api/main.go
   ```
2. Open your browser and navigate to the Swagger UI endpoint:
   ```http
   http://localhost:8080/swagger/index.html
   ```
   *(Adjust the port if you have configured Tomoshibi to run on a port other than `8080`)*.

The Swagger interface allows you to view all available endpoints, required parameters, and response types. You can even execute actual test requests (e.g., triggering a `/v1/scrape`) directly against your local running instance.

---

## Updating the Swagger Documentation

The Swagger documentation is generated dynamically when the API server is started in `debug` mode. 

By default, the server runs in `debug` mode unless `SERVER_MODE=release` is set. When running in debug mode via `go run cmd/api/main.go`, the server will automatically find the Swag CLI and run the following command to regenerate the documentation before listening for requests:

```bash
go run github.com/swaggo/swag/cmd/swag@latest init -d ./cmd/api,./internal/api/handlers,./internal/domain -g main.go -o internal/api/docs --parseDependency --parseInternal
```

### Adding Annotations
If you add a new endpoint or change an existing request/response structure, you just need to add the declarative `// @...` comments above your handler functions and restart the server.

For example, in `internal/api/handlers/scrape.go`:
```go
// @Summary      Scrape a webpage
// @Description  Scrapes a given URL and returns its markdown content.
// @Tags         scrape
// @Accept       json
// @Produce      json
// @Param        url    query     string  false  "The URL to scrape"
// @Success      200    {object}  domain.ScrapeResult
// @Router       /scrape [post]
func (h *ScrapeHandler) Scrape(c *gin.Context) {
    // ...
}
```

Once you restart the server, the changes will be automatically picked up and visible in the browser UI.
