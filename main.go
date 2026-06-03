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

	// init services
	err = initServices(ctx, &configEntity, serviceContext)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init services",
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

func initServices(ctx context.Context, serviceConfig *config.ConfigEntity,
	serviceContext *servicecontext.ServiceContext) (err error) {

	slog.InfoContext(ctx, "init services")
	grpcServiceConfig := serviceConfig.Server.GrpcService
	httpServiceConfig := serviceConfig.Server.HttpService
	_, err = initRpcServer(ctx, grpcServiceConfig, serviceContext)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init RPC server",
			slog.String("error", err.Error()))
		return err
	}
	_, err = initHttpServer(ctx, httpServiceConfig, grpcServiceConfig, serviceContext)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init HTTP server",
			slog.String("error", err.Error()))
		return err
	}
	slog.InfoContext(ctx, "init services done")
	return nil
}

// initRpcServer initializes the gRPC server and registers the service implementation.
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

// initHttpServer initializes the HTTP server and registers the http gateway.
// The http gateway translates HTTP API into gRPC calls to the gRPC server.
func initHttpServer(ctx context.Context, httpServerConfig *config.ServiceConfigEntity, grpcServerConfig *config.ServiceConfigEntity, serviceContext *servicecontext.ServiceContext) (*http.Server, error) {
	slog.InfoContext(ctx, "init http server", slog.Any("serverConfig", httpServerConfig))
	grpcHostAndPort := fmt.Sprintf("%s:%d", grpcServerConfig.Host, grpcServerConfig.Port)
	httpHostAndPort := fmt.Sprintf("%s:%d", httpServerConfig.Host, httpServerConfig.Port)
	gateway := runtime.NewServeMux()
	err := projectgeneratorapi.RegisterProjectGeneratorAPIHandlerFromEndpoint(ctx, gateway, grpcHostAndPort, []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to register http gateway", slog.String("error", err.Error()))
		return nil, err
	}

	server := http.Server{
		Addr:    httpHostAndPort,
		Handler: gateway,
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
