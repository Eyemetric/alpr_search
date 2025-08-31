package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// create http client
type PlateHitSender struct {
	client *http.Client
	base   string
	token  string
}

func NewPlateSender(conf AlertConfig) *PlateHitSender {
	return &PlateHitSender{
		client: &http.Client{Timeout: 15 * time.Second},
		base:   conf.PlateHitUrl,
		token:  conf.AuthToken,
	}
}

type SendResult struct {
	StatusCode    int
	StatusMessage string
	Error         error
}

func (p PlateHitSender) Send(ctx context.Context, hits PlateHits) SendResult {

	body, err := json.Marshal(hits)
	if err != nil {
		return SendResult{StatusCode: 0, StatusMessage: "", Error: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base, bytes.NewBuffer(body))
	if err != nil {
		return SendResult{StatusCode: 0, StatusMessage: "", Error: err}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return SendResult{StatusCode: 0, StatusMessage: "", Error: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return SendResult{StatusCode: resp.StatusCode}
	}

	data, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return SendResult{StatusCode: resp.StatusCode, StatusMessage: msg}
}
