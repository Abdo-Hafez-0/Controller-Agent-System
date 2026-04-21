package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

type Agent struct {
	Name    string
	Conn    net.Conn
	Encoder *json.Encoder
	Decoder *json.Decoder
	mu      sync.Mutex
}

func main() {
	port := flag.Int("port", 9000, "controller listen port")
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen on port %d: %v", *port, err)
	}
	defer listener.Close()

	log.Printf("controller listening on :%d", *port)
	log.Println("agents can connect now")

	var (
		agentsMu sync.RWMutex
		agents   = make(map[string]*Agent)
	)

	go acceptAgents(listener, &agentsMu, agents)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		showMenu()
		fmt.Print("Choose a number: ")
		if !scanner.Scan() {
			log.Println("input closed, shutting down controller")
			return
		}

		switch strings.TrimSpace(scanner.Text()) {
		case "1":
			broadcast(Request{Command: "lock"}, &agentsMu, agents)
		case "2":
			broadcast(Request{Command: "shutdown"}, &agentsMu, agents)
		case "3":
			req, err := buildWallpaperRequest()
			if err != nil {
				log.Printf("wallpaper selection failed: %v", err)
				continue
			}

			broadcast(req, &agentsMu, agents)
		case "4":
			printAgents(&agentsMu, agents)
		case "5":
			log.Println("controller stopped")
			return
		default:
			log.Println("invalid choice")
		}
	}
}

func acceptAgents(listener net.Listener, agentsMu *sync.RWMutex, agents map[string]*Agent) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}

		go registerAgent(conn, agentsMu, agents)
	}
}

func registerAgent(conn net.Conn, agentsMu *sync.RWMutex, agents map[string]*Agent) {
	decoder := json.NewDecoder(conn)

	var hello map[string]string
	if err := decoder.Decode(&hello); err != nil {
		log.Printf("agent handshake failed from %s: %v", conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}

	name := strings.TrimSpace(hello["name"])
	if name == "" {
		name = conn.RemoteAddr().String()
	}

	agentID := conn.RemoteAddr().String()
	agent := &Agent{
		Name:    name,
		Conn:    conn,
		Encoder: json.NewEncoder(conn),
		Decoder: decoder,
	}

	agentsMu.Lock()
	agents[agentID] = agent
	agentsMu.Unlock()

	log.Printf("agent connected: %s (%s)", name, agentID)
}

func printAgents(agentsMu *sync.RWMutex, agents map[string]*Agent) {
	agentsMu.RLock()
	defer agentsMu.RUnlock()

	if len(agents) == 0 {
		fmt.Println("no agents connected")
		return
	}

	fmt.Println("connected agents:")
	for id, agent := range agents {
		fmt.Printf("- %s (%s)\n", agent.Name, id)
	}
}

func broadcast(req Request, agentsMu *sync.RWMutex, agents map[string]*Agent) {
	agentsMu.RLock()
	snapshot := make(map[string]*Agent, len(agents))
	for id, agent := range agents {
		snapshot[id] = agent
	}
	agentsMu.RUnlock()

	if len(snapshot) == 0 {
		fmt.Println("no agents connected")
		return
	}

	var wg sync.WaitGroup
	results := make(chan string, len(snapshot))

	for id, agent := range snapshot {
		wg.Add(1)
		go func(agentID string, a *Agent) {
			defer wg.Done()
			results <- sendCommand(a, agentID, req, agentsMu, agents)
		}(id, agent)
	}

	wg.Wait()
	close(results)

	fmt.Println("broadcast results:")
	for result := range results {
		fmt.Println(result)
	}
}

func sendCommand(agent *Agent, agentID string, req Request, agentsMu *sync.RWMutex, agents map[string]*Agent) string {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	if err := agent.Encoder.Encode(req); err != nil {
		removeAgent(agentID, agent, agentsMu, agents)
		return fmt.Sprintf("[%s] send failed: %v", agent.Name, err)
	}

	var resp Response
	if err := agent.Decoder.Decode(&resp); err != nil {
		removeAgent(agentID, agent, agentsMu, agents)
		return fmt.Sprintf("[%s] response failed: %v", agent.Name, err)
	}

	if resp.OK {
		return fmt.Sprintf("[%s] success: %s", agent.Name, resp.Message)
	}

	return fmt.Sprintf("[%s] failed: %s", agent.Name, resp.Message)
}

func removeAgent(agentID string, agent *Agent, agentsMu *sync.RWMutex, agents map[string]*Agent) {
	_ = agent.Conn.Close()

	agentsMu.Lock()
	delete(agents, agentID)
	agentsMu.Unlock()
}

func showMenu() {
	fmt.Println()
	fmt.Println("1. Lock all connected devices")
	fmt.Println("2. Shutdown all connected devices")
	fmt.Println("3. Change wallpaper on all connected devices")
	fmt.Println("4. Show connected devices")
	fmt.Println("5. Exit")
}

func buildWallpaperRequest() (Request, error) {
	imagePath, err := chooseImageFile()
	if err != nil {
		return Request{}, err
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return Request{}, fmt.Errorf("failed to read image: %w", err)
	}

	return Request{
		Command:       "wallpaper",
		Wallpaper:     imagePath,
		WallpaperName: filepath.Base(imagePath),
		WallpaperData: base64.StdEncoding.EncodeToString(data),
	}, nil
}

func chooseImageFile() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return chooseImageFileWindows()
	case "linux":
		return chooseImageFileLinux()
	default:
		return chooseImageFileManual()
	}
}

func chooseImageFileWindows() (string, error) {
	script := `[System.Reflection.Assembly]::LoadWithPartialName("System.Windows.Forms") | Out-Null
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Filter = "Image Files|*.jpg;*.jpeg;*.png;*.bmp"
$dialog.Multiselect = $false
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  Write-Output $dialog.FileName
}`

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to open file picker")
	}

	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return "", fmt.Errorf("no image selected")
	}

	return validateImagePath(selected)
}

func chooseImageFileLinux() (string, error) {
	pickers := []struct {
		name string
		args []string
	}{
		{
			name: "zenity",
			args: []string{
				"--file-selection",
				"--title=Choose wallpaper",
				"--file-filter=Image files | *.jpg *.jpeg *.png *.bmp",
			},
		},
		{
			name: "kdialog",
			args: []string{
				"--getopenfilename",
				".",
				"*.jpg *.jpeg *.png *.bmp | Image files",
			},
		},
	}

	for _, picker := range pickers {
		if _, err := exec.LookPath(picker.name); err != nil {
			continue
		}

		output, err := exec.Command(picker.name, picker.args...).Output()
		if err != nil {
			continue
		}

		selected := strings.TrimSpace(string(output))
		if selected == "" {
			continue
		}

		return validateImagePath(selected)
	}

	return chooseImageFileManual()
}

func chooseImageFileManual() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the full path to the wallpaper image: ")

	selected, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read image path: %w", err)
	}

	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", fmt.Errorf("no image selected")
	}

	return validateImagePath(selected)
}

func validateImagePath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to access image: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("selected path is a directory")
	}
	return path, nil
}