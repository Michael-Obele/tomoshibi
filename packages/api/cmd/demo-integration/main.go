package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8080/v1"

type SearchRequest struct {
	Query string `json:"query"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ScrapeRequest struct {
	URL string `json:"url"`
}

type ScrapeResponse struct {
	URL      string `json:"url"`
	Metadata struct {
		Title string `json:"title"`
	} `json:"metadata"`
	Markdown string `json:"markdown"`
}

func main() {
	query := "golang latest features"
	fmt.Printf("1. Searching for: %s\n", query)

	// 1. Search
	searchRes, err := performSearch(query)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	fmt.Printf("Found %d results. Scraping top 3...\n", len(searchRes.Results))

	// 2. Scrape top 3
	count := 0
	for _, result := range searchRes.Results {
		if count >= 3 {
			break
		}
		fmt.Printf("\n--- Processing Result %d ---\n", count+1)
		fmt.Printf("Title: %s\nURL: %s\n", result.Title, result.URL)

		scrapeRes, err := performScrape(result.URL)
		if err != nil {
			fmt.Printf("  ❌ Scrape failed: %v\n", err)
		} else {
			fmt.Printf("  ✅ Scraped successfully\n")
			snippet := scrapeRes.Markdown
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			fmt.Printf("  Preview: %s\n", snippet)
		}
		count++

		// Be nice to the API and target servers
		time.Sleep(1 * time.Second)
	}

	// 3. Test Smart Scraper Specifics
	fmt.Println("\n--- Testing Smart Scraper Heuristics ---")
	spaURL := "https://react.dev"
	fmt.Printf("Targeting SPA: %s\n", spaURL)

	// Explicitly ask for smart mode (though it's default)
	scrapeRes, err := performScrape(spaURL)
	if err != nil {
		fmt.Printf("  ❌ Scrape failed: %v\n", err)
	} else {
		fmt.Printf("  ✅ Scraped successfully\n")
		// Check metadata or content to see if it worked
		snippet := scrapeRes.Markdown
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		fmt.Printf("  Preview: %s\n", snippet)
	}
}

func performSearch(query string) (*SearchResponse, error) {
	reqBody, _ := json.Marshal(SearchRequest{Query: query})
	resp, err := http.Post(baseURL+"/search", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var res SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func performScrape(urlStr string) (*ScrapeResponse, error) {
	reqBody, _ := json.Marshal(ScrapeRequest{URL: urlStr})
	resp, err := http.Post(baseURL+"/scrape", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var res ScrapeResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}
