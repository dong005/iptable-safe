package main

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"iptables-safe/database"
	"iptables-safe/handlers"
	"iptables-safe/iptables"
)

func main() {
	log.Println("Starting iptables-safe application...")

	if err := database.InitDB("./iptables-safe.db"); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.DB.Close()

	if err := iptables.InitializeFirewall(); err != nil {
		log.Fatalf("Failed to initialize firewall: %v", err)
	}

	go cleanupWorker()

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	router.GET("/", handlers.UserLoginPage)
	router.POST("/api/login", handlers.UserLogin)

	router.GET("/admin", handlers.AdminLoginPage)
	router.POST("/api/admin/login", handlers.AdminLogin)

	admin := router.Group("/admin")
	admin.Use(handlers.AdminAuthMiddleware())
	{
		admin.GET("/dashboard", handlers.AdminDashboard)
	}

	api := router.Group("/api/admin")
	api.Use(handlers.AdminAuthMiddleware())
	{
		api.GET("/whitelist", handlers.GetWhitelistIPs)
		api.POST("/whitelist", handlers.AddWhitelistIP)
		api.DELETE("/whitelist/:id", handlers.DeleteWhitelistIP)
		api.PUT("/password/user", handlers.UpdateUserPassword)
		api.PUT("/password/admin", handlers.UpdateAdminPassword)
	}

	log.Println("Server starting on :8888")
	if err := router.Run(":8888"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func cleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Running cleanup tasks...")
		
		if err := database.CleanupExpiredIPs(); err != nil {
			log.Printf("Error cleaning up expired IPs: %v", err)
		}
		
		if err := database.CleanupOldLoginAttempts(); err != nil {
			log.Printf("Error cleaning up old login attempts: %v", err)
		}
	}
}
