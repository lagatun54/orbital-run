package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"orbitalrun/api/scores/v1/scoresv1connect"
)

// GameRoutes registers the public endpoints served by the game process.
func GameRoutes(frontendOrigin string, match *MatchHandler, scoresRPC http.Handler, broker BrokerReadiness) http.Handler {
	router := newRouter()
	websocket := gin.WrapH(match)
	router.GET("/ws", websocket)
	router.HEAD("/ws", websocket)
	router.GET("/metrics", gin.WrapF(match.serveMetrics))

	health := gin.WrapF(brokerHealth(broker))
	router.GET("/healthz", health)
	router.HEAD("/healthz", health)
	if scoresRPC == nil {
		return router
	}
	// Mount the RPCs outside Gin so HTTP/2 trailers are written by net/http.
	mux := http.NewServeMux()
	rpc := allowRPCFrontend(frontendOrigin, scoresRPC)
	mux.Handle("/"+scoresv1connect.LeaderboardServiceName+"/", rpc)
	mux.Handle("/"+scoresv1connect.ResultsServiceName+"/", rpc)
	mux.Handle("/", router)
	return mux
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
