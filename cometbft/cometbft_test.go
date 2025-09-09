package cometbft

import (
	"context"
	"fmt"
	"testing"
	"time"

	"storymonitor/conf"

	tmtypes "github.com/cometbft/cometbft/types"
)

var (
	subscriber  = "subscriber"
	ctx, cancel = context.WithCancel(context.Background())
	chain       *CometbftCheckerImpl
)

func init() {
	cf := &conf.Cometbft{
		HostName:   "test-node-01",
		ChainId:    "cosmos",
		ChainName:  "cosmos",
		HttpURL:    "http://1.1.1.1:26657",
		WsEndpoint: "/websocket",
	}
	chain = &CometbftCheckerImpl{
		ctx:      ctx,
		Cometbft: cf,
	}
	chain.updateClient()
}

func TestNodeStatus(t *testing.T) {
	result, err := chain.client.Status(ctx)
	if err != nil {
		t.Error(err)
	}
	t.Log(result)
}

func TestBlock(t *testing.T) {
	result, err := chain.client.Block(chain.ctx, &[]int64{18360480}[0])
	if err != nil {
		t.Error(err)
	}
	t.Log(result.BlockID)
	t.Log(result.Block)
}

func TestTx(t *testing.T) {
	result, err := chain.client.Tx(
		chain.ctx,
		[]byte("0x6A1566DB799A831E2BA5FC461CB362D7E6118E17E88439DB5FDA6AAB7746AB92"),
		false)
	if err != nil {
		t.Error(err)
	}
	t.Log(*result)
}

func TestNewBlock(t *testing.T) {
	err := chain.client.Start()
	if err != nil {
		t.Error(err)
	}
	defer chain.client.Stop()
	isRun := chain.client.IsRunning()
	t.Log("running: ", isRun)
	query := fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader)
	eventCh, err := chain.client.Subscribe(chain.ctx, subscriber, query)
	if err != nil {
		t.Error(err)
	}
	defer chain.client.UnsubscribeAll(chain.ctx, subscriber)
	go func() {
		for {
			select {
			case evt := <-eventCh:
				if blockHeader, ok := evt.Data.(tmtypes.EventDataNewBlockHeader); ok {
					t.Log(blockHeader.Header.Height)
					t.Log(blockHeader.Header.LastCommitHash)
					t.Log(blockHeader.Header.DataHash)
				}
			case <-chain.ctx.Done():
				t.Log("canceled!")
				return
			}
		}
	}()
	<-time.After(30 * time.Second)
	cancel()
}
