// Copyright (c) 2023 Silverton Data, Inc.
// You may use, distribute, and modify this code under the terms of the Apache-2.0 license, a copy of
// which may be found at https://github.com/silverton-io/buz/blob/main/LICENSE

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apex/gateway/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/silverton-io/buz/pkg/config"
	"github.com/silverton-io/buz/pkg/constants"
	"github.com/silverton-io/buz/pkg/env"
	"github.com/silverton-io/buz/pkg/handler"
	"github.com/silverton-io/buz/pkg/input"
	"github.com/silverton-io/buz/pkg/manifold"
	"github.com/silverton-io/buz/pkg/meta"
	"github.com/silverton-io/buz/pkg/middleware"
	cloudevents "github.com/silverton-io/buz/pkg/protocol/cloudevents"
	pixel "github.com/silverton-io/buz/pkg/protocol/pixel"
	selfdescribing "github.com/silverton-io/buz/pkg/protocol/selfdescribing"
	snowplow "github.com/silverton-io/buz/pkg/protocol/snowplow"
	webhook "github.com/silverton-io/buz/pkg/protocol/webhook"
	"github.com/silverton-io/buz/pkg/registry"
	"github.com/silverton-io/buz/pkg/sink"
	"github.com/silverton-io/buz/pkg/tele"
	"github.com/spf13/viper"
)

var VERSION string

type App struct {
	config                *config.Config
	engine                *gin.Engine
	manifold              manifold.Manifold
	collectorMeta         *meta.CollectorMeta
	debug                 bool
	publicRouterGroup     *gin.RouterGroup
	switchableRouterGroup *gin.RouterGroup
}

func (a *App) configure() {
	// Set up app logger
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	gin.SetMode("release")

	// Load app config from file
	conf := os.Getenv(env.BUZ_CONFIG_PATH)
	debug := os.Getenv(env.DEBUG)
	if conf == "" {
		conf = "config.yml"
	}
	log.Info().Msg("🟢 loading config from " + conf)
	viper.SetConfigFile(conf)
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Stack().Err(err).Msg("could not read config")
	}
	a.config = &config.Config{}
	if err := viper.Unmarshal(a.config); err != nil {
		log.Fatal().Stack().Err(err).Msg("could not unmarshal config")
	}
	if debug != "" && (debug == "true" || debug == "1" || debug == "True") {
		// Put gin, logging, and request logging into debug mode
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Warn().Msg("🟡 DEBUG flag set - setting gin mode to debug")
		gin.SetMode("debug")
		log.Warn().Msg("🟡 DEBUG flag set - activating request logger")
		a.config.Middleware.RequestLogger.Enabled = true
		a.debug = true
	}
	a.config.App.Version = VERSION
	meta := meta.BuildCollectorMeta(VERSION, a.config)
	a.collectorMeta = meta
}

func (a *App) initializeManifold() {
	log.Info().Msg("🟢 initializing manifold")
	m := &manifold.ChannelManifold{}
	log.Info().Msg("🟢 initializing registry")
	registry := registry.Registry{}
	if err := registry.Initialize(a.config.Registry); err != nil {
		log.Fatal().Err(err).Msg("could not initialize registry")
	}
	log.Info().Msg("🟢 initializing sinks")
	sinks, err := sink.BuildAndInitializeSinks(a.config.Sinks)
	if err != nil {
		log.Fatal().Err(err).Msg("could not build and initialize sinks")
	}
	err = m.Initialize(&registry, &sinks, a.config, a.collectorMeta)
	if err != nil {
		log.Fatal().Stack().Err(err).Msg("could not build manifold")
	}
	a.manifold = m
}

func (a *App) initializeRouter() {
	log.Info().Msg("🟢 initializing router")
	a.engine = gin.New()
	a.publicRouterGroup = a.engine.Group("")
	a.switchableRouterGroup = a.engine.Group("")
	if err := a.engine.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	if a.debug {
		log.Info().Msg("setting up pprof at /debug/pprof")
		pprof.Register(a.engine)
	}
	a.engine.RedirectTrailingSlash = false
}

func (a *App) initializeMiddleware() {
	log.Info().Msg("🟢 initializing middleware")
	a.engine.Use(gin.Recovery())
	if a.config.Middleware.Timeout.Enabled {
		log.Info().Msg("🟢 initializing request timeout middleware")
		a.engine.Use(middleware.Timeout(a.config.Middleware.Timeout))
	}
	if a.config.Middleware.RateLimiter.Enabled {
		log.Info().Msg("🟢 initializing rate limiter middleware")
		limiter := middleware.BuildRateLimiter(a.config.Middleware.RateLimiter)
		limiterMiddleware := middleware.BuildRateLimiterMiddleware(limiter)
		a.engine.Use(limiterMiddleware)
	}
	if a.config.Middleware.Cors.Enabled {
		log.Info().Msg("🟢 initializing cors middleware")
		a.engine.Use(middleware.CORS(a.config.Middleware.Cors))
	}
	if a.config.Middleware.RequestLogger.Enabled {
		log.Info().Msg("🟢 initializing request logger middleware")
		a.engine.Use(middleware.RequestLogger())
	}
	if a.config.Middleware.Auth.Enabled {
		log.Info().Msg("🟢 initializing auth middleware")
		a.switchableRouterGroup.Use(middleware.Auth(a.config.Middleware.Auth))
	}
}

// 🐝 and healthcheck route are always public
func (a *App) initializePublicRoutes() {
	log.Info().Msg("🟢 initializing buz route")
	a.publicRouterGroup.GET("/", handler.BuzHandler())
	log.Info().Msg("🟢 initializing health check route")
	a.publicRouterGroup.GET(constants.HEALTH_PATH, handler.HealthcheckHandler)
}

func (a *App) initializeOpsRoutes() {
	log.Info().Msg("🟢 initializing stats route")
	a.switchableRouterGroup.GET(constants.STATS_PATH, handler.StatsHandler(a.collectorMeta)) // FIXME!! Pass manifold here, as it will have the statistics
	log.Info().Msg("🟢 initializing overview routes")
	a.switchableRouterGroup.GET(constants.ROUTE_OVERVIEW_PATH, handler.RouteOverviewHandler(*a.config))
	if a.config.App.EnableConfigRoute {
		log.Info().Msg("🟢 initializing config overview")
		a.switchableRouterGroup.GET(constants.CONFIG_OVERVIEW_PATH, handler.ConfigOverviewHandler(*a.config))
	}
}

func (a *App) initializeSchemaCacheRoutes() {
	r := a.manifold.GetRegistry()
	if a.config.Registry.Purge.Enabled {
		log.Info().Msg("🟢 initializing schema registry cache purge route")
		a.switchableRouterGroup.GET(registry.CACHE_PURGE_ROUTE, registry.PurgeCacheHandler(r))
		a.switchableRouterGroup.POST(registry.CACHE_PURGE_ROUTE, registry.PurgeCacheHandler(r))
	}
	if a.config.Registry.Http.Enabled {
		log.Info().Msg("🟢 initializing schema registry routes")
		a.switchableRouterGroup.GET(registry.SCHEMAS_ROUTE+"*"+registry.SCHEMA_PARAM, registry.GetSchemaHandler(r))
	}
}

func (a *App) initializeInputs() {
	inputs := []input.Input{
		&pixel.PixelInput{},
		&webhook.WebhookInput{},
		&selfdescribing.SelfDescribingInput{},
		&cloudevents.CloudeventsInput{},
		&snowplow.SnowplowInput{},
	}
	for _, i := range inputs {
		err := i.Initialize(a.switchableRouterGroup, &a.manifold, a.config, a.collectorMeta)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to initialize input")
		}
	}
}

func (a *App) Initialize() {
	log.Info().Msg("🟢 initializing app")
	a.configure()
	a.initializeRouter()
	a.initializeManifold()
	a.initializeMiddleware()
	a.initializePublicRoutes()
	a.initializeOpsRoutes()
	a.initializeSchemaCacheRoutes()
	a.initializeInputs()
}

func (a *App) serverlessMode() {
	log.Debug().Msg("🟡 running buz in serverless mode")
	log.Info().Msg("🐝🐝🐝 buz is running 🐝🐝🐝")
	err := gateway.ListenAndServe(":3000", a.engine)
	tele.Sis(a.collectorMeta)
	if err != nil {
		log.Fatal().Err(err)
	}
	err = a.manifold.Shutdown()
	if err != nil {
		log.Error().Err(err).Msg("manifold failed to shut down safely")
	}
}

func (a *App) standardMode() {
	log.Debug().Msg("🟡 running Buz in standard mode")
	srv := &http.Server{
		Addr:    ":" + a.config.App.Port,
		Handler: a.engine,
	}
	go func() {
		log.Info().Msg("🐝🐝🐝 buz is running 🐝🐝🐝")
		if err := srv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Info().Msgf("🟢 server shut down")
		}
	}()
	// Safe shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("🟢 shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		err := a.manifold.Shutdown()
		if err != nil {
			log.Error().Err(err).Msg("manifold failed to shut down safely")
		}
		log.Fatal().Stack().Err(err).Msg("server forced to shutdown")
	}
	err := a.manifold.Shutdown()
	if err != nil {
		log.Error().Err(err).Msg("manifold failed to shut down safely")
	}
	tele.Sis(a.collectorMeta)
}

func (a *App) Run() {
	log.Debug().Interface("config", a.config).Msg("running 🐝 with config")
	tele.Metry(a.config, a.collectorMeta)
	if a.config.App.Serverless {
		a.serverlessMode()
	} else {
		a.standardMode()
	}
}
