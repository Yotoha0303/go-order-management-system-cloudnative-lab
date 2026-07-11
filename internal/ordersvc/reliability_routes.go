package ordersvc

import (
	"net/http"

	"go-order-management-system/internal/platform/internalapi"

	"github.com/gin-gonic/gin"
)

func RegisterReliabilityRoutes(router *gin.Engine, internalToken string, snapshotter reliabilitySnapshotter) {
	if router == nil || snapshotter == nil {
		return
	}
	internal := router.Group("/internal/v1")
	internal.Use(internalapi.Middleware(internalToken))
	internal.GET("/operations/reliability", func(c *gin.Context) {
		snapshot, err := snapshotter.Snapshot(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 50031,
				"msg":  "collect reliability snapshot failed",
			})
			return
		}
		c.JSON(http.StatusOK, snapshot)
	})
}
