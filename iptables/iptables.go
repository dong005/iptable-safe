package iptables

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"iptables-safe/database"
)

var currentAuthPort int = 8899

func InitializeFirewall(authPort int) error {
	log.Println("Initializing firewall rules...")
	if authPort <= 0 {
		authPort = 8899
	}
	currentAuthPort = authPort

	// 默认拒绝入站；仅开放认证口；登录后该 IP 全端口放行。出站 ACCEPT。
	initCommands := [][]string{
		{"iptables", "-F"},
		{"iptables", "-X"},
		{"iptables", "-P", "INPUT", "DROP"},
		{"iptables", "-P", "FORWARD", "DROP"},
		{"iptables", "-P", "OUTPUT", "ACCEPT"},
	}
	for _, cmd := range initCommands {
		if err := runCommand(cmd...); err != nil {
			return fmt.Errorf("failed to execute %v: %v", cmd, err)
		}
	}

	_ = runCommand("iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT")
	if err := runCommand("iptables", "-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		log.Println("state module not available for INPUT, trying conntrack...")
		if err := runCommand("iptables", "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			log.Printf("Warning: state/conntrack not available for INPUT")
		}
	}

	portStr := strconv.Itoa(authPort)
	if err := runCommand("iptables", "-A", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to open auth port %d: %v", authPort, err)
	}

	log.Printf("Firewall initialized (INPUT DROP, only :%d open)", authPort)

	if err := LoadWhitelistFromDB(); err != nil {
		log.Printf("Warning: Failed to load whitelist from database: %v", err)
	}
	log.Println("Firewall initialization complete")
	return nil
}

func UpdateAuthPort(oldPort, newPort int) error {
	if newPort <= 0 || newPort > 65535 {
		return fmt.Errorf("invalid port: %d", newPort)
	}
	oldStr := strconv.Itoa(oldPort)
	newStr := strconv.Itoa(newPort)

	// 先放行新端口，再删旧端口，避免管理口短暂不可达
	if err := runCommand("iptables", "-I", "INPUT", "1", "-p", "tcp", "--dport", newStr, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add new auth port %d: %v", newPort, err)
	}
	_ = runCommand("iptables", "-D", "INPUT", "-p", "tcp", "--dport", oldStr, "-j", "ACCEPT")
	currentAuthPort = newPort
	log.Printf("Auth port updated: %d -> %d", oldPort, newPort)
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
		return fmt.Errorf("failed to add IP %s to INPUT whitelist: %v", ip, err)
	}
	log.Printf("Added IP %s to whitelist (full inbound access)", ip)
	return nil
}

func RemoveIPFromWhitelist(ip string) error {
	if !isValidIP(ip) {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	if err := runCommand("iptables", "-D", "INPUT", "-s", ip, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to remove IP %s from INPUT whitelist: %v", ip, err)
	}
	_ = runCommand("iptables", "-D", "OUTPUT", "-d", ip, "-j", "ACCEPT")
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
				if field == ip || field == ip+"/32" {
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

func CheckIPInputRule(ip string) bool {
	return IsIPWhitelisted(ip)
}

func CheckIPOutputRule(ip string) bool {
	// 当前策略 OUTPUT 默认 ACCEPT，出站不受白名单限制
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true
	}
	first := strings.SplitN(string(output), "\n", 2)[0]
	if strings.Contains(first, "policy ACCEPT") {
		return true
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "ACCEPT") && (strings.Contains(line, ip) || strings.Contains(line, ip+"/32")) {
			return true
		}
	}
	return false
}
