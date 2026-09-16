package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Michael-Obele/tomoshibi/internal/api/handlers"
	"github.com/Michael-Obele/tomoshibi/internal/search"
	"github.com/gin-gonic/gin"
)

// MockSearchService for testing
type MockSearchService struct {
	results []search.Result
	total   int
	err     error
}

func (m *MockSearchService) Search(ctx context.Context, opts search.SearchOptions) ([]search.Result, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}

	// Return only the requested slice based on offset
	start := opts.Offset
	end := opts.Offset + opts.Limit
	if end > len(m.results) {
		end = len(m.results)
	}
	if start > len(m.results) {
		start = len(m.results)
	}

	return m.results[start:end], m.total, nil
}

// setupTestServer creates a test server with the search handler
func setupTestServer(service search.Service) *httptest.Server {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	searchHandler := handlers.NewSearchHandler(service)
	router.POST("/v1/search", searchHandler.Search)

	return httptest.NewServer(router)
}

// TestIntegrationSearchPagination tests full pagination flow
func TestIntegrationSearchPagination(t *testing.T) {
	// Create mock results
	var results []search.Result
	for i := 0; i < 100; i++ {
		results = append(results, search.Result{
			Title:       fmt.Sprintf("Result %d", i),
			URL:         fmt.Sprintf("https://example.com/result-%d", i),
			Description: fmt.Sprintf("Description for result %d", i),
			ID:          fmt.Sprintf("id_%d", i),
			Domain:      "example.com",
			Relevance:   1.0 - float64(i%10)*0.1,
		})
	}

	mockService := &MockSearchService{
		results: results,
		total:   100,
	}

	server := setupTestServer(mockService)
	defer server.Close()

	// Test paginating through all results
	var allResults []search.Result
	offset := 0
	limit := 10
	pageNum := 0

	for {
		pageNum++

		// Make request
		reqBody := map[string]interface{}{
			"query":  "test",
			"offset": offset,
			"limit":  limit,
		}

		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(
			server.URL+"/v1/search",
			"application/json",
			bytes.NewReader(body),
		)

		if err != nil {
			t.Fatalf("Page %d: Request failed: %v", pageNum, err)
		}

		var searchResp handlers.SearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			t.Fatalf("Page %d: Failed to decode response: %v", pageNum, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Page %d: Expected status 200, got %d", pageNum, resp.StatusCode)
		}

		t.Logf("Page %d: Got %d results, hasMore=%v, nextOffset=%d",
			pageNum, searchResp.Count, searchResp.HasMore, searchResp.NextOffset)

		// Verify response
		if searchResp.Count == 0 && searchResp.HasMore {
			t.Errorf("Page %d: Got 0 results but hasMore=true", pageNum)
		}

		if searchResp.Count > limit {
			t.Errorf("Page %d: Got %d results, expected max %d", pageNum, searchResp.Count, limit)
		}

		allResults = append(allResults, searchResp.Results...)

		if !searchResp.HasMore {
			break
		}

		offset = searchResp.NextOffset

		if pageNum > 20 {
			t.Fatal("Too many pages, stopping to prevent infinite loop")
		}
	}

	if len(allResults) != 100 {
		t.Errorf("Expected 100 total results, got %d", len(allResults))
	}
}

// TestIntegrationDifferentQueries tests different search queries with pagination
func TestIntegrationDifferentQueries(t *testing.T) {
	queries := []string{
		"golang",
		"rust programming",
		"javascript react",
		"python data science",
		"kubernetes docker",
	}

	for _, query := range queries {
		t.Run(fmt.Sprintf("Query:%s", query), func(t *testing.T) {
			// Create mock results specific to query
			var results []search.Result
			numResults := 50
			for i := 0; i < numResults; i++ {
				results = append(results, search.Result{
					Title:       fmt.Sprintf("%s - Result %d", query, i),
					URL:         fmt.Sprintf("https://example.com/%s-%d", query, i),
					Description: fmt.Sprintf("Results for query: %s", query),
					ID:          fmt.Sprintf("id_%s_%d", query, i),
					Domain:      "example.com",
					Relevance:   1.0 - float64(i%5)*0.2,
				})
			}

			mockService := &MockSearchService{
				results: results,
				total:   numResults,
			}

			server := setupTestServer(mockService)
			defer server.Close()

			// First page
			reqBody := map[string]interface{}{
				"query":  query,
				"offset": 0,
				"limit":  10,
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(
				server.URL+"/v1/search",
				"application/json",
				bytes.NewReader(body),
			)

			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			var searchResp handlers.SearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			resp.Body.Close()

			if searchResp.Query != query {
				t.Errorf("Query mismatch: expected %q, got %q", query, searchResp.Query)
			}

			if searchResp.Count == 0 {
				t.Errorf("Expected results for query %q", query)
			}
		})
	}
}

// TestIntegrationResponseStructure tests response structure for all fields
func TestIntegrationResponseStructure(t *testing.T) {
	var results []search.Result
	for i := 0; i < 30; i++ {
		results = append(results, search.Result{
			Title:       fmt.Sprintf("Result %d", i),
			URL:         fmt.Sprintf("https://example.com/%d", i),
			Description: "Test result",
			ID:          fmt.Sprintf("id_%d", i),
			Domain:      "example.com",
			Relevance:   0.95,
		})
	}

	mockService := &MockSearchService{
		results: results,
		total:   30,
	}

	server := setupTestServer(mockService)
	defer server.Close()

	reqBody := map[string]interface{}{
		"query":  "test",
		"offset": 0,
		"limit":  10,
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(
		server.URL+"/v1/search",
		"application/json",
		bytes.NewReader(body),
	)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	var searchResp handlers.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	resp.Body.Close()

	// Check all required fields
	if searchResp.Query == "" {
		t.Error("Query field is empty")
	}

	if searchResp.Count == 0 {
		t.Error("Count field is 0")
	}

	if len(searchResp.Results) == 0 {
		t.Error("Results array is empty")
	}

	if searchResp.NextOffset == 0 && searchResp.HasMore {
		t.Error("NextOffset is 0 but HasMore is true")
	}

	// Check result structure
	for i, result := range searchResp.Results {
		if result.Title == "" {
			t.Errorf("Result %d: Title is empty", i)
		}
		if result.URL == "" {
			t.Errorf("Result %d: URL is empty", i)
		}
		if result.Domain == "" {
			t.Errorf("Result %d: Domain is empty", i)
		}
		if result.ID == "" {
			t.Errorf("Result %d: ID is empty", i)
		}
	}
}

// TestIntegrationLimitCapping tests limit parameter capping at 100
func TestIntegrationLimitCapping(t *testing.T) {
	var results []search.Result
	for i := 0; i < 200; i++ {
		results = append(results, search.Result{
			Title:       fmt.Sprintf("Result %d", i),
			URL:         fmt.Sprintf("https://example.com/%d", i),
			Description: "Test",
			ID:          fmt.Sprintf("id_%d", i),
			Domain:      "example.com",
			Relevance:   1.0,
		})
	}

	mockService := &MockSearchService{
		results: results,
		total:   200,
	}

	server := setupTestServer(mockService)
	defer server.Close()

	tests := []struct {
		requestLimit int
		expectMax    int
	}{
		{5, 5},
		{50, 50},
		{100, 100},
		{150, 100},
		{200, 100},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("RequestLimit:%d", tt.requestLimit), func(t *testing.T) {
			reqBody := map[string]interface{}{
				"query":  "test",
				"offset": 0,
				"limit":  tt.requestLimit,
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(
				server.URL+"/v1/search",
				"application/json",
				bytes.NewReader(body),
			)

			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			var searchResp handlers.SearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			resp.Body.Close()

			if searchResp.Count > tt.expectMax {
				t.Errorf("Expected max %d results, got %d", tt.expectMax, searchResp.Count)
			}
		})
	}
}

// TestIntegrationOffsetCalculation tests nextOffset calculation
func TestIntegrationOffsetCalculation(t *testing.T) {
	var results []search.Result
	for i := 0; i < 100; i++ {
		results = append(results, search.Result{
			Title: fmt.Sprintf("Result %d", i),
			URL:   fmt.Sprintf("https://example.com/%d", i),
		})
	}

	mockService := &MockSearchService{
		results: results,
		total:   100,
	}

	server := setupTestServer(mockService)
	defer server.Close()

	tests := []struct {
		offset        int
		limit         int
		expectHasMore bool
		expectNext    int
	}{
		{0, 10, true, 10},
		{10, 10, true, 20},
		{20, 20, true, 40},
		{80, 10, true, 90},
		{90, 10, false, 100},
		{95, 5, false, 100},
		{100, 10, false, 110},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Offset:%d/Limit:%d", tt.offset, tt.limit), func(t *testing.T) {
			reqBody := map[string]interface{}{
				"query":  "test",
				"offset": tt.offset,
				"limit":  tt.limit,
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(
				server.URL+"/v1/search",
				"application/json",
				bytes.NewReader(body),
			)

			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			var searchResp handlers.SearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			resp.Body.Close()

			if searchResp.HasMore != tt.expectHasMore {
				t.Errorf("Expected hasMore=%v, got %v", tt.expectHasMore, searchResp.HasMore)
			}

			if searchResp.NextOffset != tt.expectNext {
				t.Errorf("Expected nextOffset=%d, got %d", tt.expectNext, searchResp.NextOffset)
			}
		})
	}
}
