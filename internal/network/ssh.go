package network

import (
	"bufio"
	"encoding/hex"
	stdnet "net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	clkTckCache      int64
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
	procSessions := getSSHSessionsFromHostProc()
	if len(procSessions) > 0 {
		stats.Sessions = procSessions
		stats.Connections = len(procSessions)
	} else {
		whoSessions := getSSHSessionsFromWho()
		if len(whoSessions) > 0 {
			stats.Sessions = whoSessions
			stats.Connections = len(whoSessions)
		} else {
			stats.Connections = getSSHConnectionCount()
		}
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
		if content, err := os.ReadFile(path); err == nil {
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
	// Method 1: /proc/net/tcp (fastest, no exec)
	// Port 22 is 0016 in hex. State 0A is LISTEN.
	checkProc := func(path string) bool {
		content, err := os.ReadFile(path)
		if err != nil {
			return false
		}
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
		return false
	}

	if checkProc("/proc/net/tcp") || checkProc("/hostfs/proc/net/tcp") {
		return true
	}

	// Method 2: ss (fallback)
	cmd := exec.Command("ss", "-tuln")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), ":22 ") {
		return true
	}

	// Method 3: netstat (fallback)
	cmd = exec.Command("netstat", "-tuln")
	out, err = cmd.Output()
	if err == nil && strings.Contains(string(out), ":22 ") {
		return true
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
	content, err := os.ReadFile(path)
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

func getSSHSessionsFromWho() []types.SSHSession {
	// mimic legacy behavior, only remote sessions with valid IPs
	cmd := exec.Command("chroot", "/hostfs", "who")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

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
		ip := ""
		for _, part := range parts {
			if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
				candidate := part[1 : len(part)-1]
				parsed := stdnet.ParseIP(candidate)
				if parsed == nil {
					continue
				}
				if parsed.IsLoopback() {
					continue
				}
				ip = candidate
				break
			}
		}

		if ip != "" {
			sessions = append(sessions, types.SSHSession{
				User:      user,
				IP:        ip,
				LoginTime: startedStr,
			})
		}
	}
	return sessions
}

func getSSHSessionsFromHostProc() []types.SSHSession {
	clkTck := getClockTicksPerSecond()
	bootTime := getHostBootTimeUnix()
	remoteIPByInode := buildSSHRemoteIPByInode()
	if len(remoteIPByInode) == 0 {
		return nil
	}

	entries, err := os.ReadDir("/hostfs/proc")
	if err != nil {
		return nil
	}

	sessionsByKey := make(map[string]types.SSHSession)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(ent.Name())
		if err != nil || pid <= 0 {
			continue
		}

		arg0, ok := readProcArg0("/hostfs/proc/" + ent.Name() + "/cmdline")
		if !ok {
			continue
		}
		if !strings.HasPrefix(arg0, "sshd:") {
			continue
		}
		// Filter out non-session sshd processes.
		if strings.Contains(arg0, "[priv]") || strings.Contains(arg0, "[listener]") {
			continue
		}
		// Accept interactive sessions (pts) and subsystem sessions (notty).
		if !strings.Contains(arg0, "@pts/") && !strings.Contains(arg0, "@notty") {
			continue
		}

		user := parseSSHDUser(arg0)
		if user == "" {
			continue
		}

		ip := findRemoteIPForPID(pid, remoteIPByInode)
		if ip == "" {
			continue
		}
		parsed := stdnet.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() {
			continue
		}

		var loginTime string
		if ts, ok := procStartTimeRFC3339(pid, bootTime, clkTck); ok {
			loginTime = ts
		}
		if loginTime == "" {
			// Best-effort fallback; keep stable format for UI.
			loginTime = time.Now().UTC().Format(time.RFC3339)
		}

		key := user + "|" + ip + "|" + loginTime
		if _, exists := sessionsByKey[key]; exists {
			continue
		}
		sessionsByKey[key] = types.SSHSession{User: user, IP: ip, LoginTime: loginTime}
	}

	if len(sessionsByKey) == 0 {
		return nil
	}

	// Stable order not strictly required; keep insertion randomness acceptable.
	sessions := make([]types.SSHSession, 0, len(sessionsByKey))
	for _, s := range sessionsByKey {
		sessions = append(sessions, s)
	}
	return sessions
}

func readProcArg0(cmdlinePath string) (string, bool) {
	data, err := os.ReadFile(cmdlinePath)
	if err != nil || len(data) == 0 {
		return "", false
	}
	// cmdline is NUL-separated.
	parts := strings.Split(string(data), "\x00")
	if len(parts) == 0 {
		return "", false
	}
	arg0 := strings.TrimSpace(parts[0])
	if arg0 == "" {
		return "", false
	}
	return arg0, true
}

func parseSSHDUser(arg0 string) string {
	// Examples:
	//  - "sshd: user@pts/0"
	//  - "sshd: user@notty"
	//  - "sshd: user [priv]" (filtered earlier)
	s := strings.TrimSpace(strings.TrimPrefix(arg0, "sshd:"))
	if s == "" {
		return ""
	}
	at := strings.Index(s, "@")
	if at <= 0 {
		return ""
	}
	user := strings.TrimSpace(s[:at])
	if user == "" {
		return ""
	}
	return user
}

func buildSSHRemoteIPByInode() map[string]string {
	remoteIPByInode := make(map[string]string)
	for _, p := range []string{"/hostfs/proc/net/tcp", "/proc/net/tcp"} {
		mergeSSHRemoteIPByInode(remoteIPByInode, p, false)
		if len(remoteIPByInode) > 0 {
			break
		}
	}
	for _, p := range []string{"/hostfs/proc/net/tcp6", "/proc/net/tcp6"} {
		mergeSSHRemoteIPByInode(remoteIPByInode, p, true)
	}
	return remoteIPByInode
}

func mergeSSHRemoteIPByInode(dst map[string]string, path string, isV6 bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		localAddr := fields[1]
		remoteAddr := fields[2]
		state := fields[3]
		inode := fields[9]

		if inode == "" {
			continue
		}
		// Port 22 is 0016 in hex
		if !strings.HasSuffix(localAddr, ":0016") {
			continue
		}
		// 01 is ESTABLISHED
		if state != "01" {
			continue
		}

		ip, ok := parseProcNetIP(remoteAddr, isV6)
		if !ok {
			continue
		}
		if _, exists := dst[inode]; !exists {
			dst[inode] = ip
		}
	}
}

func parseProcNetIP(addrPort string, isV6 bool) (string, bool) {
	parts := strings.Split(addrPort, ":")
	if len(parts) != 2 {
		return "", false
	}
	hexAddr := parts[0]
	if isV6 {
		if len(hexAddr) != 32 {
			return "", false
		}
		b, err := hex.DecodeString(hexAddr)
		if err != nil || len(b) != 16 {
			return "", false
		}
		// /proc/net/tcp6 uses little-endian for each 32-bit word.
		for i := 0; i < 16; i += 4 {
			b[i+0], b[i+3] = b[i+3], b[i+0]
			b[i+1], b[i+2] = b[i+2], b[i+1]
		}
		ip := stdnet.IP(b)
		return ip.String(), true
	}
	if len(hexAddr) != 8 {
		return "", false
	}
	// IPv4 in /proc/net/tcp is little-endian
	b, err := hex.DecodeString(hexAddr)
	if err != nil || len(b) != 4 {
		return "", false
	}
	ip := stdnet.IP([]byte{b[3], b[2], b[1], b[0]})
	return ip.String(), true
}

func findRemoteIPForPID(pid int, remoteIPByInode map[string]string) string {
	fds, err := os.ReadDir("/hostfs/proc/" + strconv.Itoa(pid) + "/fd")
	if err != nil {
		return ""
	}
	for _, fd := range fds {
		link, err := os.Readlink("/hostfs/proc/" + strconv.Itoa(pid) + "/fd/" + fd.Name())
		if err != nil {
			continue
		}
		if !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
		if inode == "" {
			continue
		}
		if ip, ok := remoteIPByInode[inode]; ok {
			return ip
		}
	}
	return ""
}

func getHostBootTimeUnix() int64 {
	content, err := os.ReadFile("/hostfs/proc/stat")
	if err != nil {
		content, err = os.ReadFile("/proc/stat")
		if err != nil {
			return 0
		}
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "btime ") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				if v, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					return v
				}
			}
		}
	}
	return 0
}

func getClockTicksPerSecond() int64 {
	if clkTckCache > 0 {
		return clkTckCache
	}
	cmd := exec.Command("getconf", "CLK_TCK")
	out, err := cmd.Output()
	if err != nil {
		clkTckCache = 100
		return 100
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || v <= 0 {
		clkTckCache = 100
		return 100
	}
	clkTckCache = v
	return v
}

func procStartTimeRFC3339(pid int, bootTimeUnix int64, clkTck int64) (string, bool) {
	if bootTimeUnix <= 0 || clkTck <= 0 {
		return "", false
	}
	statPath := "/hostfs/proc/" + strconv.Itoa(pid) + "/stat"
	data, err := os.ReadFile(statPath)
	if err != nil {
		return "", false
	}
	line := string(data)
	idx := strings.LastIndex(line, ")")
	if idx < 0 {
		return "", false
	}
	after := strings.TrimSpace(line[idx+1:])
	fields := strings.Fields(after)
	// starttime is field 22 overall; after removing pid+comm, it's at index 19 (0-based).
	if len(fields) <= 19 {
		return "", false
	}
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil || startTicks <= 0 {
		return "", false
	}
	startUnix := bootTimeUnix + (startTicks / clkTck)
	return time.Unix(startUnix, 0).UTC().Format(time.RFC3339), true
}
