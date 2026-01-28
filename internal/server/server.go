package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	_ "github.com/bestruirui/octopus/internal/server/handlers"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/static"
	"github.com/gin-gonic/gin"
)

var httpSrv http.Server

func Start() error {
	if conf.IsDebug() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
		c.Abort()
	}))

	if conf.IsDebug() {
		r.Use(middleware.Logger())
	}
	r.Use(middleware.Cors())
	r.Use(middleware.StaticEmbed("/", static.StaticFS))

	router.RegisterAll(r)

	httpSrv.Addr = fmt.Sprintf("%s:%d", conf.AppConfig.Server.Host, conf.AppConfig.Server.Port)
	httpSrv.Handler = r

	// Apply server timeouts from configuration.
	// Note: WriteTimeout is intentionally left at 0 to support SSE streaming responses.
	if conf.AppConfig.Server.ReadHeaderTimeoutSec > 0 {
		httpSrv.ReadHeaderTimeout = time.Duration(conf.AppConfig.Server.ReadHeaderTimeoutSec) * time.Second
	}
	if conf.AppConfig.Server.ReadTimeoutSec > 0 {
		httpSrv.ReadTimeout = time.Duration(conf.AppConfig.Server.ReadTimeoutSec) * time.Second
	}
	if conf.AppConfig.Server.IdleTimeoutSec > 0 {
		httpSrv.IdleTimeout = time.Duration(conf.AppConfig.Server.IdleTimeoutSec) * time.Second
	}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("http server listen and serve error: %v", err)
		}
	}()
	return nil
}

func Close() error {
	return httpSrv.Close()
}
