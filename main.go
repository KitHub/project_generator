package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/KitHub/protocols/devicemanagementplatformapi"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ProjectRequest project generation request struct
type ProjectRequest struct {
	AppName    string `json:"app_name"`    // project name
	ModuleName string `json:"module_name"` // go module name
	Port       string `json:"port"`        // server port
}

// templates files
var templateFiles = map[string]string{
	"main":   "templates/main.go.tmpl",
	"mod":    "templates/go.mod.tmpl",
	"router": "templates/router.go.tmpl",
	"config": "templates/config.go.tmpl",
	"readme": "templates/README.md.tmpl",
}

// renderTemplate read template file and render with data
func renderTemplate(tplPath string, data ProjectRequest) (string, error) {
	tpl, err := template.ParseFiles(tplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generateProject generate project files and return zip data
func generateProject(req ProjectRequest) ([]byte, error) {
	// create temp dir
	tmpDir, err := os.MkdirTemp("", "gen-project-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// project root and dirs
	root := filepath.Join(tmpDir, req.AppName)
	dirs := []string{
		root,
		filepath.Join(root, "router"),
		filepath.Join(root, "config"),
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}

	// render templates
	mainContent, _ := renderTemplate(templateFiles["main"], req)
	modContent, _ := renderTemplate(templateFiles["mod"], req)
	routerContent, _ := renderTemplate(templateFiles["router"], req)
	configContent, _ := renderTemplate(templateFiles["config"], req)
	readmeContent, _ := renderTemplate(templateFiles["readme"], req)

	// define files to write
	files := map[string]string{
		filepath.Join(root, "main.go"):          mainContent,
		filepath.Join(root, "go.mod"):           modContent,
		filepath.Join(root, "README.md"):        readmeContent,
		filepath.Join(root, "router/router.go"): routerContent,
		filepath.Join(root, "config/config.go"): configContent,
	}

	// write files
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return nil, err
		}
	}

	// zip project
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(tmpDir, path)
		f, err := zw.Create(relPath)
		if err != nil {
			return err
		}

		b, _ := os.ReadFile(path)
		_, _ = f.Write(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = zw.Close()

	return zipBuf.Bytes(), nil
}

// generateHandler HTTP api handler for project generation
func generateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.ProjectName == "" || req.ModuleName == "" || req.Port == "" {
		http.Error(w, "project_name, module_name, port are required", http.StatusBadRequest)
		return
	}

	zipData, err := generateProject(req)
	if err != nil {
		http.Error(w, "generate project failed: "+err.Error(), http.StatusInternalServerError)
		log.Printf("generate project failed: %v", err)
		return
	}

	filename := fmt.Sprintf("%s_%d.zip", req.ProjectName, time.Now().Unix())
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(zipData)

	log.Printf("generate project succeeded: %s | module: %s | port: %s", req.ProjectName, req.ModuleName, req.Port)
}

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

	shutdownGracefully(ctx, servicecontext.GetShutdownCallbacks())
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
	devicemanagementplatformapi.RegisterDeviceManagementPlatformAPIServer(
		server, serviceContext.ApiService)

	go func() {
		err := server.Serve(listener)
		if err != nil {
			slog.ErrorContext(ctx, "failed to serve",
				slog.String("error", err.Error()))
			panic(err)
		}
	}()

	servicecontext.RegisterShutdownCallback(func(ctx context.Context) error {
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
	err = devicemanagementplatformapi.RegisterDeviceManagementPlatformAPIHandlerClient(ctx, restGateway, devicemanagementplatformapi.NewDeviceManagementPlatformAPIClient(connection))
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
			slog.ErrorContext(ctx, "Failed to start HTTP gateway", slog.String("error", err.Error()))
			panic(err)
		}
	}()

	servicecontext.RegisterShutdownCallback(func(ctx context.Context) error {
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

func shutdownGracefully(ctx context.Context, shutdownCallbacks []servicecontext.ShutdownCallback) {
	slog.InfoContext(ctx, "listening signals...")
	c := make(chan os.Signal, 1)
	signal.Notify(
		c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT,
		syscall.SIGTERM,
	)

	<-c
	slog.InfoContext(ctx, "graceful shutdown...")

	for _, callback := range shutdownCallbacks {
		if err := callback(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to execute shutdown callback", slog.Any("error", err))
		}
	}

	slog.InfoContext(ctx, "completed graceful shutdown")
}
