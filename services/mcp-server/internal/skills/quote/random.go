package quote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ThirdEyeQuote represents the exact JSON structure returned by your API
type ThirdEyeQuote struct {
	ID          string `json:"id"`
	Attribution string `json:"attribution"`
	Quote       string `json:"quote"`
}

// FetchRandomQuote makes an HTTP GET request to the ThirdEye API
func FetchRandomQuote(ctx context.Context) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second, // Always set timeouts for external calls!
	}

	// Pointing directly to your custom endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.thirdeye.live/quote", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned non-200 status: %d", resp.StatusCode)
	}

	// Your API returns an array of quote objects, so we decode into a slice
	var quotes []ThirdEyeQuote
	if err := json.NewDecoder(resp.Body).Decode(&quotes); err != nil {
		return "", fmt.Errorf("failed to parse json response: %w", err)
	}

	if len(quotes) == 0 {
		return "", fmt.Errorf("api returned an empty list")
	}

	// Grab the first object from the array
	q := quotes[0]

	// Clean up any stray newline characters in the quote string (like the \n you noticed)
	cleanQuote := strings.TrimSpace(q.Quote)

	// Format exactly as requested: "quote -attribution"
	return fmt.Sprintf("%s -%s", cleanQuote, q.Attribution), nil
}
