package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"subserver/internal/admin"
	"subserver/internal/adminstate"
	configpkg "subserver/internal/config"
	dbpkg "subserver/internal/db"
	"subserver/internal/handler"
	"subserver/internal/httpx"
	"subserver/internal/panel"
	"subserver/internal/subscription"

	_ "modernc.org/sqlite"
)

const (
	defaultHost = "127.0.0.1"
)

type Config struct {
	Host                string
	Port                int
	PanelURL            string
	PanelToken          string
	AdminToken          string
	PanelTimeout        time.Duration
	PanelCacheTTL       time.Duration
	SubPathPrefix       string
	RateLimitRPS        int
	RateLimitBurst      int
	AdminRateLimitRPS   int
	AdminRateLimitBurst int
	TrustedProxyCIDRs   []string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	loadEnvFile(envOrDefault("SUBSERVER_ENV_FILE", "/etc/subserver/subserver.env"))
	if envBool("SUBSERVER_LOAD_DOTENV") {
		loadEnvFile(".env")
	}

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}
	if err := httpx.SetTrustedProxyCIDRs(cfg.TrustedProxyCIDRs); err != nil {
		logger.Error("failed to configure trusted proxies", "error", err)
		os.Exit(1)
	}

	rootDir := resolveRootDir()

	db, err := openDatabase(rootDir)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), 5*time.Second)
	if err := dbpkg.Migrate(migrateCtx, db); err != nil {
		cancelMigrate()
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	cancelMigrate()

	configStore := configpkg.NewStore(db)
	configPath := filepath.Join(rootDir, "configs", "default.json")
	bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 5*time.Second)
	if err := bootstrapConfigs(bootstrapCtx, logger, configStore, configPath); err != nil {
		cancelBootstrap()
		logger.Error("failed to bootstrap configs", "error", err)
		os.Exit(1)
	}
	cancelBootstrap()

	templateSet, err := configStore.LoadTemplateSet(context.Background())
	if err != nil {
		logger.Error("failed to load config template", "error", err)
		os.Exit(1)
	}

	var panelCache *panel.Cache
	if cfg.PanelCacheTTL > 0 {
		panelCache = panel.NewCache(cfg.PanelCacheTTL)
	}
	panelClient := panel.NewClient(cfg.PanelURL, cfg.PanelToken, cfg.PanelTimeout, panelCache)
	builder := configpkg.NewBuilder(templateSet)

	headersPath := filepath.Join(rootDir, "data", "header_overrides.json")
	headerStore, err := adminstate.LoadHeaderStore(db)
	if err != nil {
		logger.Error("failed to load header overrides", "error", err)
	}
	if err := bootstrapHeaders(context.Background(), logger, headerStore, headersPath); err != nil {
		logger.Error("failed to bootstrap header overrides", "error", err)
	}
	if headerStore != nil {
		currentHeaders := headerStore.HeaderOverridesSet()
		updatedHeaders, changed := adminstate.EnsureMihomoOverrides(currentHeaders)
		if changed {
			if err := headerStore.SaveSet(updatedHeaders); err != nil {
				logger.Error("failed to persist mihomo header overrides", "error", err)
				os.Exit(1)
			}
		}
	}

	var limiter *handler.RateLimiter
	if cfg.RateLimitRPS > 0 {
		burst := cfg.RateLimitBurst
		if burst <= 0 {
			burst = cfg.RateLimitRPS
		}
		limiter = handler.NewRateLimiter(cfg.RateLimitRPS, burst, 2*time.Minute)
	}
	var adminLimiter *handler.RateLimiter
	if cfg.AdminRateLimitRPS > 0 {
		burst := cfg.AdminRateLimitBurst
		if burst <= 0 {
			burst = cfg.AdminRateLimitRPS
		}
		adminLimiter = handler.NewRateLimiter(cfg.AdminRateLimitRPS, burst, 2*time.Minute)
	}

	subscriptionServer := handler.NewServer(handler.Options{
		SubPathPrefix:   cfg.SubPathPrefix,
		Panel:           panelClient,
		Builder:         builder,
		Logger:          logger,
		RateLimiter:     limiter,
		HeaderOverrides: headerStore,
	})

	adminServer := admin.NewServer(admin.Options{
		RootDir:     rootDir,
		ConfigStore: configStore,
		Panel:       panelClient,
		Builder:     builder,
		Headers:     headerStore,
		Logger:      logger,
		Token:       cfg.AdminToken,
		RateLimiter: adminLimiter,
	})

	mux := http.NewServeMux()
	mux.Handle("/admin/", http.StripPrefix("/admin", adminServer))
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.Handle("/", subscriptionServer)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	logger.Info("server starting", "addr", addr)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	exitCode := 0
	received := false
	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		received = true
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		exitCode = 1
	}

	if !received {
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			exitCode = 1
		}
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func loadConfig() (*Config, error) {
	host := envOrDefault("SUBSERVER_BIND", defaultHost)
	port, err := envInt("SUBSERVER_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid SUBSERVER_PORT: %w", err)
	}

	panelCacheSeconds, err := envInt("PANEL_CACHE_TTL", 60)
	if err != nil {
		return nil, fmt.Errorf("invalid PANEL_CACHE_TTL: %w", err)
	}

	rateLimitRPS, err := envInt("RATE_LIMIT_RPS", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_RPS: %w", err)
	}

	rateLimitBurst, err := envInt("RATE_LIMIT_BURST", rateLimitRPS)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_BURST: %w", err)
	}
	adminRateLimitRPS, err := envInt("ADMIN_RATE_LIMIT_RPS", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid ADMIN_RATE_LIMIT_RPS: %w", err)
	}
	adminRateLimitBurst, err := envInt("ADMIN_RATE_LIMIT_BURST", adminRateLimitRPS)
	if err != nil {
		return nil, fmt.Errorf("invalid ADMIN_RATE_LIMIT_BURST: %w", err)
	}

	panelURL := strings.TrimRight(envOrDefault("PANEL_URL", ""), "/")
	if panelURL == "" {
		return nil, errors.New("PANEL_URL environment variable is required")
	}
	if err := validatePanelURL(panelURL); err != nil {
		return nil, fmt.Errorf("invalid PANEL_URL: %w", err)
	}
	panelToken := envOrDefault("PANEL_TOKEN", "")
	if panelToken == "" {
		return nil, errors.New("PANEL_TOKEN environment variable is required")
	}
	adminToken := envOrDefault("ADMIN_TOKEN", "")
	if strings.TrimSpace(adminToken) == "" {
		return nil, errors.New("ADMIN_TOKEN environment variable is required")
	}

	subPathPrefix := envOrDefault("SUB_PATH_PREFIX", "/")
	if !strings.HasPrefix(subPathPrefix, "/") {
		subPathPrefix = "/" + subPathPrefix
	}
	if !strings.HasSuffix(subPathPrefix, "/") {
		subPathPrefix = subPathPrefix + "/"
	}

	return &Config{
		Host:                host,
		Port:                port,
		PanelURL:            panelURL,
		PanelToken:          panelToken,
		AdminToken:          adminToken,
		PanelTimeout:        10 * time.Second,
		PanelCacheTTL:       time.Duration(panelCacheSeconds) * time.Second,
		SubPathPrefix:       subPathPrefix,
		RateLimitRPS:        rateLimitRPS,
		RateLimitBurst:      rateLimitBurst,
		AdminRateLimitRPS:   adminRateLimitRPS,
		AdminRateLimitBurst: adminRateLimitBurst,
		TrustedProxyCIDRs:   envCSV("TRUSTED_PROXY_CIDRS"),
	}, nil
}

func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Strip surrounding quotes (single or double).
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

func openDatabase(rootDir string) (*sql.DB, error) {
	dsn := strings.TrimSpace(envOrDefault("SUBSERVER_DB_DSN", ""))
	if dsn == "" {
		dbPath := strings.TrimSpace(envOrDefault("SUBSERVER_DB_PATH", ""))
		if dbPath == "" {
			dbPath = filepath.Join(rootDir, "data", "subserver.db")
		} else if !filepath.IsAbs(dbPath) {
			dbPath = filepath.Join(rootDir, dbPath)
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, err
		}
		dsn = fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func bootstrapConfigs(ctx context.Context, logger *slog.Logger, store *configpkg.Store, path string) error {
	if store == nil {
		return errors.New("config store is nil")
	}
	exists, err := store.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	raw, err := configpkg.LoadJSON(path)
	if err != nil {
		return err
	}
	xrayTemplateSet, err := configpkg.ParseTemplateSet(raw)
	if err != nil {
		return err
	}
	templateSet := configpkg.TemplateLibrary{}.WithCore(configpkg.CoreXray, xrayTemplateSet)
	if err := store.SaveTemplateSet(ctx, templateSet); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("config templates migrated", "path", path)
	}
	return nil
}

func bootstrapHeaders(ctx context.Context, logger *slog.Logger, store *adminstate.HeaderStore, path string) error {
	if store == nil {
		return nil
	}
	exists, err := store.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			empty := adminstate.CoreHeaderOverridesSet{
				Xray: adminstate.HeaderOverridesSet{
					Default: map[string]subscription.HeaderOverride{},
					Squads:  map[string]map[string]subscription.HeaderOverride{},
				},
				Mihomo: adminstate.HeaderOverridesSet{
					Default: map[string]subscription.HeaderOverride{},
					Squads:  map[string]map[string]subscription.HeaderOverride{},
				},
			}
			if err := store.SaveSet(empty); err != nil {
				return err
			}
			if logger != nil {
				logger.Info("header overrides initialized", "path", path)
			}
			return nil
		}
		return err
	}
	overrides, err := adminstate.DecodeOverridesSet(content)
	if err != nil {
		return err
	}
	if err := store.SaveSet(overrides); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("header overrides migrated", "path", path)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func envCSV(key string) []string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func envBool(key string) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func validatePanelURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("host is required")
	}
	if parsed.User != nil {
		return errors.New("embedded credentials are not allowed")
	}
	return nil
}

func resolveRootDir() string {
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		if fileExists(filepath.Join(exeDir, "configs", "default.json")) {
			return exeDir
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		return cwd
	}
	return "."
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
