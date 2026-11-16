package handlers

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iptables-safe/database"
	"iptables-safe/iptables"
	"iptables-safe/serverctl"
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
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to determine client IP"})
		return
	}
	if ip := net.ParseIP(clientIP); ip != nil && ip.IsLoopback() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Loopback IP cannot be whitelisted via login"})
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

	if err := bcrypt.CompareHashAndPassword([]byte(config.UserPassword), []byte(req.Password)); err != nil {
		_ = database.RecordLoginAttempt(clientIP, false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	_ = database.RecordLoginAttempt(clientIP, true)

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
	_ = iptables.SaveRules()

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
	clientIP := getClientIP(c)
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to determine client IP"})
		return
	}

	failedAttempts, err := database.GetRecentFailedAttempts("admin:"+clientIP, LockoutDuration)
	if err != nil {
		log.Printf("Error checking admin failed attempts: %v", err)
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

	if err := bcrypt.CompareHashAndPassword([]byte(config.AdminPassword), []byte(req.Password)); err != nil {
		_ = database.RecordLoginAttempt("admin:"+clientIP, false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	_ = database.RecordLoginAttempt("admin:"+clientIP, true)

	token, err := createAdminSession(clientIP)
	if err != nil {
		log.Printf("Error creating admin session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(AdminCookieName, token, int(AdminSessionTTL.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Login successful"})
}

func AdminLogout(c *gin.Context) {
	token, _ := c.Cookie(AdminCookieName)
	if token != "" {
		revokeAdminSession(token)
	}
	c.SetCookie(AdminCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
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
	req.IP = strings.TrimSpace(req.IP)
	if net.ParseIP(req.IP) == nil || req.IP == "0.0.0.0" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address"})
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
	_ = iptables.SaveRules()
	c.JSON(http.StatusOK, gin.H{"message": "IP added successfully"})
}

func DeleteWhitelistIP(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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
	_ = iptables.RemoveIPFromWhitelist(targetIP)
	_ = iptables.SaveRules()
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
	if len(req.NewPassword) < MinPasswordLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password too short (min 6)"})
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
	if len(req.NewPassword) < MinPasswordLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password too short (min 6)"})
		return
	}
	if err := database.UpdateAdminPassword(req.NewPassword); err != nil {
		log.Printf("Error updating admin password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}
	revokeAllAdminSessions()
	c.SetCookie(AdminCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Admin password updated successfully"})
}

func GetListenPort(c *gin.Context) {
	port, err := database.GetListenPort()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get port"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"port": port})
}

func UpdateListenPort(c *gin.Context) {
	var req struct {
		Port int `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Port must be 1-65535"})
		return
	}
	if req.Port == 22 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Port 22 is reserved"})
		return
	}

	oldPort, err := database.GetListenPort()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get current port"})
		return
	}
	if oldPort == req.Port {
		c.JSON(http.StatusOK, gin.H{"message": "Port unchanged", "port": req.Port})
		return
	}

	if err := database.UpdateListenPort(req.Port); err != nil {
		log.Printf("Error updating listen port in DB: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save port"})
		return
	}

	if err := iptables.UpdateAuthPort(oldPort, req.Port); err != nil {
		log.Printf("Error updating firewall auth port: %v", err)
		_ = database.UpdateListenPort(oldPort)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update firewall port"})
		return
	}
	_ = iptables.SaveRules()

	c.JSON(http.StatusOK, gin.H{
		"message":  "Port updated, server reloading",
		"old_port": oldPort,
		"port":     req.Port,
	})

	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := serverctl.Reload(req.Port); err != nil {
			log.Printf("Failed to reload HTTP server on :%d: %v", req.Port, err)
		}
	}()
}

func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(AdminCookieName)
		if err != nil || !validateAdminSession(token) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			} else {
				c.Redirect(http.StatusFound, "/admin")
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// getClientIP 仅信任直连 RemoteAddr，防止 X-Forwarded-For / X-Real-IP 伪造白名单。
func getClientIP(c *gin.Context) string {
	ip := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	ip = strings.Trim(ip, "[]")

	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsUnspecified() {
		log.Printf("Warning: Invalid client IP: %s", ip)
		return ""
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return ""
}

func GetVOSConfig(c *gin.Context) {
	enabled, err := database.GetVOSSyncEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}
	interval, err := database.GetVOSSyncInterval()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}
	lastSync, _ := database.GetVOSLastSync()
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "VOS config updated successfully"})
}

func ManualVOSSync(c *gin.Context) {
	result, err := vos.ManualSync()
	if err != nil {
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
