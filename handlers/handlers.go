package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iptables-safe/database"
	"iptables-safe/iptables"
	"iptables-safe/vos"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	MaxFailedAttempts     = 5
	LockoutDuration       = 15 * time.Minute
	TempWhitelistDuration = 24 * time.Hour
)

func UserLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func UserLogin(c *gin.Context) {
	clientIP := getClientIP(c)

	// 验证客户端IP
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to determine client IP"})
		return
	}

	failedAttempts, err := database.GetRecentFailedAttempts(clientIP, LockoutDuration)
	if err != nil {
		log.Printf("Error checking failed attempts: %v", err)
	}

	if failedAttempts >= MaxFailedAttempts {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many failed attempts. Please try again later.",
		})
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	config, err := database.GetConfig()
	if err != nil {
		log.Printf("Error getting config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(config.UserPassword), []byte(req.Password))
	if err != nil {
		database.RecordLoginAttempt(clientIP, false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	database.RecordLoginAttempt(clientIP, true)

	expiresAt := time.Now().Add(TempWhitelistDuration)
	if err := database.AddWhitelistIP(clientIP, "User login", false, expiresAt); err != nil {
		log.Printf("Error adding IP to database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to whitelist IP"})
		return
	}

	if err := iptables.AddIPToWhitelist(clientIP); err != nil {
		log.Printf("Error adding IP to iptables: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update firewall"})
		return
	}

	if err := iptables.SaveRules(); err != nil {
		log.Printf("Error saving iptables rules: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Access granted. Your IP has been whitelisted for 24 hours.",
		"ip":      clientIP,
		"expires": expiresAt.Format(time.RFC3339),
	})
}

func AdminLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_login.html", nil)
}

func AdminLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	config, err := database.GetConfig()
	if err != nil {
		log.Printf("Error getting config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(config.AdminPassword), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	token := generateToken()
	c.SetCookie("admin_token", token, 3600, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Login successful"})
}

func AdminDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_dashboard.html", nil)
}

func GetWhitelistIPs(c *gin.Context) {
	ips, err := database.GetAllWhitelistIPs()
	if err != nil {
		log.Printf("Error getting whitelist IPs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get whitelist"})
		return
	}

	// 检查每个IP的规则状态
	for i := range ips {
		ips[i].InputRuleExists = iptables.CheckIPInputRule(ips[i].IP)
		ips[i].OutputRuleExists = iptables.CheckIPOutputRule(ips[i].IP)
	}

	c.JSON(http.StatusOK, ips)
}

func AddWhitelistIP(c *gin.Context) {
	var req struct {
		IP          string `json:"ip" binding:"required"`
		Description string `json:"description"`
		IsPermanent bool   `json:"is_permanent"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var expiresAt time.Time
	if !req.IsPermanent {
		expiresAt = time.Now().Add(TempWhitelistDuration)
	}

	if err := database.AddWhitelistIP(req.IP, req.Description, req.IsPermanent, expiresAt); err != nil {
		log.Printf("Error adding IP to database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add IP"})
		return
	}

	if err := iptables.AddIPToWhitelist(req.IP); err != nil {
		log.Printf("Error adding IP to iptables: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update firewall"})
		return
	}

	if err := iptables.SaveRules(); err != nil {
		log.Printf("Error saving iptables rules: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "IP added successfully"})
}

func DeleteWhitelistIP(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ips, err := database.GetAllWhitelistIPs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get IP"})
		return
	}

	var targetIP string
	for _, ip := range ips {
		if ip.ID == id {
			targetIP = ip.IP
			break
		}
	}

	if targetIP == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "IP not found"})
		return
	}

	if err := database.DeleteWhitelistIP(id); err != nil {
		log.Printf("Error deleting IP from database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete IP"})
		return
	}

	if err := iptables.RemoveIPFromWhitelist(targetIP); err != nil {
		log.Printf("Error removing IP from iptables: %v", err)
	}

	if err := iptables.SaveRules(); err != nil {
		log.Printf("Error saving iptables rules: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "IP deleted successfully"})
}

func UpdateUserPassword(c *gin.Context) {
	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := database.UpdateUserPassword(req.NewPassword); err != nil {
		log.Printf("Error updating user password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User password updated successfully"})
}

func UpdateAdminPassword(c *gin.Context) {
	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := database.UpdateAdminPassword(req.NewPassword); err != nil {
		log.Printf("Error updating admin password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Admin password updated successfully"})
}

func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("admin_token")
		if err != nil || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func getClientIP(c *gin.Context) string {
	ip := c.GetHeader("X-Real-IP")
	if ip == "" {
		ip = c.GetHeader("X-Forwarded-For")
		if ip != "" {
			ips := strings.Split(ip, ",")
			ip = strings.TrimSpace(ips[0])
		}
	}
	if ip == "" {
		ip = c.ClientIP()
	}

	// 过滤掉无效IP
	if ip == "" || ip == "0.0.0.0" || ip == "::1" || strings.HasPrefix(ip, "::") {
		log.Printf("Warning: Invalid client IP detected: %s", ip)
		return ""
	}

	return ip
}

func generateToken() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func GetVOSConfig(c *gin.Context) {
	enabled, err := database.GetVOSSyncEnabled()
	if err != nil {
		log.Printf("Error getting VOS sync enabled: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}

	interval, err := database.GetVOSSyncInterval()
	if err != nil {
		log.Printf("Error getting VOS sync interval: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}

	lastSync, err := database.GetVOSLastSync()
	if err != nil {
		log.Printf("Error getting VOS last sync: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":   enabled,
		"interval":  interval,
		"last_sync": lastSync,
	})
}

func UpdateVOSConfig(c *gin.Context) {
	var req struct {
		Enabled  bool `json:"enabled"`
		Interval int  `json:"interval" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := database.UpdateVOSSyncConfig(req.Enabled, req.Interval); err != nil {
		log.Printf("Error updating VOS config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "VOS config updated successfully"})
}

func ManualVOSSync(c *gin.Context) {
	log.Println("Manual VOS sync triggered by admin")

	result, err := vos.ManualSync()
	if err != nil {
		log.Printf("Error during manual VOS sync: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Sync failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Sync completed successfully",
		"total_ips":  result.TotalIPs,
		"new_ips":    result.NewIPs,
		"failed_ips": result.FailedIPs,
		"sync_time":  result.SyncTime,
	})
}

func GetVOSStatus(c *gin.Context) {
	enabled, _ := database.GetVOSSyncEnabled()
	interval, _ := database.GetVOSSyncInterval()
	lastSync, _ := database.GetVOSLastSync()

	ips, err := database.GetAllWhitelistIPs()
	if err != nil {
		log.Printf("Error getting whitelist IPs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get status"})
		return
	}

	vosIPCount := 0
	for _, ip := range ips {
		if strings.HasPrefix(ip.Description, "VOS-") {
			vosIPCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":      enabled,
		"interval":     interval,
		"last_sync":    lastSync,
		"vos_ip_count": vosIPCount,
	})
}
