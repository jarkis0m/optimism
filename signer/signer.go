package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	oppprof "github.com/ethereum-optimism/optimism/op-service/pprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/urfave/cli"

	"github.com/ethereum-optimism/optimism/signer/service"
)

func Main(version string) func(cliCtx *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		cfg := NewConfig(cliCtx)
		if err := cfg.Check(); err != nil {
			return fmt.Errorf("invalid CLI flags: %w", err)
		}

		l := oplog.NewLogger(cfg.LogConfig)

		service := service.NewSignerService(l)

		ctx, cancel := context.WithCancel(context.Background())

		pprofConfig := cfg.PprofConfig
		if pprofConfig.Enabled {
			l.Info("starting pprof", "addr", pprofConfig.ListenAddr, "port", pprofConfig.ListenPort)
			go func() {
				if err := oppprof.ListenAndServe(ctx, pprofConfig.ListenAddr, pprofConfig.ListenPort); err != nil {
					l.Error("error starting pprof", "err", err)
				}
			}()
		}

		registry := opmetrics.NewRegistry()
		metricsCfg := cfg.MetricsConfig
		if metricsCfg.Enabled {
			l.Info(
				"starting metrics server",
				"addr",
				metricsCfg.ListenAddr,
				"port",
				metricsCfg.ListenPort,
			)
			go func() {
				if err := opmetrics.ListenAndServe(ctx, registry, metricsCfg.ListenAddr, metricsCfg.ListenPort); err != nil {
					l.Error("error starting metrics server", err)
				}
			}()
		}

		rpcCfg := cfg.RPCConfig
		server := oprpc.NewServer(
			rpcCfg.ListenAddr,
			rpcCfg.ListenPort,
			version,
		)
		service.RegisterAPIs(server)
		l.Info("starting rpc server", "addr", rpcCfg.ListenAddr, "port", rpcCfg.ListenPort)
		if err := server.Start(); err != nil {
			cancel()
			return fmt.Errorf("error starting RPC server: %w", err)
		}

		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, []os.Signal{
			os.Interrupt,
			os.Kill,
			syscall.SIGTERM,
			syscall.SIGQUIT,
		}...)
		<-interruptChannel
		cancel()
		_ = server.Stop()

		return nil
	}
}
