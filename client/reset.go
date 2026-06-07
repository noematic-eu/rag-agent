package main

import (
	"fmt"
	"io"
	"net/http"
)

func (c *Client) Reset() error {
	resp, err := http.Post(c.baseURL+"/reset", "application/json", nil)
	if err != nil {
		return fmt.Errorf("reset request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reset failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
