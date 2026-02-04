package database

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"iptables-safe/models"
)

var DB *sql.DB

func InitDB(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	if err = DB.Ping(); err != nil {
		return err
	}

	if err = createTables(); err != nil {
		return err
	}

	if err = initDefaultConfig(); err != nil {
		return err
	}

	return nil
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS whitelist_ips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip TEXT NOT NULL UNIQUE,
			description TEXT,
			is_permanent BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_password TEXT NOT NULL,
			admin_password TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS login_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_login_attempts_ip ON login_attempts(ip, timestamp)`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func initDefaultConfig() error {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM config").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		userHash, err := bcrypt.GenerateFromPassword([]byte("022018"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		adminHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		_, err = DB.Exec("INSERT INTO config (user_password, admin_password) VALUES (?, ?)",
			string(userHash), string(adminHash))
		if err != nil {
			return err
		}
		log.Println("Default passwords initialized: user=022018, admin=admin123")
	}

	return nil
}

func GetConfig() (*models.Config, error) {
	config := &models.Config{}
	err := DB.QueryRow("SELECT id, user_password, admin_password FROM config LIMIT 1").
		Scan(&config.ID, &config.UserPassword, &config.AdminPassword)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func UpdateUserPassword(newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec("UPDATE config SET user_password = ? WHERE id = 1", string(hash))
	return err
}

func UpdateAdminPassword(newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec("UPDATE config SET admin_password = ? WHERE id = 1", string(hash))
	return err
}

func AddWhitelistIP(ip, description string, isPermanent bool, expiresAt time.Time) error {
	_, err := DB.Exec(
		"INSERT OR REPLACE INTO whitelist_ips (ip, description, is_permanent, expires_at) VALUES (?, ?, ?, ?)",
		ip, description, isPermanent, expiresAt,
	)
	return err
}

func GetAllWhitelistIPs() ([]models.WhitelistIP, error) {
	rows, err := DB.Query("SELECT id, ip, description, is_permanent, created_at, expires_at FROM whitelist_ips")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []models.WhitelistIP
	for rows.Next() {
		var ip models.WhitelistIP
		var expiresAt sql.NullTime
		err := rows.Scan(&ip.ID, &ip.IP, &ip.Description, &ip.IsPermanent, &ip.CreatedAt, &expiresAt)
		if err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			ip.ExpiresAt = expiresAt.Time
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func DeleteWhitelistIP(id int) error {
	_, err := DB.Exec("DELETE FROM whitelist_ips WHERE id = ?", id)
	return err
}

func IsIPWhitelisted(ip string) (bool, error) {
	var count int
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM whitelist_ips WHERE ip = ? AND (is_permanent = 1 OR expires_at > datetime('now'))",
		ip,
	).Scan(&count)
	return count > 0, err
}

func RecordLoginAttempt(ip string, success bool) error {
	_, err := DB.Exec("INSERT INTO login_attempts (ip, success) VALUES (?, ?)", ip, success)
	return err
}

func GetRecentFailedAttempts(ip string, duration time.Duration) (int, error) {
	var count int
	cutoff := time.Now().Add(-duration)
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM login_attempts WHERE ip = ? AND success = 0 AND timestamp > ?",
		ip, cutoff,
	).Scan(&count)
	return count, err
}

func CleanupExpiredIPs() error {
	_, err := DB.Exec("DELETE FROM whitelist_ips WHERE is_permanent = 0 AND expires_at < datetime('now')")
	return err
}

func CleanupOldLoginAttempts() error {
	cutoff := time.Now().Add(-24 * time.Hour)
	_, err := DB.Exec("DELETE FROM login_attempts WHERE timestamp < ?", cutoff)
	return err
}

func GetActiveWhitelistIPs() ([]string, error) {
	rows, err := DB.Query(
		"SELECT ip FROM whitelist_ips WHERE is_permanent = 1 OR expires_at > datetime('now')",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, nil
}
