package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/KitHub/project_generator/config"
	"github.com/KitHub/project_generator/logic"
	"github.com/KitHub/project_generator/servicecontext"
	"github.com/KitHub/protocols/projectgeneratorapi"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServerArgs struct {
	ConfigFile string
}

func main() {
	ctx := context.Background()
	args := parepareArgs(ctx)

	// init config
	slog.InfoContext(ctx, "init config",
		slog.String("config_file", args.ConfigFile))
	configEntity, err := config.LoadConfig(ctx, args.ConfigFile)
	if err != nil {
		slog.ErrorContext(ctx, "init config failed",
			slog.String("error", err.Error()))
		panic(err)
	}
	slog.InfoContext(ctx, "init config done")

	// init service context
	serviceContext, err := servicecontext.InitServiceContext(
		ctx, &configEntity)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init service context",
			slog.String("error", err.Error()))
		panic(err)
	}

	// init server
	err = initServer(ctx, &configEntity, serviceContext)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init server",
			slog.String("error", err.Error()))
		panic(err)
	}

	shutdownGracefully(ctx, servicecontext.GetServiceContext().ShutdownLogic.GetShutdownCallbacks(ctx))
}

func parepareArgs(ctx context.Context) ServerArgs {
	configFile := flag.String("server_config", "", "config file for server")
	flag.Parse()
	result := ServerArgs{
		ConfigFile: *configFile,
	}
	slog.InfoContext(ctx, "parse flags done")
	return result
}

func initServer(ctx context.Context, serviceConfig *config.ConfigEntity,
	serviceContext *servicecontext.ServiceContext) (err error) {

	slog.InfoContext(ctx, "init servers")
	for _, serverConfig := range serviceConfig.Server.Services {
		switch serverConfig.Type {
		case "rpc":
			{
				_, err := initRpcServer(ctx, serverConfig, serviceContext)
				if err != nil {
					slog.ErrorContext(ctx, "failed to init RPC server",
						slog.String("error", err.Error()))
					return err
				}
			}
		case "http":
			{
				_, err := initHttpServer(ctx, serverConfig, serviceContext)
				if err != nil {
					slog.ErrorContext(ctx, "failed to init HTTP server",
						slog.String("error", err.Error()))
					return err
				}
			}
		default:
			{
				slog.ErrorContext(ctx, "unsupported server type",
					slog.String("type", serverConfig.Type))
				return fmt.Errorf("unsupported server type: %s", serverConfig.Type)
			}
		}

	}
	slog.InfoContext(ctx, "init servers done")
	return nil
}

func initRpcServer(ctx context.Context, serverConfig *config.ServiceConfigEntity, serviceContext *servicecontext.ServiceContext) (*grpc.Server, error) {
	slog.InfoContext(ctx, "init rpc server", slog.Any("serverConfig", serverConfig))
	hostAndPort := fmt.Sprintf("%s:%d", serverConfig.Host, serverConfig.Port)
	listener, err := net.Listen("tcp", hostAndPort)
	if err != nil {
		slog.ErrorContext(ctx, "failed to listen",
			slog.String("error", err.Error()))
		return nil, err
	}
	// create a new gRPC server
	server := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	// bind the service implementation to the gRPC server
	projectgeneratorapi.RegisterProjectGeneratorAPIServer(
		server, serviceContext.ProjectService)

	go func() {
		err := server.Serve(listener)
		if err != nil {
			slog.ErrorContext(ctx, "failed to serve",
				slog.String("error", err.Error()))
			panic(err)
		}
	}()

	servicecontext.GetServiceContext().ShutdownLogic.RegisterShutdownCallback(func(ctx context.Context) error {
		server.GracefulStop()
		slog.InfoContext(ctx, "gRPC server stopped gracefully")
		return nil
	})

	return server, nil
}

func initHttpServer(ctx context.Context, serverConfig *config.ServiceConfigEntity, serviceContext *servicecontext.ServiceContext) (*http.Server, error) {
	slog.InfoContext(ctx, "init http server", slog.Any("serverConfig", serverConfig))
	hostAndPort := fmt.Sprintf("%s:%d", serverConfig.Host, serverConfig.Port)
	connection, err := grpc.NewClient(hostAndPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to dial gRPC server", slog.String("error", err.Error()))
		return nil, err
	}
	restGateway := runtime.NewServeMux()
	err = projectgeneratorapi.RegisterProjectGeneratorAPIHandlerClient(ctx, restGateway, projectgeneratorapi.NewProjectGeneratorAPIClient(connection))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to register REST gateway", slog.String("error", err.Error()))
		return nil, err
	}

	server := http.Server{
		Addr:    hostAndPort,
		Handler: restGateway,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				slog.InfoContext(ctx, "HTTP server closed")
				return
			} else {
				slog.ErrorContext(ctx, "Failed to start HTTP gateway", slog.String("error", err.Error()))
				panic(err)
			}
		}
	}()

	servicecontext.GetServiceContext().ShutdownLogic.RegisterShutdownCallback(func(ctx context.Context) error {
		err = server.Shutdown(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to shutdown HTTP server gracefully", slog.String("error", err.Error()))
			return err
		}
		slog.InfoContext(ctx, "HTTP server stopped gracefully")
		return nil
	})

	return &server, nil
}

func shutdownGracefully(ctx context.Context, shutdownCallbacks []logic.ShutdownCallback) {
	slog.InfoContext(ctx, "listening close signals...")
	c := make(chan os.Signal, 1)
	signal.Notify(
		c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT,
		syscall.SIGTERM,
	)

	<-c
	slog.InfoContext(ctx, "graceful shutdown being executed...")

	for _, callback := range shutdownCallbacks {
		if err := callback(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to execute shutdown callback", slog.Any("error", err))
		}
	}

	slog.InfoContext(ctx, "graceful shutdown done")
}
