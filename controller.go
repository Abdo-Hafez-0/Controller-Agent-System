package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// CommandResponse matches the agent's response struct
type CommandResponse struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool `json:"success"`
}

// Known agents (port numbers)
var agents = []string{
	"1010",
	"1011",
	"1012",
	"1013",
}

const baseURL = "http://localhost:"

// ─── Core HTTP helpers ────────────────────────────────────────────────────────

func sendCommand(port, endpoint, cmd string) CommandResponse {
	url := baseURL + port + "/" + endpoint
	resp, err := http.Post(url, "text/plain", bytes.NewBufferString(cmd))
	if err != nil {
		return CommandResponse{
			Agent:   "Agent-" + port,
			Command: cmd,
			Output:  "ERROR: " + err.Error(),
			Success: false,
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result CommandResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// Fallback for plain text responses
		result = CommandResponse{
			Agent:   "Agent-" + port,
			Command: cmd,
			Output:  string(body),
			Success: true,
		}
	}
	return result
}

func sendToAll(endpoint, cmd string) {
	var wg sync.WaitGroup
	results := make([]CommandResponse, len(agents))

	for i, port := range agents {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			results[idx] = sendCommand(p, endpoint, cmd)
		}(i, port)
	}

	wg.Wait()
	printResults(results)
}

func sendToMany(ports []string, endpoint, cmd string) {
	var wg sync.WaitGroup
	results := make([]CommandResponse, len(ports))

	for i, port := range ports {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			results[idx] = sendCommand(p, endpoint, cmd)
		}(i, port)
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

func printAgentList() {
	fmt.Println("─────────────────────────────")
	for i, port := range agents {
		fmt.Printf("  %d. Agent-%s (port %s)\n", i+1, port, port)
	}
	fmt.Println("─────────────────────────────")
}


func pickBuiltinCommand(reader *bufio.Reader) (string, bool) {
	fmt.Println("\nChoose command:")
	fmt.Println("  1. Lock")
	fmt.Println("  2. Shutdown")
	fmt.Println("  3. Change Wallpaper")
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
		3: "wallpaper",
		4: "info",
	}

	if c == 5 {
		fmt.Print("Enter shell command: ")
		cmd, _ := reader.ReadString('\n')
		return "TERMINAL:" + strings.TrimSpace(cmd), true
	}

	cmd, ok := commands[c]
	return cmd, ok
}


func oneAgent(reader *bufio.Reader) {
	fmt.Println("\n====== One Agent ======")
	printAgentList()
	fmt.Print("Choose agent number: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(agents) {
		fmt.Println("Invalid choice.")
		return
	}

	port := agents[idx-1]
	fmt.Printf("\nSelected: Agent-%s\n", port)

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	var result CommandResponse
	if strings.HasPrefix(cmd, "TERMINAL:") {
		shellCmd := strings.TrimPrefix(cmd, "TERMINAL:")
		result = sendCommand(port, "terminal", shellCmd)
	} else {
		result = sendCommand(port, "command", cmd)
	}
	printResults([]CommandResponse{result})
}

func moreAgents(reader *bufio.Reader) {
	fmt.Println("\n====== Multiple Agents ======")
	printAgentList()
	fmt.Print("Enter agent numbers separated by commas (e.g. 1,3): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ",")

	var selectedPorts []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(agents) {
			fmt.Printf("  Skipping invalid: %s\n", p)
			continue
		}
		selectedPorts = append(selectedPorts, agents[idx-1])
	}

	if len(selectedPorts) == 0 {
		fmt.Println("No valid agents selected.")
		return
	}

	fmt.Printf("Targeting: %s\n", strings.Join(selectedPorts, ", "))

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	if strings.HasPrefix(cmd, "TERMINAL:") {
		shellCmd := strings.TrimPrefix(cmd, "TERMINAL:")
		sendToMany(selectedPorts, "terminal", shellCmd)
	} else {
		sendToMany(selectedPorts, "command", cmd)
	}
}

func allAgents(reader *bufio.Reader) {
	fmt.Println("\n====== All Agents ======")

	cmd, ok := pickBuiltinCommand(reader)
	if !ok {
		return
	}

	if strings.HasPrefix(cmd, "TERMINAL:") {
		shellCmd := strings.TrimPrefix(cmd, "TERMINAL:")
		sendToAll("terminal", shellCmd)
	} else {
		sendToAll("command", cmd)
	}
}


func terminalSession(reader *bufio.Reader) {
	fmt.Println("\n====== Terminal Session ======")
	printAgentList()
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
			sendToAll("terminal", cmd)
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

	for {
		fmt.Println("\n╔══════════════════════════════╗")
		fmt.Println("║     Distributed Controller   ║")
		fmt.Println("╚══════════════════════════════╝")
		fmt.Println("  1. One agent")
		fmt.Println("  2. Multiple agents")
		fmt.Println("  3. All agents")
		fmt.Println("  4. Terminal session")
		fmt.Println("  0. Exit")
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		c, err := strconv.Atoi(input)
		if err != nil {
			continue
		}

		switch c {
		case 1:
			oneAgent(reader)
		case 2:
			moreAgents(reader)
		case 3:
			allAgents(reader)
		case 4:
			terminalSession(reader)
		case 0:
			fmt.Println("Goodbye.")
			return
		}
	}
}

func main() {
	menu()
}