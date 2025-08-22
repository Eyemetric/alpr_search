package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func NewClient(base string) *Client {
	return &Client{
		base: base,
		http: &http.Client{},
	}
}

// helper
func getEnv(key string, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

type Wrapper struct {
	Doc json.RawMessage `json:"doc"`
}

func main() {
	//make flags for a cli?
	port := getEnv("ALPR_PORT", "8080")
	client := NewClient("http://localhost:" + port)
	//read json
	plates, err := os.Open("plate_smart.json")
	if err != nil {
		fmt.Println("hello")
		log.Fatal(err)
	}
	defer plates.Close()

	dec := json.NewDecoder(plates)
	tok, err := dec.Token()
	if err != nil {
		log.Fatal(err)
	}

	if d, ok := tok.(json.Delim); !ok || d != '[' {
		log.Fatal("file must be json array")
	}

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	timeout := time.Second * 10
	idx := 0

	for dec.More() {
		var wrapper Wrapper
		//var raw json.RawMessage
		if err := dec.Decode(&wrapper); err != nil {
			log.Fatalf("post item %d: %v", idx, err)
		}
		//post this object.
		if err := postJSON(client, wrapper.Doc, timeout); err != nil {
			log.Fatalf("post item %d failed: %v", idx, err)
		} else {
			fmt.Printf("posted item %d (%d bytes)\n", idx, len(wrapper.Doc))
		}

		idx++
		if dec.More() {
			<-ticker.C
		}
	}

	if tok, err = dec.Token(); err != nil {
		log.Fatalf("reading closing token: %v", err)
	}

}

func postJSON(client *Client, raw json.RawMessage, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.base+"/api/alpr/v1/add", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpError{Code: resp.StatusCode}
	}

	return nil
}

type httpError struct {
	Code int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("unexpected status code: %d", e.Code)
}
