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

	// 第一步：清空规则，先保持OUTPUT ACCEPT防止SSH断连
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

	// 第二步：添加INPUT链基础规则（只开放8888管理端口，22端口通过白名单IP开放）
	runCommand("iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT")
	runCommand("iptables", "-A", "INPUT", "-p", "tcp", "--dport", "8888", "-j", "ACCEPT")

	// 第三步：添加ESTABLISHED,RELATED规则（兼容state和conntrack模块）
	if err := runCommand("iptables", "-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		log.Println("state module not available for INPUT, trying conntrack...")
		if err := runCommand("iptables", "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			log.Printf("Warning: state/conntrack not available for INPUT, using sport fallback")
		}
	}

	// 第四步：添加OUTPUT链规则
	runCommand("iptables", "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT")
	runCommand("iptables", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT")
	runCommand("iptables", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "ACCEPT")
	// 允许Web管理服务回复客户端（源端口8888的出站流量）
	runCommand("iptables", "-A", "OUTPUT", "-p", "tcp", "--sport", "8888", "-j", "ACCEPT")

	if err := runCommand("iptables", "-A", "OUTPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		log.Println("state module not available for OUTPUT, trying conntrack...")
		if err := runCommand("iptables", "-A", "OUTPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			log.Printf("Warning: state/conntrack not available for OUTPUT, using sport fallback")
		}
	}

	// 第五步：最后才设置OUTPUT为DROP（此时所有规则已就绪）
	if err := runCommand("iptables", "-P", "OUTPUT", "DROP"); err != nil {
		return fmt.Errorf("failed to set OUTPUT policy to DROP: %v", err)
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

	// INPUT链：允许该IP入站
	cmd := []string{"iptables", "-I", "INPUT", "1", "-s", ip, "-j", "ACCEPT"}
	if err := runCommand(cmd...); err != nil {
		return fmt.Errorf("failed to add IP %s to INPUT whitelist: %v", ip, err)
	}

	// OUTPUT链：允许向该IP出站
	cmdOut := []string{"iptables", "-I", "OUTPUT", "1", "-d", ip, "-j", "ACCEPT"}
	if err := runCommand(cmdOut...); err != nil {
		log.Printf("Warning: failed to add IP %s to OUTPUT whitelist: %v", ip, err)
	}

	log.Printf("Added IP %s to whitelist (INPUT+OUTPUT)", ip)
	return nil
}

func RemoveIPFromWhitelist(ip string) error {
	if !isValidIP(ip) {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	// 删除INPUT链规则
	cmd := []string{"iptables", "-D", "INPUT", "-s", ip, "-j", "ACCEPT"}
	if err := runCommand(cmd...); err != nil {
		return fmt.Errorf("failed to remove IP %s from INPUT whitelist: %v", ip, err)
	}

	// 删除OUTPUT链规则
	cmdOut := []string{"iptables", "-D", "OUTPUT", "-d", ip, "-j", "ACCEPT"}
	if err := runCommand(cmdOut...); err != nil {
		log.Printf("Warning: failed to remove IP %s from OUTPUT whitelist: %v", ip, err)
	}

	log.Printf("Removed IP %s from whitelist (INPUT+OUTPUT)", ip)
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
