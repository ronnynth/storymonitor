package base

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type Client struct {
	ctx context.Context
	cli *http.Client
}

func NewClient(ctx context.Context, cli *http.Client) *Client {
	return &Client{
		cli: cli,
		ctx: ctx,
	}
}

func (c *Client) Fetch(url, method string, body interface{}, params map[string]string) ([]byte, error) {
	client := NewHTTPClient(c.cli)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		client.Payload = b
	}
	if params != nil {
		client.Params = params
	}
	resp, err := client.Req(c.ctx, url, method, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return respBody, nil
}
