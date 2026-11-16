package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"iptables-safe/database"
	"iptables-safe/handlers"
	"iptables-safe/iptables"
	"iptables-safe/serverctl"
	"iptables-safe/vos"

	"github.com/gin-gonic/gin"
)

func main() {
	if len(os.Args) > 1 {
		if err := runCLI(os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	runServer()
}

func dbPath() string {
	if v := os.Getenv("IPTABLES_SAFE_DB"); v != "" {
		return v
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "iptables-safe.db")
	}
	return "./iptables-safe.db"
}

func runCLI(args []string) error {
	cmd := args[0]
	switch cmd {
	case "passwd", "set-password", "password":
		if len(args) < 3 {
			return fmt.Errorf("usage: %s passwd <user|admin> <new-password>", os.Args[0])
		}
		role, pass := args[1], args[2]
		if len(pass) < handlers.MinPasswordLength {
			return fmt.Errorf("password too short (min %d)", handlers.MinPasswordLength)
		}
		if err := database.InitDB(dbPath()); err != nil {
			return err
		}
		defer database.DB.Close()
		switch role {
		case "user":
			if err := database.UpdateUserPassword(pass); err != nil {
				return err
			}
			fmt.Println("User password updated.")
		case "admin":
			if err := database.UpdateAdminPassword(pass); err != nil {
				return err
			}
			fmt.Println("Admin password updated.")
		default:
			return fmt.Errorf("role must be user or admin")
		}
		return nil

	case "set-port", "port":
		if len(args) < 2 {
			return fmt.Errorf("usage: %s set-port <port>", os.Args[0])
		}
		port, err := strconv.Atoi(args[1])
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid port")
		}
		if err := database.InitDB(dbPath()); err != nil {
			return err
		}
		defer database.DB.Close()
		if err := database.UpdateListenPort(port); err != nil {
			return err
		}
		fmt.Printf("Listen port saved as %d. Restart service (or use admin UI) to apply.\n", port)
		return nil

	case "help", "-h", "--help":
		printUsage()
		return nil

	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usageText())
	}
}

func printUsage() {
	fmt.Print(usageText())
}

func usageText() string {
	return fmt.Sprintf(`Usage:
  %[1]s                          Start web service
  %[1]s passwd user <password>   Change user password
  %[1]s passwd admin <password>  Change admin password
  %[1]s set-port <port>          Save listen port (default 8899)
  %[1]s help                     Show help
`, os.Args[0])
}

func runServer() {
	log.Println("Starting iptables-safe application...")
	gin.SetMode(gin.ReleaseMode)

	if err := database.InitDB(dbPath()); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.DB.Close()

	port, err := database.GetListenPort()
	if err != nil {
		log.Fatalf("Failed to get listen port: %v", err)
	}
	if v := os.Getenv("IPTABLES_SAFE_PORT"); v != "" {
		if p, e := strconv.Atoi(v); e == nil && p > 0 && p <= 65535 {
			port = p
			_ = database.UpdateListenPort(port)
		}
	}

	if err := iptables.InitializeFirewall(port); err != nil {
		log.Fatalf("Failed to initialize firewall: %v", err)
	}

	go cleanupWorker()
	go vosSyncWorker()
	go sessionCleanupWorker()

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.LoadHTMLGlob("templates/*")

	router.GET("/", handlers.UserLoginPage)
	router.POST("/api/login", handlers.UserLogin)

	router.GET("/admin", handlers.AdminLoginPage)
	router.POST("/api/admin/login", handlers.AdminLogin)
	router.POST("/api/admin/logout", handlers.AdminLogout)

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
		api.GET("/port", handlers.GetListenPort)
		api.PUT("/port", handlers.UpdateListenPort)
		api.GET("/vos/config", handlers.GetVOSConfig)
		api.PUT("/vos/config", handlers.UpdateVOSConfig)
		api.POST("/vos/sync", handlers.ManualVOSSync)
		api.GET("/vos/status", handlers.GetVOSStatus)
	}

	serverctl.SetHandler(router)
	if err := serverctl.Start(port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Printf("Server started on :%d", port)
	select {}
}

func cleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		log.Println("Running cleanup tasks...")
		expired, err := database.CleanupExpiredIPs()
		if err != nil {
			log.Printf("Error cleaning up expired IPs: %v", err)
		} else {
			for _, ip := range expired {
				if err := iptables.RemoveIPFromWhitelist(ip); err != nil {
					log.Printf("Error removing expired IP %s from iptables: %v", ip, err)
				}
			}
		}
		if err := database.CleanupOldLoginAttempts(); err != nil {
			log.Printf("Error cleaning up old login attempts: %v", err)
		}
	}
}

func sessionCleanupWorker() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		handlers.CleanupExpiredSessions()
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
