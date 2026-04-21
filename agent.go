package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/grandcat/zeroconf"
)

// CommandResponse is the structure sent back to the controller
type CommandResponse struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

var agentID string

const mdnsService = "_distcontrol._tcp"
const mdnsDomain = "local."

// ─── Shared response helper ───────────────────────────────────────────────────

func respond(w http.ResponseWriter, cmd, output string, success bool) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CommandResponse{
		Agent:   agentID,
		Command: cmd,
		Output:  output,
		Success: success,
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func commandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	command := string(body)
	fmt.Printf("[%s] Received command: %s\n", agentID, command)

	output, success := executeCommand(command)
	respond(w, command, output, success)
}

func terminalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	shellCmd := string(body)
	fmt.Printf("[%s] Terminal command: %s\n", agentID, shellCmd)

	output, success := runShell(shellCmd)
	respond(w, shellCmd, output, success)
}

// wallpaperHandler receives raw image bytes from the controller, saves them to
// a temporary file, then applies the image as the desktop wallpaper.
func wallpaperHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if len(data) == 0 {
		respond(w, "wallpaper", "ERROR: received empty image data", false)
		return
	}

	tmpFile := filepath.Join(os.TempDir(), "agent_wallpaper.jpg")
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		respond(w, "wallpaper", "Failed to save image: "+err.Error(), false)
		return
	}

	fmt.Printf("[%s] Wallpaper saved to %s (%d bytes)\n", agentID, tmpFile, len(data))

	output, success := applyWallpaper(tmpFile)
	respond(w, "wallpaper", output, success)
}

// ─── Command implementations ──────────────────────────────────────────────────

func executeCommand(command string) (string, bool) {
	switch command {
	case "lock":
		return lockDevice()
	case "shutdown":
		return shutdownDevice()
	case "wallpaper":
		return changeDefaultWallpaper()
	case "info":
		return getSystemInfo()
	default:
		return fmt.Sprintf("Unknown command: %s", command), false
	}
}

func lockDevice() (string, bool) {
	switch runtime.GOOS {
	case "windows":
		if err := exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run(); err != nil {
			return "Lock failed: " + err.Error(), false
		}
	case "linux":
		if err := exec.Command("loginctl", "lock-session").Run(); err != nil {
			return "Lock failed: " + err.Error(), false
		}
	
	}
	return "Device locked successfully", true
}

func shutdownDevice() (string, bool) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("shutdown", "/s", "/t", "0").Run()
	case "linux", "darwin":
		err = exec.Command("shutdown", "-h", "now").Run()
	}
	if err != nil {
		return "Shutdown failed: " + err.Error(), false
	}
	return "Shutting down...", true
}

// changeDefaultWallpaper applies a built-in OS wallpaper (legacy string command).
func changeDefaultWallpaper() (string, bool) {
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public class Wallpaper{[DllImport("user32.dll")]public static extern int SystemParametersInfo(int a,int b,string c,int d);}'; [Wallpaper]::SystemParametersInfo(20,0,"C:\Windows\Web\Wallpaper\Windows\img0.jpg",3)`
		if err := exec.Command("powershell", "-Command", script).Run(); err != nil {
			return "Wallpaper change failed: " + err.Error(), false
		}
	case "linux":
		if err := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", "file:///usr/share/backgrounds/warty-final-ubuntu.png").Run(); err != nil {
			return "Wallpaper change failed: " + err.Error(), false
		}

	}
	return "Wallpaper changed to default successfully", true
}

// applyWallpaper sets the desktop wallpaper to the given local file path.
func applyWallpaper(path string) (string, bool) {
	switch runtime.GOOS {
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public class W{[DllImport("user32.dll")]public static extern int SystemParametersInfo(int a,int b,string c,int d);}'; [W]::SystemParametersInfo(20,0,"%s",3)`,
			path,
		)
		if err := exec.Command("powershell", "-Command", script).Run(); err != nil {
			return "Wallpaper failed: " + err.Error(), false
		}
	case "linux":
		if err := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", "file://"+path).Run(); err != nil {
			return "Wallpaper failed: " + err.Error(), false
		}
	
	default:
		return fmt.Sprintf("Unsupported OS: %s", runtime.GOOS), false
	}
	return fmt.Sprintf("Wallpaper applied successfully (%s)", path), true
}

func getSystemInfo() (string, bool) {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("OS: %s | Arch: %s | Hostname: %s", runtime.GOOS, runtime.GOARCH, hostname), true
}

func runShell(command string) (string, bool) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", command)
	default:
		cmd = exec.Command("sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error: %s\nOutput: %s", err.Error(), string(output)), false
	}
	return string(output), true
}

// ─── mDNS registration ────────────────────────────────────────────────────────

// registerMDNS advertises this agent on the local network so the controller
// can discover it without any hardcoded IPs.
// The instance name is the agentID (e.g. "Agent-1010").
// Returns a cleanup function — call it on shutdown.
func registerMDNS(port int) (func(), error) {
	hostname, _ := os.Hostname()
	server, err := zeroconf.Register(
		agentID,      // instance name   → shown as Agent.ID on the controller
		mdnsService,  // service type
		mdnsDomain,   // domain
		port,         // port
		[]string{"version=1"}, // TXT records (optional metadata)
		nil,          // bind to all interfaces
	)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[%s] mDNS registered as %q on %s:%d\n", agentID, agentID, hostname, port)
	return func() { server.Shutdown() }, nil
}

// ─── Entry point ──────────────────────────────────────────────────────────────

func main() {
	portStr := "1010"
	if len(os.Args) > 1 {
		portStr = os.Args[1]
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Println("Invalid port:", portStr)
		os.Exit(1)
	}

	agentID = "Agent-" + portStr

	stopMDNS, err := registerMDNS(port)
	if err != nil {
		fmt.Printf("[%s] WARNING: mDNS registration failed: %v\n", agentID, err)
	} else {
		defer stopMDNS()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/command", commandHandler)
	mux.HandleFunc("/terminal", terminalHandler)
	mux.HandleFunc("/wallpaper", wallpaperHandler)

	fmt.Printf("[%s] Agent running on port %d\n", agentID, port)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Printf("\n[%s] Shutting down...\n", agentID)
		if stopMDNS != nil {
			stopMDNS()
		}
		os.Exit(0)
	}()

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Server error:", err)
		os.Exit(1)
	}
}