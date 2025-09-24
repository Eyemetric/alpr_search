package alert

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// create http client
type PlateHitSender struct {
	client *http.Client
	base   string
	token  string
}

func NewPlateSender(conf AlertConfig) *PlateHitSender {

	return &PlateHitSender{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Timeout: conf.SendTimeout},
		base:  conf.PlateHitUrl,
		token: conf.AuthToken,
	}
}

func (p PlateHitSender) Send(ctx context.Context, hits PlateHits) (int, error) {

	body, err := json.Marshal(hits)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base, bytes.NewBuffer(body))
	if err != nil {
		return 0, fmt.Errorf("building send request failed: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthToken", p.token)

	resp, err := p.client.Do(req)

	//we never reached the server
	if err != nil {
		if resp != nil {
			resp.Body.Close() //prevent rare mem leakage.
		}
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return resp.StatusCode, nil
	}

	data, _ := io.ReadAll(resp.Body)

	apiErr := &ApiError{Status: resp.StatusCode, Raw: strings.TrimSpace(string(data))}
	_ = json.Unmarshal(data, apiErr)            //parse could fail but we have raw json so ignore err
	if apiErr.Title == "" && apiErr.Raw == "" { //defaults if parse fails.
		apiErr.Title = http.StatusText(resp.StatusCode)
	}
	return resp.StatusCode, apiErr
}

//api errors from state are modeled after RFC 7807 https://datatracker.ietf.org/doc/html/rfc7807

type ApiError struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Detail string              `json:"detail"`
	Trace  string              `json:"traceId"`
	Errors map[string][]string `json:"errors"`
	Raw    string              `json:"-"` // raw body for unknown formats
}

// converting to a string should, concat all the errors from the Errors map.
func (e ApiError) Error() string {
	var parts []string
	// Prefer validation messages if present.
	if len(e.Errors) > 0 {
		for field, msgs := range e.Errors {
			if len(msgs) > 0 {
				parts = append(parts, field+": "+strings.Join(msgs, "; "))
			}
		}
	}
	switch {
	case len(parts) > 0 && e.Title != "":
		return e.Title + ": " + strings.Join(parts, " | ")
	case len(parts) > 0:
		return strings.Join(parts, " | ")
	case e.Detail != "" && e.Title != "":
		return e.Title + ": " + e.Detail
	case e.Detail != "":
		return e.Detail
	case e.Title != "":
		return e.Title
	case e.Raw != "":
		return e.Raw
	default:
		return "request failed"
	}

}
