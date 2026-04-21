package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// CommandResponse matches the agent's response struct
type CommandResponse struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

// Agent holds a discovered agent's address info
type Agent struct {
	ID   string // e.g. "Agent-1010"
	Host string // IP address
	Port int
}

func (a Agent) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", a.Host, a.Port)
}

const mdnsService = "_distcontrol._tcp"
const mdnsDomain = "local."

// ─── mDNS Discovery ───────────────────────────────────────────────────────────

// discoverAgents browses the local network for agents advertising via mDNS.
// It waits up to `timeout` for responses, then returns all found agents.
func discoverAgents(timeout time.Duration) []Agent {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		fmt.Println("ERROR: failed to create mDNS resolver:", err)
		return nil
	}

	entries := make(chan *zeroconf.ServiceEntry)
	var agents []Agent
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		for entry := range entries {
			if len(entry.AddrIPv4) == 0 {
				continue
			}
			mu.Lock()
			agents = append(agents, Agent{
				ID:   entry.Instance,
				Host: entry.AddrIPv4[0].String(),
				Port: entry.Port,
			})
			mu.Unlock()
		}
	}()

	if err := resolver.Browse(ctx, mdnsService, mdnsDomain, entries); err != nil {
		fmt.Println("ERROR: mDNS browse failed:", err)
		return nil
	}

	<-ctx.Done()
	return agents
}

// refreshAgents discovers agents and prints the result.
func refreshAgents(timeout time.Duration) []Agent {
	fmt.Printf("Scanning network for agents (%.0fs)...\n", timeout.Seconds())
	agents := discoverAgents(timeout)
	if len(agents) == 0 {
		fmt.Println("No agents found.")
	} else {
		fmt.Printf("Found %d agent(s).\n", len(agents))
	}
	return agents
}

func sendCommand(agent Agent, endpoint, cmd string) CommandResponse {
	url := agent.BaseURL() + "/" + endpoint
	resp, err := http.Post(url, "text/plain", bytes.NewBufferString(cmd))
	if err != nil {
		return CommandResponse{
			Agent:   agent.ID,
			Command: cmd,
			Output:  "ERROR: " + err.Error(),
			Success: false,
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result CommandResponse
	if err := json.Unmarshal(body, &result); err != nil {
		result = CommandResponse{
			Agent:   agent.ID,
			Command: cmd,
			Output:  string(body),
			Success: true,
		}
	}
	return result
}

func sendToAll(agents []Agent, endpoint, cmd string) {
	var wg sync.WaitGroup
	results := make([]CommandResponse, len(agents))

	for i, agent := range agents {
		wg.Add(1)
		go func(idx int, a Agent) {
			defer wg.Done()
			results[idx] = sendCommand(a, endpoint, cmd)
		}(i, agent)
	}

	wg.Wait()
	printResults(results)
}

func sendToMany(targets []Agent, endpoint, cmd string) {
	var wg sync.WaitGroup
	results := make([]CommandResponse, len(targets))

	for i, agent := range targets {
		wg.Add(1)
		go func(idx int, a Agent) {
			defer wg.Done()
			results[idx] = sendCommand(a, endpoint, cmd)
		}(i, agent)
	}

	wg.Wait()
	printResults(results)
}

// sendWallpaperFile reads an image from disk and sends raw bytes to /wallpaper
// on each target agent in parallel.
func sendWallpaperFile(targets []Agent, imagePath string) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		fmt.Println("ERROR reading file:", err)
		return
	}

	var wg sync.WaitGroup
	results := make([]CommandResponse, len(targets))

	for i, agent := range targets {
		wg.Add(1)
		go func(idx int, a Agent) {
			defer wg.Done()
			url := a.BaseURL() + "/wallpaper"
			resp, err := http.Post(url, "application/octet-stream", bytes.NewReader(data))
			if err != nil {
				results[idx] = CommandResponse{
					Agent:   a.ID,
					Command: "wallpaper",
					Output:  "ERROR: " + err.Error(),
					Success: false,
				}
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var r CommandResponse
			if err := json.Unmarshal(body, &r); err != nil {
				r = CommandResponse{
					Agent:   a.ID,
					Command: "wallpaper",
					Output:  string(body),
					Success: true,
				}
			}
			results[idx] = r
		}(i, agent)
	}

	wg.Wait()
	printResults(results)
}

func printResults(results []CommandResponse) {
	fmt.Println()
	for _, r := range results {
		status := "✓"
		if !r.Success {
			status = "✗"
		}
		fmt.Printf("[%s] %s %s\n%s\n%s\n",
			r.Agent, status, r.Command,
			strings.Repeat("-", 40),
			r.Output,
		)
	}
}

func printAgentList(agents []Agent) {
	fmt.Println("─────────────────────────────────────────────")
	if len(agents) == 0 {
		fmt.Println("  (no agents — press R at main menu to rescan)")
	}
	for i, a := range agents {
		fmt.Printf("  %d. %s  [%s:%d]\n", i+1, a.ID, a.Host, a.Port)
	}
	fmt.Println("─────────────────────────────────────────────")
}

// ─── Command picker ───────────────────────────────────────────────────────────

func pickBuiltinCommand(reader *bufio.Reader) (string, bool) {
	fmt.Println("\nChoose command:")
	fmt.Println("  1. Lock")
	fmt.Println("  2. Shutdown")
	fmt.Println("  3. Change Wallpaper (send image from this device)")
	fmt.Println("  4. Get System Info")
	fmt.Println("  5. Terminal (custom shell command)")
	fmt.Println("  0. Back")
	fmt.Print("> ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	c, err := strconv.Atoi(input)
	if err != nil || c == 0 {
		return "", false
	}

	commands := map[int]string{
		1: "lock",
		2: "shutdown",
		4: "info",
	}

	switch c {
	case 3:
		fmt.Print("Enter image path on your device: ")
		path, _ := reader.ReadString('\n')
		return "WALLPAPER:" + strings.TrimSpace(path), true
	case 5:
		fmt.Print("Enter shell command: ")
		cmd, _ := reader.ReadString('\n')
		return "TERMINAL:" + strings.TrimSpace(cmd), true
	}

	cmd, ok := commands[c]
	return cmd, ok
}

// ─── Target selectors ─────────────────────────────────────────────────────────

func oneAgent(reader *bufio.Reader, agents []Agent) {
	fmt.Println("\n====== One Agent ======")
	printAgentList(agents)
	if len(agents) == 0 {
		return
	}
	fmt.Print("Choose agent number: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(agents) {
		fmt.Println("Invalid choice.")
		return
	}

	target := agents[idx-1]
	fmt.Printf("\nSelected: %s (%s:%d)\n", target.ID, target.Host, target.Port)

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	if strings.HasPrefix(cmd, "WALLPAPER:") {
		sendWallpaperFile([]Agent{target}, strings.TrimPrefix(cmd, "WALLPAPER:"))
		return
	}

	var result CommandResponse
	if strings.HasPrefix(cmd, "TERMINAL:") {
		result = sendCommand(target, "terminal", strings.TrimPrefix(cmd, "TERMINAL:"))
	} else {
		result = sendCommand(target, "command", cmd)
	}
	printResults([]CommandResponse{result})
}

func moreAgents(reader *bufio.Reader, agents []Agent) {
	fmt.Println("\n====== Multiple Agents ======")
	printAgentList(agents)
	if len(agents) == 0 {
		return
	}
	fmt.Print("Enter agent numbers separated by commas (e.g. 1,3): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ",")

	var selected []Agent
	for _, p := range parts {
		p = strings.TrimSpace(p)
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(agents) {
			fmt.Printf("  Skipping invalid: %s\n", p)
			continue
		}
		selected = append(selected, agents[idx-1])
	}

	if len(selected) == 0 {
		fmt.Println("No valid agents selected.")
		return
	}

	names := make([]string, len(selected))
	for i, a := range selected {
		names[i] = a.ID
	}
	fmt.Printf("Targeting: %s\n", strings.Join(names, ", "))

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	if strings.HasPrefix(cmd, "WALLPAPER:") {
		sendWallpaperFile(selected, strings.TrimPrefix(cmd, "WALLPAPER:"))
		return
	}
	if strings.HasPrefix(cmd, "TERMINAL:") {
		sendToMany(selected, "terminal", strings.TrimPrefix(cmd, "TERMINAL:"))
	} else {
		sendToMany(selected, "command", cmd)
	}
}

func allAgents(reader *bufio.Reader, agents []Agent) {
	fmt.Println("\n====== All Agents ======")
	if len(agents) == 0 {
		fmt.Println("No agents available.")
		return
	}

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	if strings.HasPrefix(cmd, "WALLPAPER:") {
		sendWallpaperFile(agents, strings.TrimPrefix(cmd, "WALLPAPER:"))
		return
	}
	if strings.HasPrefix(cmd, "TERMINAL:") {
		sendToAll(agents, "terminal", strings.TrimPrefix(cmd, "TERMINAL:"))
	} else {
		sendToAll(agents, "command", cmd)
	}
}

func terminalSession(reader *bufio.Reader, agents []Agent) {
	fmt.Println("\n====== Terminal Session ======")
	printAgentList(agents)
	if len(agents) == 0 {
		return
	}
	fmt.Println("  0. All agents")
	fmt.Print("Choose agent (0 for all): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	fmt.Println("Type shell commands. Type 'exit' to leave terminal mode.")
	fmt.Println("─────────────────────────────────────────")

	for {
		fmt.Print("$ ")
		cmd, _ := reader.ReadString('\n')
		cmd = strings.TrimSpace(cmd)
		if cmd == "exit" || cmd == "quit" {
			break
		}
		if cmd == "" {
			continue
		}

		if input == "0" {
			sendToAll(agents, "terminal", cmd)
		} else {
			idx, err := strconv.Atoi(input)
			if err != nil || idx < 1 || idx > len(agents) {
				fmt.Println("Invalid agent.")
				break
			}
			result := sendCommand(agents[idx-1], "terminal", cmd)
			printResults([]CommandResponse{result})
		}
	}
}

// ─── Main menu ────────────────────────────────────────────────────────────────

func menu() {
	reader := bufio.NewReader(os.Stdin)

	// Initial discovery on startup
	agents := refreshAgents(3 * time.Second)

	for {
		fmt.Println("\n╔══════════════════════════════════╗")
		fmt.Println("║     Distributed Controller       ║")
		fmt.Printf("║     Agents found: %-14d║\n", len(agents))
		fmt.Println("╚══════════════════════════════════╝")
		fmt.Println("  1. One agent")
		fmt.Println("  2. Multiple agents")
		fmt.Println("  3. All agents")
		fmt.Println("  4. Terminal session")
		fmt.Println("  R. Rescan network")
		fmt.Println("  0. Exit")
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "1":
			oneAgent(reader, agents)
		case "2":
			moreAgents(reader, agents)
		case "3":
			allAgents(reader, agents)
		case "4":
			terminalSession(reader, agents)
		case "r":
			agents = refreshAgents(3 * time.Second)
		case "0":
			fmt.Println("Goodbye.")
			return
		}
	}
}

func main() {
	menu()
}