package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "storymonitor/base"
	"storymonitor/conf"
	"storymonitor/sched"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

var (
	confPath string
	ac       = conf.NodeConfig{}
)

func init() {
	// Initialize command line flags
	flag.StringVar(&confPath, "conf", "./config.yaml", "config file path")
	flag.Parse()
}

func loadConf(path string) error {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(yamlFile, &ac); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Validate configuration
	if err := validateConfig(&ac); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return nil
}

func validateConfig(config *conf.NodeConfig) error {
	if len(config.Evm) == 0 && len(config.Cometbft) == 0 {
		return fmt.Errorf("no monitoring targets configured")
	}

	// Validate EVM configurations
	for i, evm := range config.Evm {
		if evm.HostName == "" {
			return fmt.Errorf("evm[%d]: hostname is required", i)
		}
		if evm.HttpURL == "" {
			return fmt.Errorf("evm[%d]: http_url is required", i)
		}
		if evm.ChainName == "" {
			return fmt.Errorf("evm[%d]: chain_name is required", i)
		}
	}

	// Validate CometBFT configurations
	for i, cometbft := range config.Cometbft {
		if cometbft.HostName == "" {
			return fmt.Errorf("cometbft[%d]: hostname is required", i)
		}
		if cometbft.HttpURL == "" {
			return fmt.Errorf("cometbft[%d]: http_url is required", i)
		}
		if cometbft.ChainName == "" {
			return fmt.Errorf("cometbft[%d]: chain_name is required", i)
		}
	}

	// Beacon support removed for Story protocol-only monitor

	return nil
}

func setupHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &http.Server{
		Addr:         ":3002",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
}

func startPprofServer() {
	pprofServer := &http.Server{
		Addr:         "localhost:6062",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	go func() {
		if err := pprofServer.ListenAndServe(); err != http.ErrServerClosed {
			glog.Errorf("pprof server error: %v", err)
		}
	}()
}

func gracefulShutdown(ctx context.Context, cancel context.CancelFunc, controller *sched.Controller, server *http.Server) {
	// Wait for interrupt signals
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-term
	glog.Infof("Received signal %v, starting graceful shutdown...", sig)

	// Create shutdown timeout context
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	// Stop controller
	glog.Info("Stopping controller...")
	controller.Stop()

	// Cancel application context
	cancel()

	// Shutdown HTTP server
	glog.Info("Shutting down HTTP server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		glog.Errorf("Error during server shutdown: %v", err)
	} else {
		glog.Info("HTTP server shutdown completed")
	}
}

func main() {
	defer glog.Flush()

	// Load configuration
	if err := loadConf(confPath); err != nil {
		glog.Fatalf("Failed to load config: %v", err)
	}

	glog.Infof("Loaded config from %s", confPath)
	glog.Infof("Monitoring %d EVM chains, %d CometBFT chains",
		len(ac.Evm), len(ac.Cometbft))

	// Create application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create controller
	controller := sched.NewController(ctx, &ac)

	// Setup HTTP server
	server := setupHTTPServer()

	// Start pprof server
	startPprofServer()

	// Start graceful shutdown handler
	go gracefulShutdown(ctx, cancel, controller, server)

	// Start controller
	glog.Info("Starting blockchain monitor...")
	controller.Start()

	// Start HTTP server
	glog.Infof("HTTP server listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		glog.Errorf("HTTP server error: %v", err)
	}

	glog.Info("Application shutdown completed")
}
