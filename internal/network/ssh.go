package network

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/exec"
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
		Status:      "stopped",
		AuthMethods: make(map[string]int),
	}

	// Check SSH Status (port 22)
	// Try netstat first, then ss, then check /proc/net/tcp
	stats.Status = "stopped"
	if checkPort22Open() {
		stats.Status = "running"
	}

	// Connections
	stats.Connections = getSSHConnectionCount()

	// Sessions (who)
	// Use chroot to run 'who' on host
	cmd := exec.Command("chroot", "/hostfs", "who")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		var sessions []interface{}
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				sessions = append(sessions, line)
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
	// Try multiple key types
	keyFiles := []string{
		"/hostfs/etc/ssh/ssh_host_ed25519_key.pub",
		"/hostfs/etc/ssh/ssh_host_rsa_key.pub",
		"/hostfs/etc/ssh/ssh_host_ecdsa_key.pub",
	}
	for _, keyFile := range keyFiles {
		if content, err := ioutil.ReadFile(keyFile); err == nil {
			parts := strings.Fields(string(content))
			if len(parts) >= 2 {
				stats.HostKey = parts[1] // The key itself, frontend can truncate or hash if needed
				// Or run ssh-keygen -lf
				cmd := exec.Command("ssh-keygen", "-lf", keyFile)
				if out, err := cmd.Output(); err == nil {
					stats.HostKey = strings.TrimSpace(string(out))
				}
				break
			}
		}
	}

	// History Size (approx lines in auth.log)
	// Just count lines in /hostfs/var/log/auth.log
	if file, err := os.Open("/hostfs/var/log/auth.log"); err == nil {
		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			count++
		}
		stats.HistorySize = count
		file.Close()
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
