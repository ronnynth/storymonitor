package sched

import (
	"context"
	"sync"
	"time"

	"storymonitor/base"
	"storymonitor/cometbft"
	"storymonitor/conf"
	"storymonitor/evm"

	"github.com/golang/glog"
)

type Controller struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkers []base.CheckerTrait
	conf     *conf.NodeConfig

	// WaitGroup for managing goroutine lifecycle
	wg sync.WaitGroup

	// Flag to indicate if controller is stopped
	stopped bool
	mu      sync.RWMutex
}

func NewController(parent context.Context, conf *conf.NodeConfig) *Controller {
	ctx, cancel := context.WithCancel(parent)
	c := &Controller{
		ctx:    ctx,
		cancel: cancel,
		conf:   conf,
	}

	// Create EVM checkers
	for i, evmConf := range c.conf.Evm {
		if evmConf == nil {
			glog.Errorf("EVM config[%d] is nil, skipping", i)
			continue
		}
		glog.Infof("Creating EVM checker for %s (%s)", evmConf.HostName, evmConf.ChainName)
		checker := evm.NewEvmCheckerImpl(c.ctx, evmConf)
		c.checkers = append(c.checkers, checker)
	}

	// Create CometBFT checkers
	for i, cometbftConf := range c.conf.Cometbft {
		if cometbftConf == nil {
			glog.Errorf("CometBFT config[%d] is nil, skipping", i)
			continue
		}
		glog.Infof("Creating CometBFT checker for %s (%s)", cometbftConf.HostName, cometbftConf.ChainName)
		checker := cometbft.NewCometbftCheckerImpl(c.ctx, cometbftConf)
		c.checkers = append(c.checkers, checker)
	}

	glog.Infof("Created %d checkers total", len(c.checkers))
	return c
}

func (c *Controller) UpdateBlockLifetime() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			glog.V(5).Info("[UpdateBlockLifetime] Received stop signal, exited")
			return
		case <-ticker.C:
			// Update block lifetime metrics for all checkers
			for _, checker := range c.checkers {
				if checker != nil {
					base.BlockLastUpdateTime.WithLabelValues(
						checker.GetChainName(),
						checker.GetHostName(),
						checker.GetChainId(),
						checker.GetNodeVersion(),
						checker.GetProtocolName(),
					).Inc()
				}
			}
		}
	}
}

func (c *Controller) startChecker(checker base.CheckerTrait) {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			glog.Errorf("Checker %s (%s) panic recovered: %v",
				checker.GetHostName(), checker.GetChainName(), r)
		}
	}()

	glog.Infof("[Controller] Starting checker: %s (%s)",
		checker.GetHostName(), checker.GetChainName())

	// Start the checker
	checker.Start()

	glog.Infof("[Controller] Checker stopped: %s (%s)",
		checker.GetHostName(), checker.GetChainName())
}

func (c *Controller) IsStopped() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stopped
}

func (c *Controller) Start() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		glog.Warning("Controller is already stopped, cannot start")
		return
	}
	c.mu.Unlock()

	glog.Infof("Starting controller with %d checkers", len(c.checkers))

	// Start block lifetime updater
	c.wg.Add(1)
	go c.UpdateBlockLifetime()

	// Start all checkers
	for _, checker := range c.checkers {
		if checker != nil {
			c.wg.Add(1)
			go c.startChecker(checker)
		}
	}

	glog.Info("All checkers started")
}

func (c *Controller) Stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		glog.Info("Controller is already stopped")
		return
	}
	c.stopped = true
	c.mu.Unlock()

	glog.Info("Stopping controller...")

	// Cancel context to notify all checkers to stop
	c.cancel()

	// Wait for all goroutines to complete with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		glog.Info("All checkers stopped successfully")
	case <-time.After(30 * time.Second):
		glog.Warning("Timeout waiting for checkers to stop")
	}
}

// GetStats returns statistics about the controller
func (c *Controller) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_checkers": len(c.checkers),
		"stopped":        c.IsStopped(),
	}

	// Count checkers by type
	evmCount := len(c.conf.Evm)
	cometbftCount := len(c.conf.Cometbft)

	stats["evm_checkers"] = evmCount
	stats["cometbft_checkers"] = cometbftCount

	return stats
}
