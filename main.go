package main

import (
	"log"
	"time"

	"iptables-safe/database"
	"iptables-safe/handlers"
	"iptables-safe/iptables"
	"iptables-safe/vos"

	"github.com/gin-gonic/gin"
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
	go vosSyncWorker()

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
		api.GET("/vos/config", handlers.GetVOSConfig)
		api.PUT("/vos/config", handlers.UpdateVOSConfig)
		api.POST("/vos/sync", handlers.ManualVOSSync)
		api.GET("/vos/status", handlers.GetVOSStatus)
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

func vosSyncWorker() {
	log.Println("VOS sync worker started")

	time.Sleep(10 * time.Second)

	enabled, err := database.GetVOSSyncEnabled()
	if err == nil && enabled {
		log.Println("Running initial VOS sync...")
		if _, err := vos.SyncIPsToWhitelist(); err != nil {
			log.Printf("Initial VOS sync failed: %v", err)
		}
	}

	consecutiveFailures := 0
	maxConsecutiveFailures := 3

	for {
		enabled, err := database.GetVOSSyncEnabled()
		if err != nil {
			log.Printf("Error getting VOS sync enabled status: %v", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		if !enabled {
			time.Sleep(1 * time.Minute)
			consecutiveFailures = 0
			continue
		}

		interval, err := database.GetVOSSyncInterval()
		if err != nil {
			log.Printf("Error getting VOS sync interval: %v", err)
			time.Sleep(5 * time.Minute)
			continue
		}

		if consecutiveFailures >= maxConsecutiveFailures {
			time.Sleep(30 * time.Minute)
		} else {
			time.Sleep(time.Duration(interval) * time.Minute)
		}

		result, err := vos.SyncIPsToWhitelist()
		if err != nil {
			consecutiveFailures++
			if consecutiveFailures == 1 {
				log.Printf("VOS sync failed: %v", err)
			} else if consecutiveFailures == maxConsecutiveFailures {
				log.Printf("VOS sync failed %d times consecutively. Reducing sync frequency to 30 minutes.", maxConsecutiveFailures)
			}
		} else {
			if consecutiveFailures >= maxConsecutiveFailures {
				log.Println("VOS sync recovered, resuming normal sync interval")
			}
			consecutiveFailures = 0
			if result.NewIPs > 0 {
				log.Printf("VOS sync completed: %d new IPs added", result.NewIPs)
			}
		}
	}
}
