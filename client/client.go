package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	docmodel "github.com/noematic-eu/ai-rag-agent/model"
)

// Client représente le client pour interagir avec l'API
type Client struct {
	baseURL string
}

// NewClient crée un nouveau client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// IngestDocument envoie un document à l'API pour ingestion
func (c *Client) IngestDocument(doc docmodel.LegalDocument) error {
	jsonData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("erreur lors de la sérialisation du document : %v", err)
	}

	resp, err := http.Post(c.baseURL+"/ingest", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("erreur lors de l'envoi de la requête : %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erreur du serveur (status %d) : %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) Finalize() error {
	resp, err := http.Post(c.baseURL+"/finalize", "application/json", nil)
	if err != nil {
		return fmt.Errorf("erreur lors de l'envoi de la requête : %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusAccepted:
		return c.waitForLexicalRebuild()
	default:
		return fmt.Errorf("erreur du serveur (status %d) : %s", resp.StatusCode, string(body))
	}
}

func (c *Client) waitForLexicalRebuild() error {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := http.Get(c.baseURL + "/stats")
		if err != nil {
			return fmt.Errorf("poll stats: %w", err)
		}
		var stats struct {
			LexicalIndex struct {
				Rebuilding bool `json:"rebuilding"`
			} `json:"lexical_index"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&stats)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode stats: %w", decodeErr)
		}
		if !stats.LexicalIndex.Rebuilding {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("lexical rebuild timed out after 10 minutes")
}

// Search effectue une recherche et traite la réponse en streaming
func (c *Client) Search(query string) error {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Question: %s\n", query)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Réponse:")

	encodedQuery := url.QueryEscape(query)
	resp, err := http.Get(c.baseURL + "/search?q=" + encodedQuery)
	if err != nil {
		return fmt.Errorf("erreur lors de la requête : %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erreur du serveur (status %d) : %s", resp.StatusCode, string(body))
	}

	// Ensure UTF-8 encoding
	resp.Header.Set("Content-Type", "text/event-stream; charset=utf-8")

	reader := bufio.NewReader(resp.Body)
	var metadata map[string]interface{}
	var eventType string
	var currentData strings.Builder
	var completeResponse map[string]interface{}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("erreur lors de la lecture : %v", err)
		}

		if strings.HasPrefix(line, "event:") {
			// Process any accumulated data before changing event type
			if currentData.Len() > 0 {
				data := currentData.String()
				currentData.Reset()
				if err := processData(eventType, data, &metadata, &completeResponse); err != nil {
					fmt.Printf("Erreur lors du traitement des données: %v\n", err)
				}
			}
			eventType = strings.TrimPrefix(line, "event:")
			eventType = strings.TrimSpace(eventType)
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if data == "" {
				continue
			}
			currentData.WriteString(data)
		}
	}

	// Process any remaining data
	if currentData.Len() > 0 {
		data := currentData.String()
		if err := processData(eventType, data, &metadata, &completeResponse); err != nil {
			fmt.Printf("Erreur lors du traitement des données: %v\n", err)
		}
	}

	// If we have a complete response, replace the terminal output
	if completeResponse != nil {
		// Clear the terminal
		fmt.Print("\033[H\033[2J")
		// Move cursor to top
		fmt.Print("\033[H")

		// Print the complete response
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Printf("Question: %s\n", query)
		fmt.Println(strings.Repeat("=", 80))
		if prompt, ok := metadata["prompt"].(string); ok {
			fmt.Println("Prompt:")
			fmt.Println(prompt)
		}
		if response, ok := completeResponse["response"].(string); ok {
			fmt.Println("Réponse:")
			fmt.Println(response)
		} else {
			fmt.Println("Pas de réponse")
		}
		fmt.Println("\n" + strings.Repeat("=", 80))
	}

	return nil
}

// processData traite les données reçues selon leur type d'événement
func processData(eventType, data string, metadata *map[string]interface{}, completeResponse *map[string]interface{}) error {
	switch eventType {
	case "message":
		if data != "" {
			fmt.Print(data[:len(data)-1])
			_ = os.Stdout.Sync() // Ensure immediate output
		}
	case "metadata":
		if strings.HasPrefix(data, "{") {
			if err := json.Unmarshal([]byte(data), metadata); err == nil {
				if model, ok := (*metadata)["model"].(string); ok {
					fmt.Println("\n" + strings.Repeat("-", 80))
					fmt.Printf("Modèle Ollama: %s\n", model)
					if prompt, ok := (*metadata)["prompt"].(string); ok {
						fmt.Println("\nPrompt envoyé à Ollama:")
						fmt.Println(prompt)
					}
					fmt.Println(strings.Repeat("-", 80) + "\n")
				}
			} else {
				return fmt.Errorf("erreur lors de la désérialisation du metadata : %v", err)
			}
		}
	case "complete":
		if strings.HasPrefix(data, "{") {
			if err := json.Unmarshal([]byte(data), completeResponse); err != nil {
				return fmt.Errorf("erreur lors de la désérialisation de la réponse complète : %v", err)
			}
		}
	default:
		fmt.Printf("Type d'événement non reconnu: %s\n", eventType)
	}
	return nil
}
