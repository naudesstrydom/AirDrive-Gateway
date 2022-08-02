package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/IceWhaleTech/CasaOS-Gateway/common"
	"github.com/IceWhaleTech/CasaOS-Gateway/config"
	"github.com/IceWhaleTech/CasaOS-Gateway/route"
	"github.com/IceWhaleTech/CasaOS-Gateway/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"golang.org/x/sync/errgroup"
)

var cfg *config.State

func main() {
	cfg = config.NewState()

	if err := config.Load(cfg); err != nil {
		panic(err)
	}

	if err := checkPrequisites(); err != nil {
		panic(err)
	}

	pidFilename, err := writePidFile()
	if err != nil {
		panic(err)
	}

	defer cleanupFiles(
		pidFilename,
		service.RoutesFile,
		common.GatewayURLFilename,
		common.ManagementURLFilename,
	)

	ctx, cancel := context.WithCancel(context.Background())
	kill := make(chan os.Signal, 1)
	signal.Notify(kill, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-kill
		cancel()
	}()

	app := fx.New(
		fx.Provide(func() *service.Management {
			return service.NewManagementService(cfg)
		}),
		fx.Provide(route.NewRoutes),
		fx.Invoke(run),
	)

	if err := app.Start(ctx); err != nil {
		log.Println(err)
	}
}

func run(
	lifecycle fx.Lifecycle,
	route *gin.Engine,
	management *service.Management,
) {
	lifecycle.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				var g errgroup.Group

				// management server
				g.Go(func() error {
					return serve(common.ManagementURLFilename, "127.0.0.1:0", route)
				})

				// gateway server
				g.Go(func() error {
					gatewayMux := http.NewServeMux()
					gatewayMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
						proxy := management.GetProxy(r.URL.Path)

						if proxy == nil {
							w.WriteHeader(http.StatusNotFound)
							return
						}

						proxy.ServeHTTP(w, r)
					})

					port := cfg.GetGatewayPort()
					if port == "" {
						if err := cfg.SetGatewayPort("80"); err != nil {
							return err
						}
					}

					addr := net.JoinHostPort("", port)

					return serve(common.GatewayURLFilename, addr, gatewayMux)
				})

				return g.Wait()
			},
		})
}

func writePidFile() (string, error) {
	runtimePath := cfg.GetRuntimePath()

	filename := "gateway.pid"
	filepath := filepath.Join(runtimePath, filename)
	return filename, ioutil.WriteFile(filepath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600)
}

func writeAddressFile(filename string, address string) (string, error) {
	runtimePath := cfg.GetRuntimePath()

	err := os.MkdirAll(runtimePath, 0o755)
	if err != nil {
		return "", err
	}

	filepath := filepath.Join(runtimePath, filename)
	return filepath, ioutil.WriteFile(filepath, []byte(address), 0o600)
}

func cleanupFiles(filenames ...string) {
	runtimePath := cfg.GetRuntimePath()

	for _, filename := range filenames {
		err := os.Remove(filepath.Join(runtimePath, filename))
		if err != nil {
			log.Println(err)
		}
	}
}

func checkPrequisites() error {
	path := cfg.GetRuntimePath()

	err := os.MkdirAll(path, 0o755)
	if err != nil {
		return fmt.Errorf("please ensure the owner of this service has write permission to that path %s", path)
	}

	return nil
}

func serve(urlFilename string, addr string, route http.Handler) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}

	url := "http://" + listener.Addr().String()

	urlFilePath, err := writeAddressFile(urlFilename, url)
	if err != nil {
		return err
	}

	log.Printf("listening on %s (saved to %s)", url, urlFilePath)

	s := &http.Server{
		Handler:           route,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s.Serve(listener)
}
