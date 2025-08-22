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

func main() {
	//make flags for a cli?
	port := getEnv("ALPR_PORT", "8080")
	client := NewClient("http://localhost:" + port)
	//read json
	hotlist, err := os.Open("hotlist.json")
	if err != nil {
		fmt.Println("hello")
		log.Fatal(err)
	}
	defer hotlist.Close()

	var jmess json.RawMessage
	dec := json.NewDecoder(hotlist)
	err = dec.Decode(&jmess)
	if err != nil {
		log.Fatal(err)
	}

	timeout := time.Second * 5
	if err := postJSON(client, jmess, timeout); err != nil {
		log.Fatalf("post item failed: %v", err)
	} else {
		fmt.Printf("posted item (%d bytes)\n", len(jmess))
	}

}

func postJSON(client *Client, raw json.RawMessage, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.base+"/api/alpr/v1/hotlist", bytes.NewReader(raw))
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
