package iptables

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"iptables-safe/database"
)

func InitializeFirewall() error {
	log.Println("Initializing firewall rules...")

	commands := [][]string{
		{"iptables", "-F"},
		{"iptables", "-X"},
		{"iptables", "-P", "INPUT", "DROP"},
		{"iptables", "-P", "FORWARD", "DROP"},
		{"iptables", "-P", "OUTPUT", "ACCEPT"},
		{"iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"},
		{"iptables", "-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"iptables", "-A", "INPUT", "-p", "tcp", "--dport", "22", "-j", "ACCEPT"},
		{"iptables", "-A", "INPUT", "-p", "tcp", "--dport", "8888", "-j", "ACCEPT"},
	}

	for _, cmd := range commands {
		if err := runCommand(cmd...); err != nil {
			return fmt.Errorf("failed to execute %v: %v", cmd, err)
		}
	}

	log.Println("Firewall initialized successfully")

	// 从数据库加载活跃的白名单IP
	if err := LoadWhitelistFromDB(); err != nil {
		log.Printf("Warning: Failed to load whitelist from database: %v", err)
	}

	return nil
}

func LoadWhitelistFromDB() error {
	log.Println("Loading whitelist IPs from database...")

	ips, err := database.GetActiveWhitelistIPs()
	if err != nil {
		return fmt.Errorf("failed to get whitelist IPs: %v", err)
	}

	if len(ips) == 0 {
		log.Println("No whitelist IPs to load")
		return nil
	}

	loaded := 0
	for _, ip := range ips {
		if err := AddIPToWhitelist(ip); err != nil {
			log.Printf("Failed to add IP %s to whitelist: %v", ip, err)
			continue
		}
		loaded++
	}

	log.Printf("Loaded %d whitelist IP(s) from database", loaded)
	return nil
}

func AddIPToWhitelist(ip string) error {
	if !isValidIP(ip) {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	if IsIPWhitelisted(ip) {
		log.Printf("IP %s is already whitelisted", ip)
		return nil
	}

	cmd := []string{"iptables", "-I", "INPUT", "1", "-s", ip, "-j", "ACCEPT"}
	if err := runCommand(cmd...); err != nil {
		return fmt.Errorf("failed to add IP %s to whitelist: %v", ip, err)
	}

	log.Printf("Added IP %s to whitelist", ip)
	return nil
}

func RemoveIPFromWhitelist(ip string) error {
	if !isValidIP(ip) {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	cmd := []string{"iptables", "-D", "INPUT", "-s", ip, "-j", "ACCEPT"}
	if err := runCommand(cmd...); err != nil {
		return fmt.Errorf("failed to remove IP %s from whitelist: %v", ip, err)
	}

	log.Printf("Removed IP %s from whitelist", ip)
	return nil
}

func IsIPWhitelisted(ip string) bool {
	cmd := exec.Command("iptables", "-L", "INPUT", "-n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to check iptables rules: %v", err)
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "ACCEPT") {
			fields := strings.Fields(line)
			for _, field := range fields {
				if field == ip {
					return true
				}
			}
		}
	}
	return false
}

func SaveRules() error {
	cmd := exec.Command("sh", "-c", "iptables-save > /etc/sysconfig/iptables")
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("sh", "-c", "iptables-save > /etc/iptables/rules.v4")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to save iptables rules: %v", err)
		}
	}
	log.Println("Iptables rules saved")
	return nil
}

func runCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

func isValidIP(ip string) bool {
	// 拒绝空字符串
	if ip == "" {
		return false
	}

	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	// 拒绝 0.0.0.0
	if ip == "0.0.0.0" {
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
