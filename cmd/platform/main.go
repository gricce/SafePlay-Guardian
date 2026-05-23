package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/api"
	"github.com/gricce/SafePlay-Guardian/internal/auth"
	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/discovery"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/agent"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/noop"
	"github.com/gricce/SafePlay-Guardian/internal/event"
	"github.com/gricce/SafePlay-Guardian/internal/person"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
	"github.com/gricce/SafePlay-Guardian/internal/store"
	"github.com/gricce/SafePlay-Guardian/migrations"
	"github.com/kardianos/service"
	_ "modernc.org/sqlite"
)

const (
	serviceName    = "safeplay-guardian"
	serviceDisplay = "SafePlay Guardian"
	serviceDescr   = "Family device management background service (see CORE_PLATFORM.md)."
)

type config struct {
	dataDir          string
	addr             string
	scanInterval     time.Duration
	sweepInterval    time.Duration
	disableScan      bool
	disableSweep     bool
	retentionWindow  time.Duration
	retentionEvery   time.Duration
	disableRetention bool
}

func parseConfig() config {
	defaultData, _ := os.UserConfigDir()
	if defaultData == "" {
		defaultData = "."
	}
	defaultData = filepath.Join(defaultData, "SafePlayGuardian")

	var c config
	flag.StringVar(&c.dataDir, "data-dir", defaultData, "directory for sqlite database and runtime state")
	flag.StringVar(&c.addr, "addr", "127.0.0.1:7878", "HTTP listen address")
	flag.DurationVar(&c.scanInterval, "scan-interval", 30*time.Second, "passive scan cadence (ARP + mDNS)")
	flag.DurationVar(&c.sweepInterval, "sweep-interval", 5*time.Minute, "active ping-sweep cadence")
	flag.BoolVar(&c.disableScan, "no-scan", false, "disable LAN discovery scanner")
	flag.BoolVar(&c.disableSweep, "no-sweep", false, "disable active ping-sweep (passive ARP still runs)")
	flag.DurationVar(&c.retentionWindow, "retention-window", 14*24*time.Hour, "max age of raw events before deletion")
	flag.DurationVar(&c.retentionEvery, "retention-every", 6*time.Hour, "how often the retention sweep runs")
	flag.BoolVar(&c.disableRetention, "no-retention", false, "disable the event retention sweep (events never expire)")
	flag.Parse()
	return c
}

func openDB(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := filepath.Join(dataDir, "platform.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

type eventUsage struct{ store *event.Store }

func (u eventUsage) UsedSince(ctx context.Context, deviceID string, start time.Time) (time.Duration, error) {
	return u.store.UsageSince(ctx, deviceID, start)
}

// program adapts the service to the kardianos/service lifecycle. Start
// returns quickly (the framework requires it); the real work runs in a
// goroutine until Stop cancels the context.
type program struct {
	cfg    config
	logger *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func (p *program) Start(_ service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := runService(ctx, p.cfg, p.logger); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Error("service exited with error", "err", err)
		}
	}()
	return nil
}

func (p *program) Stop(_ service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		p.logger.Warn("service stop timed out after 10s")
	}
	return nil
}

func main() {
	mode, ok := peelSubcommand()
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\nusage: %s [install|uninstall|start|stop|restart|status] [flags]\n", mode, filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	cfg := parseConfig()

	// Resolve to absolute so the service manager has an unambiguous path
	// regardless of which directory the parent ran the install command from.
	if abs, err := filepath.Abs(cfg.dataDir); err == nil {
		cfg.dataDir = abs
	}

	logger, closeLog := setupLogger(cfg)
	defer closeLog()

	prg := &program{cfg: cfg, logger: logger}
	svc, err := service.New(prg, buildServiceConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "service init failed: %v\n", err)
		os.Exit(1)
	}

	switch mode {
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, mode); err != nil {
			fmt.Fprintf(os.Stderr, "%s failed: %v\n", mode, err)
			os.Exit(1)
		}
		fmt.Printf("%s ok\n", mode)
	case "status":
		st, err := svc.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(statusString(st))
	case "":
		if err := svc.Run(); err != nil {
			logger.Error("service run failed", "err", err)
			os.Exit(1)
		}
	}
}

// peelSubcommand returns (mode, recognized). recognized=false signals an
// unknown first arg that looks like a subcommand attempt (no leading dash) so
// main can reject it instead of silently treating it as the foreground run.
func peelSubcommand() (string, bool) {
	if len(os.Args) < 2 {
		return "", true
	}
	if strings.HasPrefix(os.Args[1], "-") {
		return "", true
	}
	switch os.Args[1] {
	case "install", "uninstall", "start", "stop", "restart", "status":
		mode := os.Args[1]
		os.Args = append(os.Args[:1], os.Args[2:]...)
		return mode, true
	}
	return os.Args[1], false
}

func buildServiceConfig(cfg config) *service.Config {
	args := []string{
		"-data-dir", cfg.dataDir,
		"-addr", cfg.addr,
		"-scan-interval", cfg.scanInterval.String(),
		"-sweep-interval", cfg.sweepInterval.String(),
		"-retention-window", cfg.retentionWindow.String(),
		"-retention-every", cfg.retentionEvery.String(),
	}
	if cfg.disableScan {
		args = append(args, "-no-scan")
	}
	if cfg.disableSweep {
		args = append(args, "-no-sweep")
	}
	if cfg.disableRetention {
		args = append(args, "-no-retention")
	}
	return &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplay,
		Description: serviceDescr,
		Arguments:   args,
	}
}

func statusString(s service.Status) string {
	switch s {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown (not installed?)"
	}
}

// setupLogger writes JSON-structured logs to stdout when the binary is
// running interactively (foreground / dev), and to a rotating-friendly
// append-only file under the data directory when running as a managed
// service — where stdout typically goes nowhere a parent can find.
func setupLogger(cfg config) (*slog.Logger, func()) {
	var out io.Writer = os.Stdout
	closer := func() {}
	if !service.Interactive() {
		_ = os.MkdirAll(cfg.dataDir, 0o700)
		f, err := os.OpenFile(filepath.Join(cfg.dataDir, "platform.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			out = f
			closer = func() { _ = f.Close() }
		}
	}
	logger := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	return logger, closer
}

// runService is the meat of the binary: open the DB, run migrations, build
// the registry / coordinator / scanner / HTTP server, and block until ctx
// is canceled by Stop.
func runService(ctx context.Context, cfg config, logger *slog.Logger) error {
	db, err := openDB(cfg.dataDir)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer db.Close()

	if err := store.Migrate(db, migrations.FS); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	registry := device.NewSQLiteRegistry(db)
	people := person.NewStore(db)
	policyStore := policy.NewSQLiteStore(db)
	eventStore := event.NewStore(db)
	authStore := auth.NewStore(db)

	eventLogger := &event.Logger{Store: eventStore, Lookup: policyStore, Slog: logger}

	resolver := &policy.Resolver{
		Devices: registry,
		Store:   policyStore,
		Usage:   eventUsage{store: eventStore},
	}
	agentBackend := agent.New()
	coordinator := &enforce.Coordinator{
		Backends: []enforce.Enforcer{agentBackend, noop.Backend{Logger: logger}},
		Resolver: resolver,
		Devices:  registry,
		Logger:   logger,
	}

	registry.SetObserveHook(func(ctx context.Context, deviceID string, obs device.Observation) {
		d, err := registry.Get(ctx, deviceID)
		if err != nil {
			return
		}
		_ = eventLogger.Append(ctx, event.Event{
			Kind:     event.KindDeviceSeen,
			DeviceID: deviceID,
			PersonID: d.PersonID,
			At:       obs.SeenAt,
			Attrs:    map[string]any{"source": obs.Source},
		})
	})

	coordinator.OnRule = func(ctx context.Context, deviceID string, ro enforce.RuleOutcome) {
		var kind event.Kind
		sev := event.SeverityInfo
		switch ro.Outcome {
		case enforce.RuleApplied:
			kind = event.KindPolicyApplied
		case enforce.RuleFailed:
			kind = event.KindPolicyFailed
			sev = event.SeverityError
		case enforce.RuleUnenforceable:
			kind = event.KindPolicyUnenforceable
			sev = event.SeverityWarn
		}
		var pid *string
		if d, err := registry.Get(ctx, deviceID); err == nil {
			pid = d.PersonID
		}
		attrs := map[string]any{
			"capability": string(ro.Capability),
			"backend":    ro.Backend,
		}
		if ro.Note != "" {
			attrs["note"] = ro.Note
		}
		if ro.Error != "" {
			attrs["error"] = ro.Error
		}
		_ = eventLogger.Append(ctx, event.Event{
			Kind: kind, Severity: sev,
			DeviceID: deviceID, PersonID: pid,
			Attrs: attrs,
		})
	}

	server := &api.Server{
		Registry:     registry,
		People:       people,
		PolicyStore:  policyStore,
		Resolver:     resolver,
		Coordinator:  coordinator,
		Events:       eventStore,
		EventLogger:  eventLogger,
		Auth:         authStore,
		AgentBackend: agentBackend,
		Logger:       logger,
	}

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	_ = authStore.PurgeExpired(ctx)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.addr, "data_dir", cfg.dataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	if !cfg.disableScan {
		startScanner(ctx, cfg, registry, logger)
	}
	if !cfg.disableRetention {
		go event.RunRetention(ctx, eventStore, cfg.retentionWindow, cfg.retentionEvery, logger)
	}

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			logger.Error("http server error", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

func startScanner(ctx context.Context, cfg config, registry *device.SQLiteRegistry, logger *slog.Logger) {
	hide := discovery.LocalMACs()
	logger.Info("scanner starting",
		"interval", cfg.scanInterval.String(),
		"sweep_enabled", !cfg.disableSweep,
		"sweep_interval", cfg.sweepInterval.String(),
		"hide_local_macs", len(hide),
	)

	scanner := &discovery.Scanner{
		Reconciler: registry,
		Sources: []discovery.Source{
			discovery.ARPSource{},
			discovery.MDNSSource{},
		},
		Interval:  cfg.scanInterval,
		MinCycles: 2,
		HideMACs:  hide,
		Logger:    logger,
	}
	go func() {
		if err := scanner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scanner stopped", "err", err)
		}
	}()

	if cfg.disableSweep {
		return
	}
	net := discovery.PrimaryIPv4Network()
	if net == nil {
		logger.Warn("no IPv4 interface for sweep; skipping")
		return
	}
	go func() {
		t := time.NewTicker(cfg.sweepInterval)
		defer t.Stop()
		for {
			discovery.PingSweep(ctx, net, 32, 800*time.Millisecond)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
