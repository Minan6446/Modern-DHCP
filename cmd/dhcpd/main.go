package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/automation"
	"modern-dhcp/internal/automation/workflow"
	workflowactions "modern-dhcp/internal/automation/workflow/actions"
	"modern-dhcp/internal/backup"
	"modern-dhcp/internal/collab"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/db"
	"modern-dhcp/internal/dhcpv4"
	"modern-dhcp/internal/events"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/iot"
	iotregistry "modern-dhcp/internal/iot/registry"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/mobility"
	"modern-dhcp/internal/mobility/mdm"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/ops"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/rbac"
	"modern-dhcp/internal/relay"
	"modern-dhcp/internal/replication"
	"modern-dhcp/internal/reporting"
	securityguard "modern-dhcp/internal/security/guard"
	securitypolicy "modern-dhcp/internal/security/policy"
	securityquarantine "modern-dhcp/internal/security/quarantine"
	"modern-dhcp/internal/server"
	"modern-dhcp/internal/storage"
	"modern-dhcp/internal/superadmin"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/internal/visualization"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	dhcpv4Workers := flag.Int("dhcpv4-workers", 0, "Override DHCPv4 worker count")
	dhcpv4Queue := flag.Int("dhcpv4-queue", 0, "Override DHCPv4 datagram queue depth")
	listenerShards := flag.Int("listeners", 0, "Override DHCPv4 listener shard count (SO_REUSEPORT)")
	rxBatch := flag.Int("rx-batch", 0, "Override DHCPv4 UDP receive batch size")
	reusePortFlag := boolFlag{value: true}
	flag.Var(&reusePortFlag, "reuseport", "Control SO_REUSEPORT usage for DHCPv4 listeners (true/false)")
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}
	if *dhcpv4Workers > 0 {
		cfg.Service.DHCPv4.WorkerCount = *dhcpv4Workers
	}
	if *dhcpv4Queue > 0 {
		cfg.Service.DHCPv4.QueueDepth = *dhcpv4Queue
	}
	if *listenerShards > 0 {
		cfg.Service.DHCPv4.ListenerFanout = *listenerShards
	}
	if *rxBatch > 0 {
		cfg.Service.DHCPv4.RXBatchSize = *rxBatch
	}
	if reusePortFlag.WasSet() {
		cfg.Service.DHCPv4.ReusePort = boolPtr(reusePortFlag.Bool())
	}

	ctx := context.Background()
	mysqlConfig := cfg.MySQL
	postgresConfig := cfg.Postgres
	var (
		sqlDB        *sqlx.DB
		dbHealthName = "mysql"
	)
	plan, planErr := storage.DeriveRelationalPlan(cfg.Storage)
	if planErr != nil {
		logger.Warn("derive storage plan", zap.Error(planErr))
	} else {
		logger.Info("storage plan selected",
			zap.String("driver", plan.Driver),
			zap.String("source", plan.Source),
			zap.String("failover", plan.FailoverHint),
		)
		switch plan.Driver {
		case storage.DriverPostgres:
			if plan.DSN != "" {
				postgresConfig.DSN = plan.DSN
			}
			dbHandle, err := db.NewPostgres(ctx, postgresConfig)
			if err != nil {
				logger.Fatal("connect postgres", zap.Error(err))
			}
			sqlDB = dbHandle
			dbHealthName = "postgres"
		case storage.DriverMySQL:
			if plan.DSN != "" {
				mysqlConfig.DSN = plan.DSN
			}
			dbHandle, err := db.NewMySQL(ctx, mysqlConfig)
			if err != nil {
				logger.Fatal("connect mysql", zap.Error(err))
			}
			sqlDB = dbHandle
			dbHealthName = "mysql"
		default:
			logger.Warn("relational driver not supported, falling back to mysql", zap.String("driver", plan.Driver))
		}
	}
	if sqlDB == nil {
		logger.Info("using fallback mysql configuration block")
		dbHandle, err := db.NewMySQL(ctx, mysqlConfig)
		if err != nil {
			logger.Fatal("connect mysql", zap.Error(err))
		}
		sqlDB = dbHandle
		dbHealthName = "mysql"
	}
	defer sqlDB.Close()
	tenantRouter := storage.NewTenantRouter(sqlDB, cfg.Tenancy)
	defer tenantRouter.Close()
	metricCollector := metrics.NewCollector(cfg.Service.Name)
	journal := replication.NewKafkaJournal(cfg.HA.Replication, logger)
	if journal != nil {
		defer journal.Close()
	}
	failoverManager := failover.NewManager(cfg.HA, logger).WithMetricsCollector(metricCollector)

	healthHooks := []server.HealthHook{
		server.NewHealthHook(dbHealthName, func(ctx context.Context) error { return sqlDB.PingContext(ctx) }),
	}
	backupManager := backup.NewManager(cfg.Backup, logger)
	if backupManager != nil {
		healthHooks = append(healthHooks, backupManager)
	}

	poolRepo := pool.NewRepository(sqlDB, pool.WithTenantRouter(tenantRouter))
	leaseRepo := lease.NewRepository(sqlDB, lease.WithTenantRouter(tenantRouter))
	quotaRepo := tenant.NewQuotaRepository(sqlDB)
	quotaSvc := tenant.NewQuotaService(quotaRepo, poolRepo, leaseRepo, logger)
	authRepo := auth.NewRepository(sqlDB)
	authSvc := auth.NewService(authRepo, logger)
	configureAuthProviders(cfg.Auth, authSvc, authRepo, logger)
	rbacRepo := rbac.NewRepository(sqlDB)
	rbacResolver := rbac.NewResolver(rbacRepo, rbac.ResolverOptions{CacheTTL: cfg.RBAC.CacheTTL, Logger: logger})
	var notificationScheduler lease.NotificationScheduler
	if len(cfg.Kafka.Brokers) > 0 && cfg.Kafka.Topics.LeaseEvents != "" {
		notificationScheduler = lease.NewKafkaNotificationScheduler(cfg.Kafka.Brokers, cfg.Kafka.Topics.LeaseEvents, cfg.Kafka.ClientID, logger)
	}
	if notificationScheduler == nil {
		notificationScheduler = lease.NewLoggingScheduler(logger)
	}
	auditRepo := audit.NewRepository(sqlDB)
	auditSvc := audit.NewService(auditRepo, logger)
	leaseOpts := []lease.ServiceOption{lease.WithNotificationScheduler(notificationScheduler)}
	if metricCollector != nil {
		leaseOpts = append(leaseOpts, lease.WithMetricsCollector(metricCollector))
	}
	if journal != nil {
		leaseOpts = append(leaseOpts, lease.WithReplicationJournal(journal))
	}
	leaseOpts = append(leaseOpts, lease.WithAuditService(auditSvc))
	leaseOpts = append(leaseOpts, lease.WithExhaustionConfig(cfg.Security.Exhaustion))
	leaseOpts = append(leaseOpts, lease.WithQuotaEnforcer(quotaSvc))
	if cfg.Security.ConflictPrevention.Enabled {
		prober := netutil.NewICMPProber(cfg.Security.ConflictPrevention.ProbeTimeout)
		leaseOpts = append(leaseOpts, lease.WithConflictPrevention(cfg.Security.ConflictPrevention, prober))
	}
	leaseSvc := lease.NewService(leaseRepo, poolRepo, cfg.Policy, logger, leaseOpts...)
	var reclaimer *lease.Reclaimer
	if cfg.Policy.Lifecycle.AutoReclaim.Enabled {
		reclaimer = lease.NewReclaimer(leaseRepo, logger, lease.ReclaimerOptions{
			Interval:    cfg.Policy.Lifecycle.AutoReclaim.Interval,
			GracePeriod: cfg.Policy.Lifecycle.AutoReclaim.GracePeriod,
			BatchSize:   cfg.Policy.Lifecycle.AutoReclaim.BatchSize,
		})
	}
	policyRepo := policy.NewRepository(sqlDB)
	policyEngine := policy.NewEngine(policyRepo, logger)
	var policyPublishers []events.PolicyPublisher
	if len(cfg.Kafka.Brokers) > 0 && cfg.Kafka.Topics.PolicyEvents != "" {
		policyPublishers = append(policyPublishers, events.NewKafkaPublisher(cfg.Kafka.Brokers, cfg.Kafka.Topics.PolicyEvents, logger))
	}
	if len(cfg.Events.PolicyWebhooks) > 0 {
		policyPublishers = append(policyPublishers, events.NewWebhookPublisher(cfg.Events.PolicyWebhooks, logger))
	}
	var policyPublisher events.PolicyPublisher
	switch len(policyPublishers) {
	case 0:
		// noop
	case 1:
		policyPublisher = policyPublishers[0]
	default:
		policyPublisher = events.NewMultiPublisher(policyPublishers...)
	}
	policySvc := policy.NewService(policyRepo, policyEngine, policyPublisher, logger)
	securityPolicyRepo := securitypolicy.NewRepository(sqlDB)
	securityPolicyEvaluator := securitypolicy.NewEvaluator(securityPolicyRepo, securitypolicy.EvaluatorOptions{
		CacheTTL: cfg.Security.Policy.CacheTTL,
		Logger:   logger,
	})
	securityPolicySvc := securitypolicy.NewService(securityPolicyRepo, logger, securitypolicy.WithEvaluator(securityPolicyEvaluator))
	poolSvc := pool.NewService(poolRepo, logger, pool.WithQuotaEnforcer(quotaSvc))
	var collabHub *collab.Hub
	if cfg.UI.Collaboration.Enabled {
		collabHub = collab.NewHub(logger, collab.HubOptions{
			Repository:        collab.NewRepository(sqlDB),
			PresenceHeartbeat: cfg.UI.Collaboration.PresenceHeartbeat,
			OptimisticLockTTL: cfg.UI.Collaboration.OptimisticLockTTL,
			MaxSubscribers:    cfg.UI.Collaboration.MaxConcurrentLocks,
		})
	}
	requestTracker := monitoring.NewRequestTracker()
	rateTracker := monitoring.NewRateLimitTracker(512)
	snoopingTracker := monitoring.NewSnoopingTracker(512)
	monitorAggregator := monitoring.NewAggregator(monitoring.Options{
		PoolService:     poolSvc,
		LeaseRepo:       leaseRepo,
		RequestTracker:  requestTracker,
		RateTracker:     rateTracker,
		SnoopingTracker: snoopingTracker,
		Logger:          logger,
	})
	alertFeed := monitoring.NewAlertFeed(250)
	alertFeed.Seed("tenant-acme", monitoring.SampleAlertEntries("tenant-acme"))
	reportSvc := reporting.NewService(reporting.Options{
		Monitor:      monitorAggregator,
		Audit:        auditSvc,
		LeaseHistory: leaseSvc,
		ExportPath:   cfg.OpsSupport.ImportExport.StoragePath,
		Logger:       logger,
	})
	defer reportSvc.Close()
	visualizationSvc := visualization.NewService(visualization.Options{
		Logger:       logger,
		PoolService:  poolSvc,
		LeaseReader:  leaseRepo,
		MaxNodes:     cfg.UI.Visualization.MaxCanvasNodes,
		HeatmapLimit: cfg.UI.Visualization.MaxCanvasNodes,
	})
	var iotRegistrySvc *iotregistry.Service
	if cfg.IoT.Registry.Enabled {
		iotRepo := iot.NewRepository(sqlDB, iot.WithTenantRouter(tenantRouter))
		defaults := iotregistry.Defaults{
			SleepInterval: cfg.IoT.Registry.DefaultSleepInterval,
			OfflineWindow: cfg.IoT.Registry.DefaultOfflineWindow,
		}
		iotRegistrySvc = iotregistry.NewService(iotRepo, logger, iotregistry.WithDefaults(defaults))
	}
	alertController, alertManager := setupAlerting(cfg.Monitoring.Alerting, monitorAggregator, rateTracker, alertFeed, logger)
	notificationDispatcher := setupNotifications(cfg.Notifications, logger)
	jobStore := automation.NewJobStore(sqlDB)
	automationSvc := setupAutomation(cfg.Automation, cfg.Notifications, notificationDispatcher, monitorAggregator, jobStore, logger)
	workflowRepo := workflow.NewRepository(sqlDB)
	workflowSvc, err := workflow.NewService(workflow.ServiceOptions{
		Repository: workflowRepo,
		Dispatcher: automationSvc,
		Logger:     logger,
	})
	if err != nil {
		logger.Fatal("init workflow service", zap.Error(err))
	}
	workflowExecutor := workflow.NewExecutor(workflow.ExecutorOptions{
		Logger: logger.Named("workflow-executor"),
		TaskHandlers: map[string]workflow.TaskHandler{
			workflowactions.ActionLeaseReclaim: workflowactions.NewLeaseReclaimHandler(leaseRepo, logger.Named("workflow-task-lease")),
		},
	})
	if automationSvc != nil {
		automationSvc.RegisterHandler(automation.JobWorkflowExecution, workflowExecutor)
	} else {
		logger.Warn("automation disabled; workflow jobs cannot be scheduled")
	}
	var apiKeys map[string]server.APIKeyMetadata
	if len(cfg.Auth.APIKeys) > 0 {
		apiKeys = make(map[string]server.APIKeyMetadata, len(cfg.Auth.APIKeys))
		for token, meta := range cfg.Auth.APIKeys {
			apiKeys[token] = server.APIKeyMetadata{
				DisplayName: meta.DisplayName,
				Role:        meta.Role,
				PrincipalID: meta.PrincipalID,
			}
		}
	}
	uiOptions := buildUIOptions(cfg.UI)
	opsSupportOpts := buildOpsSupportOptions(cfg.OpsSupport)
	opsSettingsDefaults := ops.SystemSettings{
		Theme:             cfg.OpsSupport.System.Theme,
		Locale:            cfg.OpsSupport.System.Locale,
		MaintenanceMode:   cfg.OpsSupport.System.MaintenanceMode,
		MaintenanceWindow: cfg.OpsSupport.System.MaintenanceWindow,
		Announcement:      cfg.OpsSupport.System.Announcement,
		UpdatedAt:         time.Now().UTC(),
		UpdatedBy:         "bootstrap",
	}
	opsSettingsStore := ops.NewSQLSettingsStore(sqlDB, opsSettingsDefaults)
	superAdminOpts := server.SuperAdminOptions{}
	if cfg.Auth.SuperAdmin.Enabled {
		if cfg.Auth.SuperAdmin.APIKeyRef == "" {
			logger.Fatal("super admin apiKeyRef required when enabled")
		}
		if _, ok := cfg.Auth.APIKeys[cfg.Auth.SuperAdmin.APIKeyRef]; !ok {
			logger.Fatal("super admin apiKeyRef missing from auth.apiKeys", zap.String("apiKeyRef", cfg.Auth.SuperAdmin.APIKeyRef))
		}
		manager, err := superadmin.NewManager(cfg.Auth.SuperAdmin.StateFile)
		if err != nil {
			logger.Fatal("init super admin manager", zap.Error(err))
		}
		username := strings.TrimSpace(cfg.Auth.SuperAdmin.Username)
		if username == "" {
			username = superadmin.DefaultUsername
		}
		superAdminOpts = server.SuperAdminOptions{
			Enabled:  true,
			Username: username,
			APIKey:   cfg.Auth.SuperAdmin.APIKeyRef,
			Manager:  manager,
		}
	}
	serverOpts := server.Options{
		RequireAuth:        cfg.Auth.Enabled,
		APIKeys:            apiKeys,
		Metrics:            metricCollector,
		PrometheusEnabled:  cfg.Monitoring.Prometheus.Enabled,
		PrometheusEndpoint: cfg.Monitoring.Prometheus.Endpoint,
		RBAC: server.RBACOptions{
			Resolver:            rbacResolver,
			EnforceCapabilities: cfg.RBAC.EnforceCapabilities,
			ShadowMode:          cfg.RBAC.ShadowMode,
		},
		CORS: server.CORSOptions{
			Enabled:        cfg.Service.CORS.Enabled,
			AllowedOrigins: cfg.Service.CORS.AllowedOrigins,
		},
		HealthHooks:        healthHooks,
		Coordinator:        failoverManager,
		FailoverController: failoverManager,
		HARunbooks:         cfg.HA.Runbooks,
		Monitoring: server.MonitoringOptions{
			Aggregator:      monitorAggregator,
			AlertFeed:       alertFeed,
			AlertManager:    alertManager,
			AlertController: alertController,
		},
		Automation:       automationSvc,
		Workflow:         workflowSvc,
		OpsSupport:       opsSupportOpts,
		OpsSettingsStore: opsSettingsStore,
		UI:               uiOptions,
		SuperAdmin:       superAdminOpts,
		TokenProvider:    authSvc,
	}
	apiServer, err := server.NewHTTPServer(logger, serverOpts, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, auditSvc, reportSvc, visualizationSvc, iotRegistrySvc, collabHub, quotaSvc, rbacRepo, authSvc)
	if err != nil {
		logger.Fatal("init http server", zap.Error(err))
	}
	quarantineSink := securityquarantine.NewLeaseSink(leaseSvc, logger)
	var guardPolicyEvaluator securitypolicy.Evaluator
	if cfg.Security.Policy.Enabled {
		guardPolicyEvaluator = securityPolicyEvaluator
	}
	dhcpGuard := securityguard.NewFromConfig(cfg.Security, metricCollector, logger, quarantineSink, auditSvc, guardPolicyEvaluator, rateTracker, snoopingTracker)
	relayParser := relay.NewOption82Parser(cfg.Relay.Option82)
	relaySelector := relay.NewSelector(cfg.Relay, logger)
	relayAuth := relay.NewAuthenticator(cfg.Relay.Authentication, logger)
	affinityCache := mobility.NewAffinityCache(cfg.Policy.Lifecycle.MobilityAffinityTTL, metricCollector, logger)
	fingerprinter := mobility.NewDetector(cfg.Policy.MobileProfiles)
	mdmService := mdm.NewService(cfg.Security.MDM, logger)
	dhcpHandler := dhcpv4.NewHandler(leaseSvc, policyEngine, poolSvc, metricCollector, dhcpGuard, relayAuth, relaySelector, logger, requestTracker, affinityCache, fingerprinter, mdmService)
	serverIP := net.ParseIP(cfg.Service.BindAddress)
	tenantID := cfg.Service.NodeID
	if tenantID == "" {
		tenantID = "default"
	}
	relayPartitioner := relay.NewPartitioner(cfg.Relay.LoadBalancing, tenantID, logger)
	dhcpv4Runtime := cfg.Service.DHCPv4
	reusePortEnabled := true
	if dhcpv4Runtime.ReusePort != nil {
		reusePortEnabled = *dhcpv4Runtime.ReusePort
	}
	v4Server := dhcpv4.NewServer(dhcpv4.Options{
		BindAddress:    cfg.Service.BindAddress,
		Port:           cfg.Service.DHCPv4Port,
		TenantID:       tenantID,
		ServerIP:       serverIP,
		RelayParser:    relayParser,
		Partitioner:    relayPartitioner,
		WorkerCount:    dhcpv4Runtime.WorkerCount,
		QueueDepth:     dhcpv4Runtime.QueueDepth,
		ListenerFanout: dhcpv4Runtime.ListenerFanout,
		RXBatchSize:    dhcpv4Runtime.RXBatchSize,
		ReusePort:      reusePortEnabled,
	}, dhcpHandler, logger)

	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	defer runtimeCancel()
	failoverManager.Start(runtimeCtx)
	defer failoverManager.Stop()
	if alertController != nil {
		alertController.Start(runtimeCtx)
	}
	if backupManager != nil {
		go backupManager.Start(runtimeCtx)
	}
	if mdmService != nil {
		go mdmService.Run(runtimeCtx)
	}
	if automationSvc != nil {
		if err := automationSvc.Start(runtimeCtx); err != nil {
			logger.Fatal("start automation", zap.Error(err))
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := automationSvc.Stop(stopCtx); err != nil {
				logger.Warn("automation stop", zap.Error(err))
			}
		}()
	}

	go func() {
		if err := apiServer.Start(cfg.Service.APIPort); err != nil {
			logger.Fatal("http server stopped", zap.Error(err))
		}
	}()

	if reclaimer != nil {
		go reclaimer.Run(runtimeCtx)
	}

	go func() {
		if err := v4Server.ListenAndServe(runtimeCtx); err != nil {
			logger.Error("dhcpv4 server stopped", zap.Error(err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	runtimeCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	if collabHub != nil {
		if err := collabHub.Shutdown(shutdownCtx); err != nil {
			logger.Warn("collab hub shutdown", zap.Error(err))
		}
	}
	if err := v4Server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("dhcpv4 shutdown failed", zap.Error(err))
	}
	if policyPublisher != nil {
		if err := policyPublisher.Close(shutdownCtx); err != nil {
			logger.Warn("policy publisher close", zap.Error(err))
		}
	}
	if notificationScheduler != nil {
		if err := notificationScheduler.Close(shutdownCtx); err != nil {
			logger.Warn("notification scheduler close", zap.Error(err))
		}
	}
}

func configureAuthProviders(cfg config.AuthConfig, svc *auth.Service, repo auth.Repository, logger *zap.Logger) {
	if svc == nil {
		return
	}
	if cfg.Providers.Local.Disabled {
		svc.UnregisterProvider("password")
	}
	ldapCfg := cfg.Providers.LDAP
	if ldapCfg.Enabled {
		provider, err := auth.NewLDAPProvider(repo, logger, buildLDAPProviderOptions(ldapCfg))
		if err != nil {
			logger.Fatal("init ldap provider", zap.Error(err))
		}
		var providerOpts []auth.ProviderOption
		if ldapCfg.Default {
			providerOpts = append(providerOpts, auth.WithProviderDefault())
		}
		svc.RegisterProvider(provider, providerOpts...)
	} else if cfg.Providers.Local.Disabled {
		logger.Warn("auth: no identity providers enabled; logins will fail")
	}
}

func buildLDAPProviderOptions(cfg config.LDAPProviderConfig) auth.LDAPProviderOptions {
	return auth.LDAPProviderOptions{
		URL:                  cfg.URL,
		BindDN:               cfg.BindDN,
		BindPassword:         cfg.BindPassword,
		UserBaseDN:           cfg.UserBaseDN,
		UserFilter:           cfg.UserFilter,
		UsernameAttribute:    cfg.UsernameAttribute,
		DisplayNameAttribute: cfg.DisplayNameAttribute,
		EmailAttribute:       cfg.EmailAttribute,
		UseStartTLS:          cfg.UseStartTLS,
		SkipTLSVerify:        cfg.SkipTLSVerify,
		Timeout:              cfg.Timeout,
	}
}

func buildUIOptions(cfg config.UIConfig) server.UIOptions {
	themes := server.ThemeOptions{
		Default:             cfg.Themes.Default,
		Supported:           append([]string(nil), cfg.Themes.Supported...),
		AllowTenantOverride: cfg.Themes.AllowTenantOverride,
	}
	localization := server.LocalizationOptions{
		DefaultLocale: cfg.Localization.DefaultLocale,
		Supported:     append([]string(nil), cfg.Localization.Supported...),
		Fallback:      cfg.Localization.Fallback,
		DetectBrowser: cfg.Localization.DetectBrowser,
	}
	accessibility := server.AccessibilityOptions{
		HighContrastEnabled: cfg.Accessibility.HighContrastEnabled,
		FontScaleMin:        cfg.Accessibility.FontScaleMin,
		FontScaleMax:        cfg.Accessibility.FontScaleMax,
		ReducedMotion:       cfg.Accessibility.ReducedMotion,
		ScreenReaderHints:   cfg.Accessibility.ScreenReaderHints,
		FocusOutline:        cfg.Accessibility.FocusOutline,
		AnnounceLiveRegions: cfg.Accessibility.AnnounceUpdates,
		ValidationStrategy:  cfg.Accessibility.ValidationStrategy,
		LiveRegionDebounce:  int64(cfg.Accessibility.LiveRegionDebounce / time.Millisecond),
	}
	collab := server.CollaborationOptions{
		Enabled:            cfg.Collaboration.Enabled,
		OptimisticLockTTL:  int64(cfg.Collaboration.OptimisticLockTTL / time.Millisecond),
		PresenceHeartbeat:  int64(cfg.Collaboration.PresenceHeartbeat / time.Millisecond),
		MaxConcurrentLocks: cfg.Collaboration.MaxConcurrentLocks,
		CommentLengthLimit: cfg.Collaboration.CommentLengthLimit,
		ApprovalLevels:     cfg.Collaboration.ApprovalLevels,
		TaskQueueEnabled:   cfg.Collaboration.TaskQueueEnabled,
	}
	visualization := server.VisualizationOptions{
		Enable3D:               cfg.Visualization.Enable3D,
		MapProvider:            cfg.Visualization.MapProvider,
		LeaseHeatmapResolution: cfg.Visualization.LeaseHeatmapResolution,
		TopologyAutoDiscovery:  cfg.Visualization.TopologyAutoDiscovery,
		MaxCanvasNodes:         cfg.Visualization.MaxCanvasNodes,
		SnapshotIntervalMillis: int64(cfg.Visualization.SnapshotInterval / time.Millisecond),
	}
	return server.UIOptions{
		Themes:        themes,
		Localization:  localization,
		Accessibility: accessibility,
		Collaboration: collab,
		Visualization: visualization,
	}
}

func buildOpsSupportOptions(cfg config.OpsSupportConfig) ops.Options {
	importExport := ops.ImportExportOptions{
		Enabled:     cfg.ImportExport.Enabled,
		StoragePath: cfg.ImportExport.StoragePath,
		ObjectStore: cfg.ImportExport.ObjectStore,
		Formats:     append([]string(nil), cfg.ImportExport.Formats...),
		MaxFileSize: cfg.ImportExport.MaxFileSize,
	}
	helpCenter := ops.HelpCenterOptions{
		Enabled:           cfg.HelpCenter.Enabled,
		BaseURL:           cfg.HelpCenter.BaseURL,
		ArticlesPath:      cfg.HelpCenter.ArticlesPath,
		FAQPath:           cfg.HelpCenter.FAQPath,
		ReleaseNotesPath:  cfg.HelpCenter.ReleaseNotesPath,
		FeaturedTopics:    append([]string(nil), cfg.HelpCenter.FeaturedTopics...),
		ExternalSearchURL: cfg.HelpCenter.ExternalSearchURL,
		CacheTTL:          cfg.HelpCenter.CacheTTL,
	}
	support := ops.ExternalSupportOptions{
		Enabled:          cfg.Support.Enabled,
		TicketURL:        cfg.Support.TicketURL,
		ChatURL:          cfg.Support.ChatURL,
		Phone:            cfg.Support.Phone,
		EscalationPolicy: cfg.Support.EscalationPolicy,
		Contacts:         append([]string(nil), cfg.Support.Contacts...),
	}
	scripts := ops.ScriptRunnerOptions{
		Enabled:        cfg.Scripts.Enabled,
		DefaultTimeout: cfg.Scripts.DefaultTimeout,
		MaxConcurrent:  cfg.Scripts.MaxConcurrent,
		SandboxImage:   cfg.Scripts.SandboxImage,
	}
	if len(cfg.Scripts.Catalog) > 0 {
		scripts.Catalog = make([]ops.ScriptDescriptor, len(cfg.Scripts.Catalog))
		for i, entry := range cfg.Scripts.Catalog {
			scripts.Catalog[i] = ops.ScriptDescriptor{
				Name:             entry.Name,
				Description:      entry.Description,
				Command:          entry.Command,
				Args:             append([]string(nil), entry.Args...),
				AllowedRoles:     append([]string(nil), entry.AllowedRoles...),
				RequiresApproval: entry.RequiresApproval,
			}
		}
	}
	return ops.Options{
		System: ops.OpsSystemOptions{
			Enabled:           cfg.System.Enabled,
			AllowConfigExport: cfg.System.AllowConfigExport,
			AllowConfigImport: cfg.System.AllowConfigImport,
			BackupLocation:    cfg.System.BackupLocation,
			MaintenanceWindow: cfg.System.MaintenanceWindow,
			Theme:             cfg.System.Theme,
			Locale:            cfg.System.Locale,
			MaintenanceMode:   cfg.System.MaintenanceMode,
			Announcement:      cfg.System.Announcement,
		},
		ImportExport: importExport,
		HelpCenter:   helpCenter,
		Support:      support,
		Scripts:      scripts,
	}
}

func boolPtr(b bool) *bool {
	return &b
}

type boolFlag struct {
	value bool
	set   bool
}

func (b *boolFlag) String() string {
	return strconv.FormatBool(b.value)
}

func (b *boolFlag) Set(val string) error {
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}
	b.value = parsed
	b.set = true
	return nil
}

func (b *boolFlag) IsBoolFlag() bool {
	return true
}

func (b *boolFlag) Bool() bool {
	return b.value
}

func (b *boolFlag) WasSet() bool {
	return b.set
}
