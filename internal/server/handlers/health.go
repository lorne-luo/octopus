package handlers

import (
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/health").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listHealth),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createHealth),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateHealth),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteHealth),
		)
}

func listHealth(c *gin.Context) {
	healthChecks, err := op.HealthList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, healthChecks)
}

func createHealth(c *gin.Context) {
	var hc model.HealthCheck
	if err := c.ShouldBindJSON(&hc); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	if err := op.HealthCreate(c.Request.Context(), &hc); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Register the health check task
	task.RegisterHealthTask(&hc)

	resp.Success(c, hc)
}

func updateHealth(c *gin.Context) {
	var hc model.HealthCheck
	if err := c.ShouldBindJSON(&hc); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	if err := op.HealthUpdate(c.Request.Context(), &hc); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Update the health check task
	task.UpdateHealthTask(&hc)

	resp.Success(c, hc)
}

func deleteHealth(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := op.HealthDelete(c.Request.Context(), id); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Unregister the health check task
	task.UnregisterHealthTask(id)

	resp.Success(c, nil)
}
