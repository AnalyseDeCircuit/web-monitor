package network

import (
	"bufio"
	"io/ioutil"
	stdnet "net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/process"
)

var (
	sshStatsCache    types.SSHStats
	lastSSHTime      time.Time
	sshAuthLogOffset int64
	sshAuthCounters  = map[string]int{"publickey": 0, "password": 0, "other": 0, "failed": 0}
	sshStatsLock     sync.Mutex
)

// GetSSHStats 获取SSH统计信息
func GetSSHStats() types.SSHStats {
	sshStatsLock.Lock()
	defer sshStatsLock.Unlock()

	if time.Since(lastSSHTime) < 120*time.Second && sshStatsCache.Status != "" {
		return sshStatsCache
	}

	stats := types.SSHStats{
		Status:      "Stopped",
		AuthMethods: make(map[string]int),
	}

	// Check SSH Status (port 22)
	// Try netstat first, then ss, then check /proc/net/tcp
	if checkPort22Open() {
		stats.Status = "Running"
	}

	// Connections
	stats.Connections = getSSHConnectionCount()

	// Sessions (who) - mimic legacy behavior, only remote sessions with valid IPs
	cmd := exec.Command("chroot", "/hostfs", "who")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		var sessions []types.SSHSession
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) < 5 {
				continue
			}

			user := parts[0]
			startedStr := parts[2] + " " + parts[3]

			// Try to parse time and convert to ISO 8601 (UTC)
			// who output format: YYYY-01-02 15:04
			if t, err := time.ParseInLocation("2006-01-02 15:04", startedStr, time.Local); err == nil {
				startedStr = t.UTC().Format(time.RFC3339)
			}

			// Extract IP from fields with parentheses, only valid IPs
			ipsFound := make(map[string]bool)
			ip := ""
			for _, part := range parts {
				if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
					candidate := part[1 : len(part)-1]
					if stdnet.ParseIP(candidate) != nil && !ipsFound[candidate] {
						ipsFound[candidate] = true
						ip = candidate
						break
					}
				}
			}

			// Only add if we found a valid IP (remote session)
			if ip != "" {
				sessions = append(sessions, types.SSHSession{
					User:      user,
					IP:        ip,
					LoginTime: startedStr,
				})
			}
		}
		stats.Sessions = sessions
	}

	// Auth Logs (Incremental)
	updateSSHAuthStats()
	stats.AuthMethods = make(map[string]int)
	for k, v := range sshAuthCounters {
		stats.AuthMethods[k] = v
	}
	stats.FailedLogins = sshAuthCounters["failed"]

	// Host Key Fingerprint
	// Try multiple key types and generate fingerprint
	keyFiles := []string{
		"/hostfs/etc/ssh/ssh_host_ed25519_key.pub",
		"/hostfs/etc/ssh/ssh_host_rsa_key.pub",
		"/hostfs/etc/ssh/ssh_host_ecdsa_key.pub",
	}
	for _, keyFile := range keyFiles {
		// Check if file exists
		if _, err := os.Stat(keyFile); err == nil {
			// Generate fingerprint using ssh-keygen
			cmd := exec.Command("ssh-keygen", "-lf", keyFile)
			if out, err := cmd.Output(); err == nil {
				stats.HostKey = strings.TrimSpace(string(out))
				break
			}
		}
	}

	// History Size (known_hosts) - count entries from known_hosts files on host
	knownHostsPaths := []string{
		"/root/.ssh/known_hosts",
		"/hostfs/root/.ssh/known_hosts",
		os.ExpandEnv("$HOME/.ssh/known_hosts"),
	}
	globPatterns := []string{
		"/home/*/.ssh/known_hosts",
		"/hostfs/home/*/.ssh/known_hosts",
	}
	for _, pattern := range globPatterns {
		if matches, err := filepath.Glob(pattern); err == nil {
			knownHostsPaths = append(knownHostsPaths, matches...)
		}
	}
	for _, path := range knownHostsPaths {
		if path == "" {
			continue
		}
		if content, err := ioutil.ReadFile(path); err == nil {
			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			if len(lines) > 0 {
				stats.HistorySize = len(lines)
				break
			}
		}
	}

	// OOM Risk Processes (sshd)
	procs, _ := process.Processes()
	for _, p := range procs {
		name, _ := p.Name()
		if name == "sshd" {
			memPercent, _ := p.MemoryPercent()
			if memPercent > 1.0 { // Only show if > 1%
				stats.OOMRiskProcesses = append(stats.OOMRiskProcesses, types.ProcessInfo{
					PID:           p.Pid,
					Name:          name,
					MemoryPercent: float64(int(memPercent*10)) / 10.0,
				})
			}
			stats.SSHProcessMemory += float64(memPercent)
		}
	}

	sshStatsCache = stats
	lastSSHTime = time.Now()
	return stats
}

func checkPort22Open() bool {
	// Method 1: netstat
	cmd := exec.Command("netstat", "-tuln")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), ":22 ") {
		return true
	}

	// Method 2: ss
	cmd = exec.Command("ss", "-tuln")
	out, err = cmd.Output()
	if err == nil && strings.Contains(string(out), ":22 ") {
		return true
	}

	// Method 3: /proc/net/tcp (if mounted from host or using host net)
	// Port 22 is 0016 in hex. State 0A is LISTEN.
	content, err := ioutil.ReadFile("/proc/net/tcp")
	if err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			localAddr := fields[1]
			state := fields[3]
			if strings.HasSuffix(localAddr, ":0016") && state == "0A" {
				return true
			}
		}
	}

	// Method 4: /hostfs/proc/net/tcp (if mounted)
	content, err = ioutil.ReadFile("/hostfs/proc/net/tcp")
	if err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			localAddr := fields[1]
			state := fields[3]
			if strings.HasSuffix(localAddr, ":0016") && state == "0A" {
				return true
			}
		}
	}

	return false
}

func getSSHConnectionCount() int {
	// Check /proc/net/tcp and tcp6 for port 22 connections
	count := 0
	// Try host proc first if available
	if c := countSSHConnectionsFromProc("/hostfs/proc/net/tcp"); c > 0 {
		count += c
	} else if c := countSSHConnectionsFromProc("/proc/net/tcp"); c > 0 {
		count += c
	}

	if c := countSSHConnectionsFromProc("/hostfs/proc/net/tcp6"); c > 0 {
		count += c
	} else if c := countSSHConnectionsFromProc("/proc/net/tcp6"); c > 0 {
		count += c
	}
	return count
}

func countSSHConnectionsFromProc(path string) int {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(content), "\n")
	count := 0
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// local_address is field 1 (0-indexed)
		localAddr := fields[1]
		state := fields[3]

		// Port 22 is 0016 in hex
		if strings.HasSuffix(localAddr, ":0016") && state == "01" { // 01 is ESTABLISHED
			count++
		}
	}
	return count
}

func updateSSHAuthStats() {
	logPath := "/hostfs/var/log/auth.log"
	file, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return
	}

	// If file truncated, reset offset
	if stat.Size() < sshAuthLogOffset {
		sshAuthLogOffset = 0
		// Reset counters too? Maybe not, to keep history.
	}

	if _, err := file.Seek(sshAuthLogOffset, 0); err != nil {
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		sshAuthLogOffset += int64(len(line) + 1) // +1 for newline

		if !strings.Contains(line, "sshd") {
			continue
		}

		if strings.Contains(line, "Accepted publickey") {
			sshAuthCounters["publickey"]++
		} else if strings.Contains(line, "Accepted password") {
			sshAuthCounters["password"]++
		} else if strings.Contains(line, "Accepted") {
			sshAuthCounters["other"]++
		} else if strings.Contains(line, "Failed password") || strings.Contains(line, "Connection closed by authenticating user") {
			sshAuthCounters["failed"]++
		}
	}
}
