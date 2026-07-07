package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/svetozarm/servitor/internal/config"
	"github.com/svetozarm/servitor/internal/proxy"
	"github.com/svetozarm/servitor/internal/scaffold"
	"github.com/svetozarm/servitor/internal/server"
	"github.com/svetozarm/servitor/internal/sync"
	"github.com/svetozarm/servitor/internal/update"
)

var version string

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "update":
			runUpdate()
			return
		case "version":
			v := version
			if v == "" {
				v = "dev"
			}
			fmt.Println("servitor " + v)
			return
		}
	}
	runServer()
}

func runInit() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var templateName, repoURL string
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--repo" && i+1 < len(args) {
			repoURL = args[i+1]
			i++
		} else if args[i][0] != '-' && templateName == "" {
			templateName = args[i]
		}
	}

	var upstreamURL string
	_, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		fmt.Print("upstream git repo URL (leave empty to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			upstreamURL = scanner.Text()
		}
	}

	opts := scaffold.Options{
		TemplateName: templateName,
		RepoURL:      repoURL,
		TargetDir:    ".",
		UpstreamURL:  upstreamURL,
	}
	if err := scaffold.Init(opts, logger); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("project initialised successfully")
}

func runServer() {
	cfg, err := config.Load(".servitor.conf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var srv *server.Server
	var syncDones []chan struct{}

	apps, err := config.ResolveApps(cfg, ".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var appHandlers []server.AppHandler
	for _, app := range apps {
		if app.Sync.Enabled {
			dataDir := filepath.Join(app.Dir, "data") + "/"
			cwd, _ := os.Getwd()
			relDataDir, err := filepath.Rel(cwd, filepath.Join(app.Dir, "data"))
			if err != nil {
				relDataDir = filepath.Join(app.Dir, "data")
			}
			syncer, err := sync.NewGoGitSyncer(".", logger)
			if err != nil {
				logger.Error("failed to open git repo", "app", app.Name, "err", err)
				os.Exit(1)
			}
			engine := sync.NewSyncEngineWithPaths(syncer, dataDir, relDataDir+"/", app.Sync.InactivityDelay.Duration, app.Sync.MaxInterval.Duration, logger)
			done := make(chan struct{})
			syncDones = append(syncDones, done)
			go func() {
				defer close(done)
				if err := engine.Start(ctx); err != nil {
					logger.Error("sync engine error", "app", app.Name, "err", err)
				}
			}()
		}

		invoker := &proxy.CGIInvoker{
			BinaryPath: app.Backend.Path,
			Timeout:    app.Backend.Timeout.Duration,
			WorkDir:    app.Dir,
		}
		proxyHandler := &proxy.ProxyHandler{Invoker: invoker, Logger: logger}

		appHandlers = append(appHandlers, server.AppHandler{
			Name:      app.Name,
			Proxy:     proxyHandler,
			Static:    http.FileServer(http.Dir(app.Frontend.Path)),
			APIPrefix: app.Backend.APIPrefix,
		})
	}

	srv = server.New(cfg, appHandlers, logger)

	fmt.Fprintf(os.Stderr, "\n  servitor running\n\n  ➜  http://%s\n\n", cfg.Addr())

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(ctx) }()

	<-ctx.Done()
	logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "err", err)
	}
	<-startErr

	for _, done := range syncDones {
		<-done
	}

	logger.Info("shutdown complete")
}

func runUpdate() {
	ctx := context.Background()
	checker := update.NewGitHubClient("svetozarm/servitor")
	result, err := update.Run(ctx, version, checker)
	if err != nil {
		if errors.Is(err, update.ErrDevBuild) {
			fmt.Fprintln(os.Stderr, "update unavailable for development builds")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if result.UpToDate {
		fmt.Printf("already up to date: %s\n", version)
		return
	}
	fmt.Printf("updated: %s → %s\n", result.OldVersion, result.NewVersion)
}
