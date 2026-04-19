package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

// Response structure sent back to master
type CommandResponse struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

var agentID string

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

	resp := CommandResponse{
		Agent:   agentID,
		Command: command,
		Output:  output,
		Success: success,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	resp := CommandResponse{
		Agent:   agentID,
		Command: shellCmd,
		Output:  output,
		Success: success,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func executeCommand(command string) (string, bool) {
	switch command {
	case "lock":
		return lockDevice()
	case "shutdown":
		return shutdownDevice()
	case "wallpaper":
		return changeWallpaper()
	case "info":
		return getSystemInfo()
	default:
		return fmt.Sprintf("Unknown command: %s", command), false
	}
}

func lockDevice() (string, bool) {
	switch runtime.GOOS {
	case "windows":
		err := exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
		if err != nil {
			return "Lock failed: " + err.Error(), false
		}
	case "linux":
		err := exec.Command("loginctl", "lock-session").Run()
		if err != nil {
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
	case  "linux":
		err = exec.Command("shutdown", "-h", "now").Run()
	}
	if err != nil {
		return "Shutdown failed: " + err.Error(), false
	}
	return "Shutting down...", true
}

func changeWallpaper() (string, bool) {
	// Placeholder - customize path as needed
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public class Wallpaper{[DllImport("user32.dll")]public static extern int SystemParametersInfo(int a,int b,string c,int d);}'; [Wallpaper]::SystemParametersInfo(20,0,"C:\Windows\Web\Wallpaper\Windows\img0.jpg",3)`
		err := exec.Command("powershell", "-Command", script).Run()
		if err != nil {
			return "Wallpaper change failed: " + err.Error(), false
		}
	case "linux":
		err := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", "file:///usr/share/backgrounds/warty-final-ubuntu.png").Run()
		if err != nil {
			return "Wallpaper change failed: " + err.Error(), false
		}
	
	}
	return "Wallpaper changed successfully", true
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

func main() {
	port := "1010"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	agentID = "Agent-" + port

	mux := http.NewServeMux()
	mux.HandleFunc("/command", commandHandler)
	mux.HandleFunc("/terminal", terminalHandler)

	fmt.Printf("[%s] Agent running on port %s\n", agentID, port)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Server error:", err)
		os.Exit(1)
	}
}