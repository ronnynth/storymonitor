package evm

import (
	"context"
	"testing"
	"time"

	"storymonitor/conf"
)

var (
	ctx, cancel = context.WithCancel(context.Background())
	chain       *EvmCheckerImpl
)

func init() {
	cf := &conf.Evm{
		HostName:  "ethereum-01",
		ChainId:   "1",
		ChainName: "ethereum",
		HttpURL:   "https://mainnet.gateway.tenderly.co",
		WsURL:     "wss://0xrpc.io/eth",
	}
	chain = &EvmCheckerImpl{
		ctx: ctx,
		Evm: cf,
	}
	chain.updateClient()
}

func TestClientVersion(t *testing.T) {
	var (
		ver string
	)
	err := chain.http.Client().CallContext(chain.ctx, &ver, "web3_clientVersion")
	if err != nil {
		t.Error(err)
	}
	t.Log(ver)
}

func TestWSChainID(t *testing.T) {
	if chain.ws == nil {
		t.Skip("WebSocket client not initialized, skipping test")
		return
	}

	ctx, cancel := context.WithTimeout(chain.ctx, 10*time.Second)
	defer cancel()

	chainID, err := chain.ws.ChainID(ctx)
	if err != nil {
		t.Errorf("Failed to get ChainID via WebSocket: %v", err)
		return
	}

	if chainID == nil {
		t.Error("ChainID is nil")
		return
	}

	expectedChainID := "1"
	if chainID.String() != expectedChainID {
		t.Errorf("Expected ChainID %s, got %s", expectedChainID, chainID.String())
	}

	t.Logf("WebSocket ChainID: %s", chainID.String())
}

func TestWSClientVersion(t *testing.T) {
	if chain.ws == nil {
		t.Skip("WebSocket client not initialized, skipping test")
		return
	}

	ctx, cancel := context.WithTimeout(chain.ctx, 10*time.Second)
	defer cancel()

	var version string
	err := chain.ws.Client().CallContext(ctx, &version, "web3_clientVersion")
	if err != nil {
		t.Errorf("Failed to get client version via WebSocket: %v", err)
		return
	}

	if version == "" {
		t.Error("Client version is empty")
		return
	}

	t.Logf("WebSocket Client Version: %s", version)
}

func TestWSConnectionHealth(t *testing.T) {
	if chain.ws == nil {
		t.Skip("WebSocket client not initialized, skipping test")
		return
	}

	ctx, cancel := context.WithTimeout(chain.ctx, 5*time.Second)
	defer cancel()

	_, err := chain.ws.ChainID(ctx)
	if err != nil {
		t.Errorf("WebSocket connection health check failed: %v", err)
	} else {
		t.Log("WebSocket connection is healthy")
	}
}
