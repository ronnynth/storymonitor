package cometbft

import (
	"context"
	"fmt"
	"time"

	"storymonitor/base"
	"storymonitor/conf"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/golang/glog"
)

type CometbftCheckerImpl struct {
	*conf.Cometbft
	base.BaseChecker

	ctx context.Context

	client *rpchttp.HTTP
}

func NewCometbftCheckerImpl(ctx context.Context, conf *conf.Cometbft) base.CheckerTrait {
	checker := &CometbftCheckerImpl{
		ctx:      ctx,
		Cometbft: conf,
		BaseChecker: base.BaseChecker{
			ChainName:    conf.ChainName,
			HostName:     conf.HostName,
			ChainId:      conf.ChainId,
			NodeVersion:  conf.NodeVersion,
			ProtocolName: conf.ProtocolName,
		},
	}

	// Set default values
	if checker.CheckSecond == 0 {
		checker.CheckSecond = 5
	}
	if checker.WsEndpoint == "" {
		checker.WsEndpoint = "/websocket"
	}

	checker.updateClient()
	return checker
}

func (chain *CometbftCheckerImpl) updateClient() {
	nodeName := chain.Cometbft.HostName

	client, err := rpchttp.New(chain.HttpURL, chain.WsEndpoint)
	if err != nil {
		glog.Errorf("[updateClient] Node %s endpoint %s connect fail: %v", nodeName, chain.HttpURL, err)
		chain.RecordConnectionAttempt("http", false)
		return
	}

	chain.client = client

	// Get node status and information
	result, err := chain.client.Status(chain.ctx)
	if err != nil {
		glog.Errorf("[updateClient] Node %s status check fail: %v", nodeName, chain.HttpURL, err)
		chain.RecordConnectionAttempt("http", false)
		return
	}

	chain.RecordConnectionAttempt("http", true)
	chain.Cometbft.ChainId = result.NodeInfo.Network
	chain.Cometbft.NodeVersion = result.NodeInfo.Version
	chain.BaseChecker.ChainId = result.NodeInfo.Network
	chain.BaseChecker.NodeVersion = result.NodeInfo.Version

	glog.V(5).Infof("[updateClient] Node %s connected - Chain: %s, Version: %s",
		nodeName, chain.Cometbft.ChainId, chain.Cometbft.NodeVersion)
}

func (chain *CometbftCheckerImpl) checkStatus() {
	chain.HealthCheckOperation("node_status", func() error {
		_, err := chain.client.Status(chain.ctx)
		if err != nil {
			return err
		}

		return nil
	})
}

func (chain *CometbftCheckerImpl) startAndSubscribe(subscriber string) (<-chan ctypes.ResultEvent, error) {
	nodeName := chain.Cometbft.HostName

	if chain.client == nil {
		return nil, fmt.Errorf("[startAndSubscribe] client is nil")
	}

	if err := chain.client.Start(); err != nil {
		return nil, fmt.Errorf("[startAndSubscribe] Node %s client start fail: %v", nodeName, err)
	}

	// Subscribe to new block header events
	query := fmt.Sprintf("%s='%s'", tmtypes.EventTypeKey, tmtypes.EventNewBlockHeader)
	eventCh, err := chain.client.Subscribe(chain.ctx, subscriber, query)
	if err != nil {
		glog.Errorf("[startAndSubscribe] Node %s subscribe fail: %v", nodeName, err)
		return nil, err
	}

	return eventCh, nil
}

func (chain *CometbftCheckerImpl) subscribe() {
	var (
		subscriber = "subscriber"
		nodeName   = chain.Cometbft.HostName
		eventCh    <-chan ctypes.ResultEvent
		err        error
	)

	ticker := base.CheckSecondToTicker(chain.CheckSecond, 5)
	defer ticker.Stop()
	defer func() {
		if chain.client != nil {
			chain.client.UnsubscribeAll(chain.ctx, subscriber)
			chain.client.OnStop()
			chain.client.Stop()
		}
	}()

	ensureSubscription := func(chain *CometbftCheckerImpl) error {
		// Initialize subscription
		eventCh, err = chain.startAndSubscribe(subscriber)
		if err != nil {
			glog.Errorf("[subscribe] Initial subscription failed for %s: %v", nodeName, err)
			return err
		}
		return nil
	}

	if ensureSubscription(chain) != nil {
		return
	}

	for {
		select {
		case <-chain.ctx.Done():
			glog.V(5).Info("[subscribe] Received stop signal, exited")
			return

		case event := <-eventCh:
			if blockHeader, ok := event.Data.(tmtypes.EventDataNewBlockHeader); ok {
				header := blockHeader.Header
				chain.UpdateLastBlockTime()
				delaySecond := float64(time.Now().Unix() - header.Time.Unix())
				chain.RecordBlockProcessingDelay(delaySecond)
				glog.V(5).Infof("[subscribe] %s Node BlockNumber %d Delay %.2f s",
					nodeName, header.Height, delaySecond)
				chain.checkStatus()
			}

		case <-ticker.C:
			// Periodically check connection status
			if chain.client == nil {
				chain.updateClient()
				ensureSubscription(chain)
			} else if !chain.client.IsRunning() {
				ensureSubscription(chain)
			}
		}
	}
}

func (chain *CometbftCheckerImpl) GetHostName() string {
	return chain.Cometbft.HostName
}

func (chain *CometbftCheckerImpl) GetChainId() string {
	return chain.Cometbft.ChainId
}

func (chain *CometbftCheckerImpl) GetNodeVersion() string {
	return chain.Cometbft.NodeVersion
}

func (chain *CometbftCheckerImpl) GetChainName() string {
	return chain.Cometbft.ChainName
}

func (chain *CometbftCheckerImpl) GetProtocolName() string {
	return chain.Cometbft.ProtocolName
}

func (chain *CometbftCheckerImpl) Start() {
	glog.Infof("[CometBFT] Starting checker for %s (%s)", chain.Cometbft.HostName, chain.Cometbft.ChainName)

	// Start main subscription logic
	chain.subscribe()
}
