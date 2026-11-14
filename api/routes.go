package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GameRoutes registers the public endpoints served by the game process.
func GameRoutes(frontendOrigin string, match, scoresProxy http.Handler, broker BrokerReadiness) http.Handler {
	router := newRouter()
	websocket := gin.WrapH(match)
	router.GET("/ws", websocket)
	router.HEAD("/ws", websocket)

	scores := router.Group("/api")
	proxy := gin.WrapH(allowFrontend(frontendOrigin, scoresProxy))
	scores.GET("/leaderboard", proxy)
	scores.HEAD("/leaderboard", proxy)
	scores.GET("/results/:id", proxy)
	scores.HEAD("/results/:id", proxy)
	scores.POST("/local-results", proxy)
	scores.OPTIONS("/local-results", gin.WrapF(localResultsPreflight(frontendOrigin)))

	health := gin.WrapF(brokerHealth(broker))
	router.GET("/healthz", health)
	router.HEAD("/healthz", health)
	return router
}

func newRouter() *gin.Engine {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.NoRoute(func(c *gin.Context) {
		http.NotFound(c.Writer, c.Request)
	})
	router.NoMethod(func(c *gin.Context) {
		http.Error(c.Writer, "Method Not Allowed", http.StatusMethodNotAllowed)
	})
	return router
}
