package base

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

type HTTPClient struct {
	Header  map[string]string `json:"header"`
	Cli     *http.Client      `json:"cli"`
	Payload []byte            `json:"payload"`
	Params  map[string]string `json:"params"`
}

func (r *HTTPClient) SetTimeOut(timeOut time.Duration) {
	r.Cli.Timeout = timeOut
}

func (r *HTTPClient) SetHeader(key, value string) *HTTPClient {
	r.Header[key] = value
	return r
}

func (r *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	params := r.Params
	q := req.URL.Query()
	if params != nil {
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	resp, err := r.Cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %s", err)
	}
	return resp, nil
}

func (r *HTTPClient) Req(ctx context.Context, url, method string, header http.Header) (*http.Response, error) {
	var req *http.Request
	var err error
	data := r.Payload
	if data != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(data))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	if header != nil {
		req.Header = header
	}
	for key, value := range r.Header {
		req.Header.Set(key, value)
	}

	req = req.WithContext(ctx)
	return r.Do(req)
}

func NewHTTPClient(cli *http.Client) *HTTPClient {
	header := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	return &HTTPClient{
		Cli:    cli,
		Header: header,
	}
}
