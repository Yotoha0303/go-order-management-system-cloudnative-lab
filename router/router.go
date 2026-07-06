package router

import (
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"log/slog"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Product   *handler.ProductHandler
	Inventory *handler.InventoryHandler
	StockLog  *handler.StockLogHandler
	Order     *handler.OrderHandler
	Health    *handler.HealthHandler
	User      *handler.UserHandler
	Assistant *handler.AssistantHandler
}

func SetupRouters(
	logger *slog.Logger,
	handlers Handlers,
	tokenManager *auth.TokenManager,
	roleChecker middleware.RoleChecker,
) *gin.Engine {
	r := gin.New()

	r.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		middleware.Recovery(logger),
	)

	registerHealthRouters(r, handlers)
	registerAPIRouter(r, handlers, tokenManager, roleChecker)
	return r
}

func registerHealthRouters(r *gin.Engine, handlers Handlers) {
	healthHandler := handlers.Health

	r.GET("/ping", healthHandler.PingHandler)
	r.GET("/live", healthHandler.LiveHandler)
	r.GET("/readyz", healthHandler.ReadyzHandler)
}

func registerAPIRouter(
	rg *gin.Engine,
	handlers Handlers,
	tokenManager *auth.TokenManager,
	roleChecker middleware.RoleChecker,
) {
	apiV1 := rg.Group("/api/v1")

	registerAuthAPIRouter(apiV1, handlers.User)

	protected := apiV1.Group("")
	protected.Use(middleware.AuthMiddleware(tokenManager))
	registerUserAPIRouter(protected, handlers.User)
	registerProductAPIRouter(protected, handlers.Product, roleChecker)
	registerInventoryAPIRouter(protected, handlers.Inventory, roleChecker)
	registerStockLogAPIRouter(protected, handlers.StockLog, roleChecker)
	registerOrderAPIRouter(protected, handlers.Order)
	registerAssistantAPIRouter(protected, handlers.Assistant, roleChecker)
}

func registerAuthAPIRouter(rg *gin.RouterGroup, userHandler *handler.UserHandler) {
	authRoutes := rg.Group("/auth")
	authRoutes.POST("/register", userHandler.Register)
	authRoutes.POST("/login", userHandler.Login)
}

func registerUserAPIRouter(rg *gin.RouterGroup, userHandler *handler.UserHandler) {
	users := rg.Group("/users")
	users.GET("/me", userHandler.Me)
	users.PUT("/me/profile", userHandler.UpdateProfile)
	users.PATCH("/me/password", userHandler.UpdatePassword)
}

func registerProductAPIRouter(
	rg *gin.RouterGroup,
	productHandler *handler.ProductHandler,
	roleChecker middleware.RoleChecker,
) {

	rg.GET("/products", productHandler.ListProducts)
	rg.GET("/products/:id", productHandler.GetProductByID)

	admin := rg.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/products", productHandler.CreateProduct)
	admin.PATCH("/products/:id/on-sale", productHandler.OnSaleProduct)
	admin.PATCH("/products/:id/off-sale", productHandler.OffSaleProduct)

}

func registerInventoryAPIRouter(
	rg *gin.RouterGroup,
	inventoryHandler *handler.InventoryHandler,
	roleChecker middleware.RoleChecker,
) {

	rg.GET("/inventory/products/:product_id", inventoryHandler.GetInventoryByProductID)

	admin := rg.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/inventory/init", inventoryHandler.InitInventory)
	admin.POST("/inventory/add", inventoryHandler.AddInventory)
}

func registerStockLogAPIRouter(
	rg *gin.RouterGroup,
	stockLogHandler *handler.StockLogHandler,
	roleChecker middleware.RoleChecker,
) {

	admin := rg.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.GET("/stock-logs", stockLogHandler.ListStockLogs)

}

func registerOrderAPIRouter(rg *gin.RouterGroup, orderHandler *handler.OrderHandler) {

	rg.POST("/orders", orderHandler.CreateOrder)
	rg.GET("/orders/:id", orderHandler.GetOrderByID)
	rg.GET("/orders", orderHandler.ListOrders)
	rg.PATCH("/orders/:id/cancel", orderHandler.CancelOrders)
	rg.PATCH("/orders/:id/pay", orderHandler.PayOrder)
	rg.PATCH("/orders/:id/finish", orderHandler.FinishOrder)
}

func registerAssistantAPIRouter(
	rg *gin.RouterGroup,
	assistantHandler *handler.AssistantHandler,
	roleChecker middleware.RoleChecker,
) {
	admin := rg.Group("/admin")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/assistant/chat", assistantHandler.Chat)
}
