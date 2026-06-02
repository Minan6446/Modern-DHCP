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
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/automation"
	"modern-dhcp/internal/automation/workflow"
	workflowactions "modern-dhcp/internal/automation/workflow/actions"
	"modern-dhcp/internal/backup"
	"modern-dhcp/internal/cache"
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
	"modern-dhcp/internal/security/maclist"
	securitypolicy "modern-dhcp/internal/security/policy"
	securityquarantine "modern-dhcp/internal/security/quarantine"
	"modern-dhcp/internal/server"
	"modern-dhcp/internal/storage"
	"modern-dhcp/internal/superadmin"
	"modern-dhcp/internal/telemetry"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/internal/visualization"
	"modern-dhcp/pkg/models"
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
	securityguard.RegisterMetrics()

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
	if err := cfg.ValidateAuth(); err != nil {
		logger.Fatal("invalid auth config", zap.Error(err))
	}

	ctx := context.Background()
	telemetryOpts := telemetry.Options{
		Enabled:      strings.EqualFold(cfg.Monitoring.Tracing.Exporter, "otlp"),
		ServiceName:  strings.TrimSpace(cfg.Service.Name),
		Environment:  strings.TrimSpace(cfg.Service.Env),
		OTLPEndpoint: strings.TrimSpace(cfg.Monitoring.Tracing.Endpoint),
		Insecure:     cfg.Monitoring.Tracing.Insecure,
		SamplerRatio: cfg.Monitoring.Tracing.SamplerRatio,
	}
	telemetryProvider, telemetryErr := telemetry.Setup(ctx, telemetryOpts)
	if telemetryErr != nil {
		logger.Warn("telemetry setup", zap.Error(telemetryErr))
	} else if telemetryProvider != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
				logger.Warn("telemetry shutdown", zap.Error(err))
			}
		}()
	}
	mysqlConfig := cfg.MySQL
	postgresConfig := cfg.Postgres
	var (
		sqlDB        *sqlx.DB
		sqlReplica   *sqlx.DB
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
			replicaHandle, rErr := db.NewMySQLReplica(ctx, mysqlConfig)
			if rErr != nil {
				logger.Warn("connect mysql replica", zap.Error(rErr))
			} else {
				sqlReplica = replicaHandle
			}
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
		replicaHandle, rErr := db.NewMySQLReplica(ctx, mysqlConfig)
		if rErr != nil {
			logger.Warn("connect mysql replica", zap.Error(rErr))
		} else {
			sqlReplica = replicaHandle
		}
		dbHealthName = "mysql"
	}
	defer sqlDB.Close()
	if sqlReplica != nil {
		defer sqlReplica.Close()
	}
	tenantRouter := storage.NewTenantRouter(sqlDB, cfg.Tenancy).WithDefaultReplica(sqlReplica)
	defer tenantRouter.Close()
	metricCollector := metrics.NewCollector(cfg.Service.Name)
	journal := replication.NewKafkaJournal(cfg.HA.Replication, logger)
	if journal != nil {
		defer journal.Close()
	}
	failoverManager := failover.NewManager(cfg.HA, logger).WithMetricsCollector(metricCollector)

	redisClient, redisErr := cache.NewRedisClient(cfg.Redis, logger)
	if redisErr != nil {
		logger.Warn("redis not available", zap.Error(redisErr))
	}
	cacheStore := cache.NewLayeredCache(redisClient, cache.Options{
		DefaultTTL: 30 * time.Second,
		Prefix:     cfg.Service.Name,
		MaxItems:   8192,
		Logger:     logger,
	})

	healthHooks := []server.HealthHook{
		server.NewHealthHook(dbHealthName, func(ctx context.Context) error { return sqlDB.PingContext(ctx) }),
	}
	if redisClient != nil {
		healthHooks = append(healthHooks, server.NewHealthHook("redis", func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }))
		defer redisClient.Close()
	}
	backupManager := backup.NewManager(cfg.Backup, logger)
	if backupManager != nil {
		healthHooks = append(healthHooks, backupManager)
	}

	poolRepo := pool.NewRepository(sqlDB,
		pool.WithTenantRouter(tenantRouter),
		pool.WithCache(cacheStore, 30*time.Second),
		pool.WithMetrics(metricCollector),
		pool.WithLogger(logger),
	)
	leaseRepo := lease.NewRepository(sqlDB,
		lease.WithTenantRouter(tenantRouter),
		lease.WithCache(cacheStore, 15*time.Second),
		lease.WithMetrics(metricCollector),
	)
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
	poolSvc := pool.NewService(poolRepo, logger, pool.WithQuotaEnforcer(quotaSvc))
	leaseOpts := []lease.ServiceOption{lease.WithNotificationScheduler(notificationScheduler)}
	leaseOpts = append(leaseOpts, lease.WithFailoverStatusReporter(failoverManager))
	if cfg.HA.Replication.EnforceAck {
		if ackGate := replication.NewFailoverAckGate(cfg.HA, logger); ackGate != nil {
			policy := strings.TrimSpace(cfg.HA.Replication.AckFailurePolicy)
			if policy == "" {
				policy = "strict"
			}
			leaseOpts = append(leaseOpts, lease.WithSyncAckGate(ackGate), lease.WithSyncAckPolicy(policy))
		}
	}
	if redisClient != nil {
		leaseOpts = append(leaseOpts, lease.WithRedisAllocator(redisClient))
	}
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
		prober := netutil.NewDHCPv4ACDProber(
			cfg.Security.ConflictPrevention.ARPTimeout,
			cfg.Security.ConflictPrevention.ProbeTimeout,
		)
		leaseOpts = append(leaseOpts, lease.WithConflictPrevention(cfg.Security.ConflictPrevention, prober))
	}
	leaseSvc := lease.NewService(leaseRepo, poolSvc, cfg.Policy, logger, leaseOpts...)
	leaseScope := lease.NewResourceScope("global", "global")
	if repaired, recErr := leaseSvc.ReconcileDHCPv4LeaseFSMOnStartup(ctx, leaseScope, 200); recErr != nil {
		logger.Warn("dhcpv4 lease fsm startup reconciliation failed", zap.Error(recErr))
	} else if repaired > 0 {
		logger.Info("dhcpv4 lease fsm startup reconciliation repaired records", zap.Int("count", repaired))
	}
	var reclaimer *lease.Reclaimer
	if cfg.Policy.Lifecycle.AutoReclaim.Enabled {
		reclaimer = lease.NewReclaimer(leaseRepo, logger, lease.ReclaimerOptions{
			Interval:    cfg.Policy.Lifecycle.AutoReclaim.Interval,
			GracePeriod: cfg.Policy.Lifecycle.AutoReclaim.GracePeriod,
			BatchSize:   cfg.Policy.Lifecycle.AutoReclaim.BatchSize,
		})
	}
	policyRepo := policy.NewRepository(sqlDB,
		policy.WithCache(cacheStore, cfg.Policy.CacheTTL),
		policy.WithMetrics(metricCollector),
	)
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
	macListRepo := maclist.NewRepository(sqlDB)
	var macListCacheClient *redislib.Client
	if concrete, ok := redisClient.(*redislib.Client); ok {
		macListCacheClient = concrete
	} else if redisClient != nil {
		logger.Warn("mac list cache disabled: redis client is not single-node client")
	}
	macListSvc := maclist.NewServiceWithCache(macListRepo, logger, macListCacheClient, cfg.Security.MACACL.CacheTTL)
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
		LeaseRepo:       leaseSvc,
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
		LeaseReader:  leaseSvc,
		MaxNodes:     cfg.UI.Visualization.MaxCanvasNodes,
		HeatmapLimit: cfg.UI.Visualization.MaxCanvasNodes,
	})
	var iotRegistrySvc *iotregistry.Service
	if cfg.IoT.Registry.Enabled {
		iotRepo := iot.NewRepository(sqlDB)
		defaults := iotregistry.Defaults{
			SleepInterval: cfg.IoT.Registry.DefaultSleepInterval,
			OfflineWindow: cfg.IoT.Registry.DefaultOfflineWindow,
		}
		iotRegistrySvc = iotregistry.NewService(iotRepo, logger, iotregistry.WithDefaults(defaults))
	}
	alertStore := alerting.NewSQLStore(sqlDB)
	routeSnapshot := []alerting.RoutingRule{}
	if alertStore != nil {
		if snapshot, err := alertStore.ListRoutes(ctx); err != nil {
			logger.Warn("load alert routes from db", zap.Error(err))
		} else {
			routeSnapshot = snapshot
		}
	}
	routingStore := alerting.NewRoutingStore(routeSnapshot, "bootstrap")
	if len(routeSnapshot) == 0 && len(cfg.Monitoring.Alerting.Routes) > 0 {
		routingStore = alerting.NewRoutingStore(convertRouteRules(cfg.Monitoring.Alerting.Routes), "bootstrap")
	}
	if len(routingStore.Snapshot().Rules) == 0 {
		channels := notifierChannels(cfg.Monitoring.Alerting.Notifiers)
		if defaults := defaultRoutesFromChannels(channels); len(defaults) > 0 {
			routingStore = alerting.NewRoutingStore(convertRouteRules(defaults), "bootstrap")
			if alertStore != nil {
				if err := alertStore.ReplaceRoutes(ctx, routingStore.Snapshot().Rules, "system"); err != nil {
					logger.Warn("persist default alert routes", zap.Error(err))
				}
			}
		}
	}
	alertController, alertManager := setupAlerting(cfg.Monitoring.Alerting, monitorAggregator, rateTracker, alertFeed, alertStore, logger)
	if alertManager != nil && routingStore != nil {
		alertManager.ConfigureRoutes(alerting.BuildRoutes(routingRulesToConfigs(routingStore.Snapshot().Rules)))
	}
	notificationDispatcher := setupNotifications(cfg.Notifications, logger)
	jobStore := automation.NewJobStore(sqlDB)
	automationSvc, automationApprovals, automationApprovalPolicy := setupAutomation(cfg.Automation, cfg.Notifications, notificationDispatcher, monitorAggregator, jobStore, sqlDB, logger)
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
		RequireAuth: cfg.Auth.Enabled,
		APIKeys:     apiKeys,
		APITLS: server.APITLSOptions{
			Enabled:           cfg.Service.TLS.Enabled,
			AutoSelfSigned:    cfg.Service.TLS.AutoSelfSigned,
			CertFile:          strings.TrimSpace(cfg.Service.TLS.CertFile),
			KeyFile:           strings.TrimSpace(cfg.Service.TLS.KeyFile),
			ClientCAFile:      strings.TrimSpace(cfg.Service.TLS.ClientCAFile),
			RequireClientCert: cfg.Service.TLS.RequireClientCert,
			SelfSignedDir:     strings.TrimSpace(cfg.Service.TLS.SelfSignedDir),
		},
		HAConfig:           cfg.HA,
		Metrics:            metricCollector,
		PrometheusEnabled:  cfg.Monitoring.Prometheus.Enabled,
		PrometheusEndpoint: cfg.Monitoring.Prometheus.Endpoint,
		Environment:        strings.TrimSpace(cfg.Service.Env),
		Telemetry: server.TelemetryOptions{
			Enabled:      telemetryOpts.Enabled,
			Endpoint:     telemetryOpts.OTLPEndpoint,
			Insecure:     telemetryOpts.Insecure,
			SamplerRatio: telemetryOpts.SamplerRatio,
			ServiceName:  telemetryOpts.ServiceName,
			Environment:  telemetryOpts.Environment,
		},
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
		Alerting: server.AlertingOptions{
			Routes:        routingStore,
			Dispatcher:    notificationDispatcher,
			RuleStore:     alertStore,
			RouteStore:    alertStore,
			ConfigStore:   alertStore,
			TemplateStore: alertStore,
			ReceiverStore: alertStore,
		},
		Automation:               automationSvc,
		AutomationApprovals:      automationApprovals,
		AutomationApprovalPolicy: automationApprovalPolicy,
		Workflow:                 workflowSvc,
		OpsSupport:               opsSupportOpts,
		OpsSettingsStore:         opsSettingsStore,
		OptionRepo:               server.NewSQLOptionRepository(sqlDB),
		TemplateRepo:             server.NewSQLTemplateRepository(sqlDB),
		ScopeRepo:                server.NewSQLScopeRepository(sqlDB),
		TemplateApplyHistoryRepo: server.NewSQLTemplateApplyHistoryRepository(sqlDB),
		ClusterConfigStore:       server.NewSQLClusterConfigStore(sqlDB, logger),
		UI:                       uiOptions,
		SuperAdmin:               superAdminOpts,
		TokenProvider:            authSvc,
	}
	apiServer, err := server.NewHTTPServer(logger, serverOpts, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, auditSvc, reportSvc, visualizationSvc, iotRegistrySvc, collabHub, quotaSvc, rbacRepo, authSvc, macListSvc)
	if err != nil {
		logger.Fatal("init http server", zap.Error(err))
	}
	quarantineSink := securityquarantine.NewLeaseSink(leaseSvc, logger)
	var guardPolicyEvaluator securitypolicy.Evaluator
	if cfg.Security.Policy.Enabled {
		guardPolicyEvaluator = securityPolicyEvaluator
	}
	dhcpGuard := securityguard.NewFromConfig(cfg.Security, metricCollector, logger, quarantineSink, auditSvc, guardPolicyEvaluator, macListSvc, poolSvc, leaseSvc, rateTracker, snoopingTracker)
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
		BindAddress: cfg.Service.BindAddress,
		Port:        cfg.Service.DHCPv4Port,
		TenantID:    tenantID,
		ServerIP:    serverIP,
		Security:    dhcpv4SecurityOptionsFromConfig(cfg),
		Metrics:     metricCollector,
		RelayParser: relayParser,
		Option82Policy: dhcpv4.Option82Policy{
			Enabled:        cfg.Relay.Option82.Whitelist.Enabled,
			CircuitIDAllow: append([]string(nil), cfg.Relay.Option82.Whitelist.CircuitIDAllow...),
			CircuitIDDeny:  append([]string(nil), cfg.Relay.Option82.Whitelist.CircuitIDDeny...),
			RemoteIDAllow:  append([]string(nil), cfg.Relay.Option82.Whitelist.RemoteIDAllow...),
			RemoteIDDeny:   append([]string(nil), cfg.Relay.Option82.Whitelist.RemoteIDDeny...),
		},
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
	startPoolUsageSnapshotter(runtimeCtx, logger, poolSvc, leaseSvc)
	startLeaseBitmapSync(runtimeCtx, logger, poolSvc, leaseSvc)
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

func startPoolUsageSnapshotter(ctx context.Context, logger *zap.Logger, poolSvc *pool.Service, leaseSvc *lease.Service) {
	if poolSvc == nil || leaseSvc == nil {
		return
	}
	const tenantID = "global"
	go func() {
		snapshot := func() {
			poolScope := pool.NewResourceScope("global", tenantID)
			leaseScope := lease.NewResourceScope("global", tenantID)
			pools, err := listAllPoolsForSnapshot(ctx, poolSvc, poolScope, 500)
			if err != nil {
				logger.Warn("pool usage snapshot failed", zap.Error(err))
				return
			}
			if len(pools) == 0 {
				return
			}
			poolIDs := make([]string, 0, len(pools))
			for _, p := range pools {
				poolIDs = append(poolIDs, p.ID)
			}
			counts, err := leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, poolIDs)
			if err != nil {
				logger.Warn("pool usage snapshot leases failed", zap.Error(err))
				return
			}
			day := time.Now().UTC()
			day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
			entries := make([]models.PoolUsageDaily, 0, len(pools))
			for _, p := range pools {
				capacity := monitoring.CalculatePoolCapacity(p)
				entries = append(entries, models.PoolUsageDaily{
					PoolID:   p.ID,
					Day:      day,
					Used:     counts[p.ID],
					Capacity: capacity,
				})
			}
			if err := poolSvc.UpsertUsageDaily(ctx, poolScope, entries); err != nil {
				logger.Warn("pool usage snapshot upsert failed", zap.Error(err))
			}
		}

		snapshot()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snapshot()
			}
		}
	}()
}

func startLeaseBitmapSync(ctx context.Context, logger *zap.Logger, poolSvc *pool.Service, leaseSvc *lease.Service) {
	if poolSvc == nil || leaseSvc == nil {
		return
	}
	const tenantID = "global"
	go func() {
		syncOnce := func() {
			poolScope := pool.NewResourceScope("global", tenantID)
			leaseScope := lease.NewResourceScope("global", tenantID)
			pools, err := listAllPoolsForSnapshot(ctx, poolSvc, poolScope, 500)
			if err != nil {
				logger.Warn("lease bitmap sync list pools failed", zap.Error(err))
				return
			}
			for i := range pools {
				if _, err := leaseSvc.SyncPoolBitmapFromMySQL(ctx, leaseScope, pools[i]); err != nil {
					logger.Warn("lease bitmap sync pool failed", zap.String("poolId", pools[i].ID), zap.Error(err))
				}
			}
		}

		syncOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncOnce()
			}
		}
	}()
}

func listAllPoolsForSnapshot(ctx context.Context, poolSvc *pool.Service, scopeRef pool.ResourceScope, pageSize int) ([]models.AddressPool, error) {
	if poolSvc == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	offset := 0
	pools := make([]models.AddressPool, 0)
	for {
		batch, err := poolSvc.ListPools(ctx, scopeRef, pageSize, offset)
		if err != nil {
			return nil, err
		}
		pools = append(pools, batch...)
		if len(batch) < pageSize {
			break
		}
		offset += len(batch)
		if offset >= 5000 {
			break
		}
	}
	return pools, nil
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

	oidcCfg := cfg.Providers.OIDC
	if oidcCfg.Enabled {
		provider, err := auth.NewOIDCProvider(repo, logger, buildOIDCProviderOptions(oidcCfg))
		if err != nil {
			logger.Fatal("init oidc provider", zap.Error(err))
		}
		var providerOpts []auth.ProviderOption
		if oidcCfg.Default {
			providerOpts = append(providerOpts, auth.WithProviderDefault())
		}
		svc.RegisterProvider(provider, providerOpts...)
	}
}

func dhcpv4SecurityOptionsFromConfig(cfg *config.Config) dhcpv4.SecurityOptions {
	if cfg == nil {
		return dhcpv4.SecurityOptions{MACRateLimitPPS: 10}
	}
	pps := cfg.Security.DHCPv4.MACRateLimitPPS
	if pps <= 0 {
		pps = 10
	}
	return dhcpv4.SecurityOptions{
		MACRateLimitPPS: pps,
		RelayWhitelist:  append([]string(nil), cfg.Security.DHCPv4.RelayWhitelist...),
		RogueDetector: dhcpv4.RogueDetectorConfig{
			Enabled:      cfg.Security.DHCPv4.RogueDetector.Enabled,
			Interface:    strings.TrimSpace(cfg.Security.DHCPv4.RogueDetector.Interface),
			ClusterNodes: append([]string(nil), cfg.Security.DHCPv4.RogueDetector.ClusterNodes...),
		},
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

func buildOIDCProviderOptions(cfg config.OIDCProviderConfig) auth.OIDCProviderOptions {
	return auth.OIDCProviderOptions{
		Issuer:           cfg.Issuer,
		Audience:         cfg.Audience,
		JWKSURL:          cfg.JWKSURL,
		JWKSCacheTTL:     cfg.JWKSCacheTTL,
		HMACSecret:       cfg.HMACSecret,
		RequiredScopes:   append([]string(nil), cfg.RequiredScopes...),
		ScopeRoles:       cfg.ScopeRoles,
		DefaultRole:      cfg.DefaultRole,
		ClockSkew:        cfg.ClockSkew,
		UsernameClaim:    cfg.UsernameClaim,
		DisplayNameClaim: cfg.DisplayNameClaim,
		EmailClaim:       cfg.EmailClaim,
		RoleClaim:        cfg.RoleClaim,
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
	cloneStrings := func(values []string) []string {
		if len(values) == 0 {
			return nil
		}
		out := make([]string, len(values))
		copy(out, values)
		return out
	}
	cloneStringMap := func(src map[string]string) map[string]string {
		if len(src) == 0 {
			return nil
		}
		out := make(map[string]string, len(src))
		for k, v := range src {
			out[k] = v
		}
		return out
	}
	toStep := func(step config.MaintenanceStep) ops.MaintenanceStepOption {
		return ops.MaintenanceStepOption{
			ID:                step.ID,
			Title:             step.Title,
			Summary:           step.Summary,
			Duration:          step.Duration,
			Responsible:       step.Responsible,
			RequiresApproval:  step.RequiresApproval,
			DependsOn:         cloneStrings(step.DependsOn),
			AutomationJobType: step.AutomationJobType,
		}
	}
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
	maintenance := ops.MaintenanceOptions{Enabled: cfg.Maintenance.Enabled}
	if len(cfg.Maintenance.Playbooks) > 0 {
		maintenance.Playbooks = make([]ops.MaintenancePlaybookOption, len(cfg.Maintenance.Playbooks))
		for i, playbook := range cfg.Maintenance.Playbooks {
			steps := make([]ops.MaintenanceStepOption, len(playbook.Steps))
			for j, step := range playbook.Steps {
				steps[j] = toStep(step)
			}
			maintenance.Playbooks[i] = ops.MaintenancePlaybookOption{
				ID:          playbook.ID,
				Title:       playbook.Title,
				Description: playbook.Description,
				Tags:        cloneStrings(playbook.Tags),
				Steps:       steps,
			}
		}
	}
	if len(cfg.Maintenance.Windows) > 0 {
		maintenance.Windows = make([]ops.MaintenanceWindowOption, len(cfg.Maintenance.Windows))
		for i, window := range cfg.Maintenance.Windows {
			maintenance.Windows[i] = ops.MaintenanceWindowOption{
				ID:       window.ID,
				Name:     window.Name,
				Cron:     window.Cron,
				Duration: window.Duration,
				Timezone: window.Timezone,
			}
		}
	}
	if len(cfg.Maintenance.UpgradePlans) > 0 {
		maintenance.UpgradePlans = make([]ops.MaintenanceUpgradePlanOption, len(cfg.Maintenance.UpgradePlans))
		for i, plan := range cfg.Maintenance.UpgradePlans {
			steps := make([]ops.MaintenanceStepOption, len(plan.Steps))
			for j, step := range plan.Steps {
				steps[j] = toStep(step)
			}
			rollback := make([]ops.MaintenanceStepOption, len(plan.RollbackPlan))
			for j, step := range plan.RollbackPlan {
				rollback[j] = toStep(step)
			}
			maintenance.UpgradePlans[i] = ops.MaintenanceUpgradePlanOption{
				ID:            plan.ID,
				Version:       plan.Version,
				Summary:       plan.Summary,
				ScheduledFor:  plan.ScheduledFor,
				Prerequisites: cloneStrings(plan.Prerequisites),
				Steps:         steps,
				RollbackPlan:  rollback,
				ReleaseNotes:  plan.ReleaseNotes,
			}
		}
	}
	backups := ops.BackupOptions{Enabled: cfg.Backups.Enabled}
	if len(cfg.Backups.Schedules) > 0 {
		backups.Schedules = make([]ops.BackupScheduleOption, len(cfg.Backups.Schedules))
		for i, schedule := range cfg.Backups.Schedules {
			backups.Schedules[i] = ops.BackupScheduleOption{
				ID:           schedule.ID,
				Name:         schedule.Name,
				Cron:         schedule.Cron,
				Retention:    schedule.Retention,
				Window:       schedule.Window,
				Type:         schedule.Type,
				Enabled:      schedule.Enabled,
				Destinations: cloneStrings(schedule.Destinations),
			}
		}
	}
	if len(cfg.Backups.Destinations) > 0 {
		backups.Destinations = make([]ops.BackupDestinationOption, len(cfg.Backups.Destinations))
		for i, target := range cfg.Backups.Destinations {
			backups.Destinations[i] = ops.BackupDestinationOption{
				ID:          target.ID,
				Name:        target.Name,
				Kind:        target.Kind,
				Endpoint:    target.Endpoint,
				Credentials: target.Credentials,
				Metadata:    cloneStringMap(target.Metadata),
			}
		}
	}
	if len(cfg.Backups.RestoreFlows) > 0 {
		backups.RestoreFlows = make([]ops.RestoreWorkflowOption, len(cfg.Backups.RestoreFlows))
		for i, flow := range cfg.Backups.RestoreFlows {
			steps := make([]ops.MaintenanceStepOption, len(flow.Steps))
			for j, step := range flow.Steps {
				steps[j] = toStep(step)
			}
			backups.RestoreFlows[i] = ops.RestoreWorkflowOption{
				ID:          flow.ID,
				Name:        flow.Name,
				Description: flow.Description,
				Steps:       steps,
				Checks:      cloneStrings(flow.Checks),
				Approvals:   cloneStrings(flow.Approvals),
			}
		}
	}
	backups.Verification = ops.BackupVerificationOption{
		Enabled:      cfg.Backups.Verification.Enabled,
		Schedule:     cfg.Backups.Verification.Schedule,
		Retention:    cfg.Backups.Verification.Retention,
		Sandbox:      cfg.Backups.Verification.SandboxEnv,
		AlertChannel: cfg.Backups.Verification.AlertChannel,
	}
	performance := ops.PerformanceOptions{Enabled: cfg.Performance.Enabled}
	if len(cfg.Performance.Probes) > 0 {
		performance.Probes = make([]ops.PerformanceProbeOption, len(cfg.Performance.Probes))
		for i, probe := range cfg.Performance.Probes {
			thresholds := make(map[string]float64, len(probe.Thresholds))
			for k, v := range probe.Thresholds {
				thresholds[k] = v
			}
			performance.Probes[i] = ops.PerformanceProbeOption{
				ID:          probe.ID,
				Name:        probe.Name,
				Description: probe.Description,
				Command:     probe.Command,
				Interval:    probe.Interval,
				SLO:         probe.SLO,
				Units:       probe.Units,
				Thresholds:  thresholds,
			}
		}
	}
	if len(cfg.Performance.Indicators) > 0 {
		performance.Indicators = make([]ops.PerformanceIndicatorOption, len(cfg.Performance.Indicators))
		for i, indicator := range cfg.Performance.Indicators {
			performance.Indicators[i] = ops.PerformanceIndicatorOption{
				ID:          indicator.ID,
				Name:        indicator.Name,
				Description: indicator.Description,
				Target:      indicator.Target,
				Units:       indicator.Units,
			}
		}
	}
	if len(cfg.Performance.Recommendations) > 0 {
		performance.Recommendations = make([]ops.PerformancePlaybookOption, len(cfg.Performance.Recommendations))
		for i, rec := range cfg.Performance.Recommendations {
			steps := make([]ops.MaintenanceStepOption, len(rec.Steps))
			for j, step := range rec.Steps {
				steps[j] = toStep(step)
			}
			performance.Recommendations[i] = ops.PerformancePlaybookOption{
				ID:      rec.ID,
				Title:   rec.Title,
				Summary: rec.Summary,
				Impact:  rec.Impact,
				Steps:   steps,
				Signals: cloneStrings(rec.Signals),
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
		Maintenance:  maintenance,
		Backups:      backups,
		Performance:  performance,
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
