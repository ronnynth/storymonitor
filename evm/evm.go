package evm

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"storymonitor/base"
	"storymonitor/conf"

	"github.com/gorilla/websocket"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
)

type EvmCheckerImpl struct {
	*conf.Evm
	base.BaseChecker

	ctx context.Context

	http *client.Client
	ws   *client.Client
}

func NewEvmCheckerImpl(ctx context.Context, conf *conf.Evm) base.CheckerTrait {
	checker := &EvmCheckerImpl{
		Evm: conf,
		BaseChecker: base.BaseChecker{
			ChainName:    conf.ChainName,
			HostName:     conf.HostName,
			ChainId:      conf.ChainId,
			NodeVersion:  conf.NodeVersion,
			ProtocolName: conf.ProtocolName,
		},
		ctx: ctx,
	}

	// Set default check interval
	if checker.CheckSecond == 0 {
		checker.CheckSecond = 5
	}

	checker.updateClient()
	return checker
}

func (chain *EvmCheckerImpl) updateClient() {
	var (
		err         error
		c           *rpc.Client
		nodeVersion string
	)

	nodeName := chain.Evm.HostName

	// Attempt WebSocket connection
	if chain.WsURL != "" {
		dialer := websocket.Dialer{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			HandshakeTimeout: 12 * time.Second,
		}
		c, err = rpc.DialOptions(chain.ctx, chain.WsURL, rpc.WithWebsocketDialer(dialer))
		if err != nil {
			chain.RecordConnectionAttempt("ws", false)
			glog.Errorf("[updateClient] Node %s ws %s connect fail: %v", nodeName, chain.WsURL, err)
		} else {
			chain.RecordConnectionAttempt("ws", true)
			glog.V(5).Infof("[updateClient] Node %s ws %s connect success", nodeName, chain.WsURL)
			chain.ws = client.NewClient(c)
		}
		// websocket.PingMessage
	}

	// Attempt HTTP connection
	if chain.HttpURL != "" {
		if chain.http, err = client.DialContext(chain.ctx, chain.HttpURL); err != nil {
			chain.RecordConnectionAttempt("http", false)
			glog.Errorf("[updateClient] Node %s http %s connect fail: %v", nodeName, chain.HttpURL, err)
		} else {
			chain.RecordConnectionAttempt("http", true)
			glog.V(5).Infof("[updateClient] Node %s http %s connect success", nodeName, chain.HttpURL)

			// Get chain ID
			if chainID, err := chain.http.NetworkID(chain.ctx); err == nil {
				chain.Evm.ChainId = chainID.String()
				chain.BaseChecker.ChainId = chainID.String()
			}

			// Get node version
			if err := chain.http.Client().CallContext(chain.ctx, &nodeVersion, "web3_clientVersion"); err == nil {
				chain.Evm.NodeVersion = nodeVersion
				chain.BaseChecker.NodeVersion = nodeVersion
			}
		}
	}
}

func (chain *EvmCheckerImpl) subscribeNewHead() (sub ethereum.Subscription, headers chan *types.Header, err error) {
	headers = make(chan *types.Header)
	nodeName := chain.Evm.HostName
	if chain.ws == nil {
		return nil, nil, fmt.Errorf("websocket connection not available for node %s", nodeName)
	}

	sub, err = chain.ws.SubscribeNewHead(chain.ctx, headers)
	if err != nil {
		glog.Errorf("[subscribeNewHead] Node %s ws %s subscribe newhead fail: %v", nodeName, chain.WsURL, err)
	}
	return sub, headers, err
}

func (chain *EvmCheckerImpl) checkGetBlockByNumber() {
	chain.HealthCheckOperation("block_retrieval", func() error {
		_, err := chain.http.BlockNumber(chain.ctx)
		if err != nil {
			return err
		}

		return nil
	})
}

func (chain *EvmCheckerImpl) clientHealthCheck() {
	ticker := base.CheckSecondToTicker(chain.CheckSecond, 5)
	defer ticker.Stop()

	for {
		if !base.WaitForContextOrTicker(chain.ctx, ticker) {
			glog.V(5).Info("[clientHealthCheck] Received stop signal, exited")
			return
		}

		if chain.http == nil {
			glog.V(5).Infof("[clientHealthCheck] node: %s rebuilding chain client", chain.Evm.HostName)
			chain.updateClient()
		} else {
			glog.V(5).Infof("[clientHealthCheck] node: %s, chain: %s, connection normal", chain.Evm.HostName, chain.Evm.ChainName)
		}
	}
}

func (chain *EvmCheckerImpl) subscribe() {
	nodeName := chain.Evm.HostName
	ticker := base.CheckSecondToTicker(chain.CheckSecond, 5)
	defer ticker.Stop()

	var sub ethereum.Subscription
	var headers chan *types.Header
	var err error

	ensureSubscription := func() {
		// First ensure we have a WebSocket client
		if chain.ws == nil {
			chain.updateClient()
		}

		// Test WebSocket connection health if client exists
		if chain.ws != nil {
			ctx, cancel := context.WithTimeout(chain.ctx, 3*time.Second)
			defer cancel()
			_, err := chain.ws.ChainID(ctx)
			if err != nil {
				glog.Warningf("[subscribe] WebSocket health check failed for node %s: %v, reconnecting", nodeName, err)
				chain.updateClient()
			}
		}

		// Subscribe if we have a client but no active subscription
		if chain.ws != nil && sub == nil {
			sub, headers, err = chain.subscribeNewHead()
			if err != nil {
				glog.Errorf("[subscribe] Failed to subscribe: %v", err)
			}
		}
	}

	ensureSubscription()

	for {
		select {
		case <-chain.ctx.Done():
			glog.V(5).Info("[subscribe] Received stop signal, exited")
			if sub != nil {
				sub.Unsubscribe()
			}
			return

		case header := <-headers:
			if header == nil {
				glog.Warningf("[subscribe] Received nil header for node %s, reconnecting", nodeName)
				if sub != nil {
					sub.Unsubscribe()
					sub = nil
				}
				chain.updateClient()
				ensureSubscription()
				continue
			}

			chain.UpdateLastBlockTime()
			delaySecond := float64(time.Now().Unix() - int64(header.Time))
			chain.RecordBlockProcessingDelay(delaySecond)
			glog.V(5).Infof("[subscribe] %s Node BlockNumber %d Delay %.2f s", nodeName, header.Number.Uint64(), delaySecond)
			chain.checkGetBlockByNumber()

		case <-ticker.C:
			ensureSubscription()

		case err, ok := <-sub.Err():
			if !ok || err != nil {
				if err != nil {
					glog.Errorf("[subscribe] Subscription error for node %s: %v", chain.Evm.HostName, err)
				} else {
					glog.Warningf("[subscribe] Subscription channel closed for node %s", chain.Evm.HostName)
				}
				if sub != nil {
					sub.Unsubscribe()
					sub = nil
				}
				// Force reconnect on subscription errors
				chain.updateClient()
				ensureSubscription()
			}
		}
	}
}

func (chain *EvmCheckerImpl) Start() {
	glog.Infof("[EVM] Starting checker for %s (%s)", chain.Evm.HostName, chain.Evm.ChainName)

	// Start health check
	go chain.clientHealthCheck()

	// Start block subscription
	chain.subscribe()
}

func (chain *EvmCheckerImpl) GetHostName() string {
	return chain.Evm.HostName
}

func (chain *EvmCheckerImpl) GetChainId() string {
	return chain.Evm.ChainId
}

func (chain *EvmCheckerImpl) GetNodeVersion() string {
	return chain.Evm.NodeVersion
}

func (chain *EvmCheckerImpl) GetChainName() string {
	return chain.Evm.ChainName
}

func (chain *EvmCheckerImpl) GetProtocolName() string {
	return chain.Evm.ProtocolName
}
