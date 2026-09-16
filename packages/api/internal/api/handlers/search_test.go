package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/gin-gonic/gin"
)

// MockSearchService implements search.Service for testing
type MockSearchService struct {
	SearchFunc func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error)
}

func (m *MockSearchService) Search(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, opts)
	}
	return nil, 0, nil
}

// TestSearchHandlerBasic tests basic search handler functionality
func TestSearchHandlerBasic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			return []search.Result{
				{
					Title:       "Test Result 1",
					URL:         "https://example.com/1",
					Description: "Test description 1",
					ID:          "test_1",
					Domain:      "example.com",
					Relevance:   1.0,
				},
			}, 100, nil
		},
	}

	handler := NewSearchHandler(mockService)

	reqBody := SearchRequest{
		Query:  "test",
		Offset: 0,
		Limit:  10,
		Mode:   "balanced",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Search(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Query != reqBody.Query {
		t.Errorf("Expected query %q, got %q", reqBody.Query, resp.Query)
	}

	if len(resp.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(resp.Results))
	}
}

// TestSearchHandlerPagination tests pagination calculation
func TestSearchHandlerPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		offset        int
		limit         int
		totalCount    int
		expectHasMore bool
		expectNextOff int
	}{
		{
			name:          "First page with more",
			offset:        0,
			limit:         10,
			totalCount:    100,
			expectHasMore: true,
			expectNextOff: 10,
		},
		{
			name:          "Last page",
			offset:        90,
			limit:         10,
			totalCount:    100,
			expectHasMore: false,
			expectNextOff: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockSearchService{
				SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
					var results []search.Result
					for i := 0; i < tt.limit; i++ {
						results = append(results, search.Result{
							Title: fmt.Sprintf("Result %d", i),
						})
					}
					return results, tt.totalCount, nil
				},
			}

			handler := NewSearchHandler(mockService)

			reqBody := SearchRequest{
				Query:  "test",
				Offset: tt.offset,
				Limit:  tt.limit,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			handler.Search(c)

			var resp SearchResponse
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp.HasMore != tt.expectHasMore {
				t.Errorf("Expected hasMore %v, got %v", tt.expectHasMore, resp.HasMore)
			}

			if resp.NextOffset != tt.expectNextOff {
				t.Errorf("Expected nextOffset %d, got %d", tt.expectNextOff, resp.NextOffset)
			}
		})
	}
}

// TestSearchHandlerLimitValidation tests limit parameter validation
func TestSearchHandlerLimitValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			var results []search.Result
			for i := 0; i < opts.Limit; i++ {
				results = append(results, search.Result{
					Title: fmt.Sprintf("Result %d", i),
				})
			}
			return results, 1000, nil
		},
	}

	handler := NewSearchHandler(mockService)

	tests := []struct {
		name        string
		limit       int
		expectLimit int
	}{
		{
			name:        "Zero limit defaults to 10",
			limit:       0,
			expectLimit: 10,
		},
		{
			name:        "Limit 100 max",
			limit:       200,
			expectLimit: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := SearchRequest{
				Query:  "test",
				Offset: 0,
				Limit:  tt.limit,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			handler.Search(c)

			if w.Code != http.StatusOK {
				t.Fatalf("Request failed with status %d", w.Code)
			}

			var resp SearchResponse
			json.Unmarshal(w.Body.Bytes(), &resp)

			if len(resp.Results) > tt.expectLimit {
				t.Errorf("Expected max %d results, got %d", tt.expectLimit, len(resp.Results))
			}
		})
	}
}

// TestSearchHandlerOffsetNegative tests negative offset handling
func TestSearchHandlerOffsetNegative(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			if opts.Offset < 0 {
				t.Errorf("Service received negative offset: %d", opts.Offset)
			}
			return []search.Result{}, 0, nil
		},
	}

	handler := NewSearchHandler(mockService)

	reqBody := SearchRequest{
		Query:  "test",
		Offset: -10,
		Limit:  10,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Search(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestSearchHandlerMissingQuery tests missing query parameter
func TestSearchHandlerMissingQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{}
	handler := NewSearchHandler(mockService)

	reqBody := SearchRequest{
		Query:  "",
		Offset: 0,
		Limit:  10,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Search(c)

	if w.Code == http.StatusOK {
		t.Errorf("Expected error for missing query")
	}
}

// TestSearchHandlerErrorHandling tests error handling
func TestSearchHandlerErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			return nil, 0, fmt.Errorf("test error")
		},
	}

	handler := NewSearchHandler(mockService)

	reqBody := SearchRequest{
		Query:  "test",
		Offset: 0,
		Limit:  10,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Search(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// TestSearchHandlerResponseFormat tests response format
func TestSearchHandlerResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			return []search.Result{
				{
					Title:       "Test",
					URL:         "https://example.com",
					Description: "Test",
					ID:          "test",
					Domain:      "example.com",
					Relevance:   1.0,
				},
			}, 50, nil
		},
	}

	handler := NewSearchHandler(mockService)

	reqBody := SearchRequest{
		Query:  "test",
		Offset: 0,
		Limit:  10,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Search(c)

	var resp SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if resp.Query == "" {
		t.Error("Query is empty")
	}
	if resp.Count != 1 {
		t.Errorf("Expected count 1, got %d", resp.Count)
	}
	if resp.NextOffset == 0 {
		t.Error("NextOffset is not set")
	}
}

// TestSearchHandlerCategoryValidation verifies the category enum is validated
// and that valid categories are passed through to the service.
func TestSearchHandlerCategoryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		category   string
		wantStatus int
		wantErr    bool
	}{
		{"general valid", "general", http.StatusOK, false},
		{"news valid", "news", http.StatusOK, false},
		{"code valid", "code", http.StatusOK, false},
		{"invalid returns 400", "invalid", http.StatusBadRequest, true},
		{"company invalid", "company", http.StatusBadRequest, true},
		{"empty allowed", "", http.StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCategory string
			mockService := &MockSearchService{
				SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
					gotCategory = opts.Category
					return []search.Result{{Title: "ok", URL: "https://ok.dev/"}}, 1, nil
				},
			}
			handler := NewSearchHandler(mockService)
			// Use GET with query param to test pass-through.
			url := "/search?query=test"
			if tt.category != "" {
				url += "&category=" + tt.category
			}
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.Search(c)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			if !tt.wantErr && tt.category != "" && gotCategory != tt.category {
				t.Errorf("service Category = %q, want %q", gotCategory, tt.category)
			}
			if tt.wantErr && gotCategory != "" {
				t.Errorf("service should not have been called for invalid category, got %q", gotCategory)
			}
		})
	}
}

// TestSearchHandlerCategoryFromJSON verifies category from POST JSON body.
func TestSearchHandlerCategoryFromJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotCategory string
	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			gotCategory = opts.Category
			return []search.Result{{Title: "ok", URL: "https://ok.dev/"}}, 1, nil
		},
	}
	handler := NewSearchHandler(mockService)
	body, _ := json.Marshal(SearchRequest{Query: "test", Category: "news"})
	req := httptest.NewRequest("POST", "/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.Search(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if gotCategory != "news" {
		t.Errorf("Category from JSON = %q, want news", gotCategory)
	}
}

// TestSearchHandlerCategoryQueryOverridesJSON verifies query param overrides JSON body.
func TestSearchHandlerCategoryQueryOverridesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotCategory string
	mockService := &MockSearchService{
		SearchFunc: func(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
			gotCategory = opts.Category
			return []search.Result{{Title: "ok", URL: "https://ok.dev/"}}, 1, nil
		},
	}
	handler := NewSearchHandler(mockService)
	body, _ := json.Marshal(SearchRequest{Query: "test", Category: "general"})
	req := httptest.NewRequest("POST", "/search?category=code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.Search(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotCategory != "code" {
		t.Errorf("Category = %q, want code (query should override JSON)", gotCategory)
	}
}
