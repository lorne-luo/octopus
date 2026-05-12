package handlers

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/transformer2/adapter"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/v1").
		Use(middleware.APIKeyAuth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/chat/completions", http.MethodPost).
				Handle(chat),
		).
		AddRoute(
			router.NewRoute("/responses", http.MethodPost).
				Handle(response),
		).
		AddRoute(
			router.NewRoute("/messages", http.MethodPost).
				Handle(message),
		).
		AddRoute(
			router.NewRoute("/embeddings", http.MethodPost).
				Handle(embedding),
		)
}

func chat(c *gin.Context) {
	relay.Handler(adapter.ClientOpenAIChat, c)
}
func response(c *gin.Context) {
	relay.Handler(adapter.ClientOpenAIResponse, c)
}
func message(c *gin.Context) {
	relay.Handler(adapter.ClientAnthropic, c)
}
func embedding(c *gin.Context) {
	relay.Handler(adapter.ClientOpenAIEmbedding, c)
}