package vos

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"iptables-safe/database"
	"iptables-safe/iptables"
)

type SyncResult struct {
	TotalIPs  int
	NewIPs    int
	FailedIPs int
	SyncTime  time.Time
	Error     error
}

var (
	mysqlAvailable     bool
	mysqlCheckDone     bool
	mysqlCheckMutex    sync.Mutex
	lastMySQLCheckTime time.Time
	mysqlCheckInterval = 5 * time.Minute
)

func checkMySQLAvailability() bool {
	mysqlCheckMutex.Lock()
	defer mysqlCheckMutex.Unlock()

	if mysqlCheckDone && time.Since(lastMySQLCheckTime) < mysqlCheckInterval {
		return mysqlAvailable
	}

	cmd := exec.Command("mysql", "--version")
	if err := cmd.Run(); err != nil {
		log.Println("MySQL client not found, VOS sync will be disabled")
		mysqlAvailable = false
		mysqlCheckDone = true
		lastMySQLCheckTime = time.Now()
		return false
	}

	cmd = exec.Command("mysql", "-uroot", "-e", "SELECT 1", "vos3000")
	cmd.Env = append(cmd.Env, "MYSQL_PWD=")
	err := cmd.Run()

	if err != nil {
		log.Printf("MySQL connection test failed: %v. VOS sync will be disabled", err)
		mysqlAvailable = false
	} else {
		log.Println("MySQL connection successful, VOS sync enabled")
		mysqlAvailable = true
	}

	mysqlCheckDone = true
	lastMySQLCheckTime = time.Now()
	return mysqlAvailable
}

func FetchGatewayIPs() (map[string]string, error) {
	if !checkMySQLAvailability() {
		return nil, fmt.Errorf("MySQL is not available")
	}

	ipMap := make(map[string]string)

	tables := []struct {
		name  string
		query string
	}{
		{
			name:  "e_gatewaymapping",
			query: "SELECT remoteips FROM e_gatewaymapping WHERE remoteips IS NOT NULL AND remoteips != ''",
		},
		{
			name:  "e_gatewayrouting",
			query: "SELECT remoteips FROM e_gatewayrouting WHERE remoteips IS NOT NULL AND remoteips != ''",
		},
	}

	for _, table := range tables {
		cmd := exec.Command("mysql", "-uroot", "-e", table.query, "vos3000")
		cmd.Env = append(cmd.Env, "MYSQL_PWD=")

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Warning: Failed to query %s: %v", table.name, err)
			mysqlCheckMutex.Lock()
			mysqlAvailable = false
			mysqlCheckMutex.Unlock()
			continue
		}

		lines := strings.Split(string(output), "\n")
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}

			remoteips := strings.TrimSpace(line)
			if remoteips == "" || remoteips == "remoteips" {
				continue
			}

			ips := strings.Split(remoteips, ",")
			for _, ip := range ips {
				ip = strings.TrimSpace(ip)
				if ip != "" && isValidIP(ip) {
					ipMap[ip] = fmt.Sprintf("VOS-%s", table.name)
				}
			}
		}
	}

	return ipMap, nil
}

func SyncIPsToWhitelist() (*SyncResult, error) {
	result := &SyncResult{
		SyncTime: time.Now(),
	}

	if !checkMySQLAvailability() {
		result.Error = fmt.Errorf("MySQL not available, skipping VOS sync")
		return result, result.Error
	}

	log.Println("Starting VOS IP synchronization...")

	ipMap, err := FetchGatewayIPs()
	if err != nil {
		result.Error = err
		log.Printf("VOS sync skipped: %v", err)
		return result, err
	}

	result.TotalIPs = len(ipMap)
	log.Printf("Fetched %d unique IPs from VOS database", result.TotalIPs)

	for ip, source := range ipMap {
		whitelisted, err := database.IsIPWhitelisted(ip)
		if err != nil {
			log.Printf("Error checking if IP %s is whitelisted: %v", ip, err)
			result.FailedIPs++
			continue
		}

		if whitelisted {
			continue
		}

		if err := database.AddWhitelistIP(ip, source, true, time.Time{}); err != nil {
			log.Printf("Failed to add IP %s to database: %v", ip, err)
			result.FailedIPs++
			continue
		}

		if err := iptables.AddIPToWhitelist(ip); err != nil {
			log.Printf("Failed to add IP %s to iptables: %v", ip, err)
			result.FailedIPs++
			continue
		}

		result.NewIPs++
	}

	if err := database.UpdateVOSLastSync(); err != nil {
		log.Printf("Warning: Failed to update last sync time: %v", err)
	}

	log.Printf("VOS sync completed: %d total IPs, %d new IPs added, %d failed",
		result.TotalIPs, result.NewIPs, result.FailedIPs)

	return result, nil
}

func ManualSync() (*SyncResult, error) {
	return SyncIPsToWhitelist()
}

func isValidIP(ip string) bool {
	if ip == "" || ip == "0.0.0.0" {
		return false
	}

	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		num := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
			num = num*10 + int(c-'0')
		}
		if num > 255 {
			return false
		}
	}
	return true
}
