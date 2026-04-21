package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Request struct {
	Command       string `json:"command"`
	Wallpaper     string `json:"wallpaper,omitempty"`
	WallpaperName string `json:"wallpaper_name,omitempty"`
	WallpaperData string `json:"wallpaper_data,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func main() {
	controllerAddr := flag.String("controller", "10.251.91.88:9000", "controller address")
	name := flag.String("name", "", "optional device name")
	flag.Parse()

	deviceName := strings.TrimSpace(*name)
	if deviceName == "" {
		host, err := os.Hostname()
		if err != nil {
			deviceName = "unknown-device"
		} else {
			deviceName = host
		}
	}

	for {
		if err := runAgent(*controllerAddr, deviceName); err != nil {
			log.Printf("connection lost: %v", err)
		}

		time.Sleep(3 * time.Second)
	}
}

func runAgent(controllerAddr, deviceName string) error {
	conn, err := net.Dial("tcp", controllerAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("connected to controller %s as %s", controllerAddr, deviceName)

	if err := json.NewEncoder(conn).Encode(map[string]string{"name": deviceName}); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(conn)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			return err
		}

		resp := Response{OK: true, Message: "command executed successfully"}
		if err := executeCommand(req); err != nil {
			resp.OK = false
			resp.Message = err.Error()
		}

		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
}

func executeCommand(req Request) error {
	switch strings.ToLower(strings.TrimSpace(req.Command)) {
	case "lock":
		return lockDevice()
	case "shutdown":
		return shutdownDevice()
	case "wallpaper":
		if strings.TrimSpace(req.WallpaperData) == "" {
			return errors.New("wallpaper image data is required for wallpaper command")
		}
		return saveAndSetWallpaper(req)
	default:
		return fmt.Errorf("unsupported command: %s", req.Command)
	}
}

func saveAndSetWallpaper(req Request) error {
	imageBytes, err := base64.StdEncoding.DecodeString(req.WallpaperData)
	if err != nil {
		return fmt.Errorf("failed to decode wallpaper: %w", err)
	}

	fileName := strings.TrimSpace(req.WallpaperName)
	if fileName == "" {
		fileName = "wallpaper.jpg"
	}

	targetPath := filepath.Join(os.TempDir(), "distributed-controller-"+filepath.Base(fileName))
	if err := os.WriteFile(targetPath, imageBytes, 0644); err != nil {
		return fmt.Errorf("failed to save wallpaper: %w", err)
	}

	return changeWallpaper(targetPath)
}

func lockDevice() error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
	case "linux":
		return exec.Command("loginctl", "lock-session").Run()
	default:
		return fmt.Errorf("lock is not implemented for %s", runtime.GOOS)
	}
}

func shutdownDevice() error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("shutdown", "/s", "/t", "0").Run()
	case "linux":
		return exec.Command("shutdown", "now").Run()
	case "darwin":
		return exec.Command("sudo", "shutdown", "-h", "now").Run()
	default:
		return fmt.Errorf("shutdown is not implemented for %s", runtime.GOOS)
	}
}

func changeWallpaper(path string) error {
	switch runtime.GOOS {
	case "windows":
		return changeWallpaperWindows(path)
	case "linux":
		return changeWallpaperLinux(path)
	default:
		return fmt.Errorf("wallpaper change is not implemented for %s", runtime.GOOS)
	}
}

func changeWallpaperWindows(path string) error {
	script := fmt.Sprintf(
		`Add-Type @"
using System.Runtime.InteropServices;
public class Wallpaper {
  [DllImport("user32.dll", SetLastError = true)]
  public static extern bool SystemParametersInfo(int uAction, int uParam, string lpvParam, int fuWinIni);
}
"@; [Wallpaper]::SystemParametersInfo(20, 0, "%s", 3)`,
		escapeForPowerShell(path),
	)
	return exec.Command("powershell", "-NoProfile", "-Command", script).Run()
}

func changeWallpaperLinux(path string) error {
	fileURI := "file://" + filepath.ToSlash(path)

	commands := []struct {
		name string
		args []string
	}{
		{
			name: "gsettings",
			args: []string{"set", "org.gnome.desktop.background", "picture-uri", fileURI},
		},
		{
			name: "gsettings",
			args: []string{"set", "org.gnome.desktop.background", "picture-uri-dark", fileURI},
		},
		{
			name: "gsettings",
			args: []string{"set", "org.cinnamon.desktop.background", "picture-uri", fileURI},
		},
		{
			name: "gsettings",
			args: []string{"set", "org.mate.background", "picture-filename", path},
		},
		{
			name: "xfconf-query",
			args: []string{"-c", "xfce4-desktop", "-p", "/backdrop/screen0/monitor0/image-path", "-s", path},
		},
		{
			name: "xfconf-query",
			args: []string{"-c", "xfce4-desktop", "-p", "/backdrop/screen0/monitor0/workspace0/last-image", "-s", path},
		},
		{
			name: "feh",
			args: []string{"--bg-fill", path},
		},
	}

	var attempted []string
	for _, command := range commands {
		if _, err := exec.LookPath(command.name); err != nil {
			continue
		}

		attempted = append(attempted, command.name)
		if err := exec.Command(command.name, command.args...).Run(); err == nil {
			return nil
		}
	}

	if len(attempted) == 0 {
		return errors.New("no supported Linux wallpaper tool found (tried gsettings, xfconf-query, feh)")
	}

	return fmt.Errorf("failed to change wallpaper with available Linux tools: %s", strings.Join(attempted, ", "))
}

func escapeForPowerShell(value string) string {
	return strings.ReplaceAll(value, `"`, "`\"")
}
