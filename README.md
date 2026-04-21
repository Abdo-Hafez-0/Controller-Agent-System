# Controller-Agent System

This repository contains a simple Go-based remote command system with two programs:

- `controller.go`: a server that accepts connections from agents and sends commands.
- `agent.go`: a client that connects to the controller and executes received commands.

## Overview

The system works as a basic TCP controller-agent architecture:

1. The controller listens on a TCP port for incoming agent connections.
2. Each agent connects to the controller and sends a handshake containing its device name.
3. The controller tracks connected agents in memory.
4. The controller can broadcast commands to all connected agents.
5. Agents receive commands, execute them locally, and reply with success or failure.

## controller.go

The controller is an interactive command-line program.

Key behaviors:

- Listens on a configurable port (`-port`, default `9000`).
- Accepts TCP connections from agents.
- Reads a JSON handshake from each agent containing its name.
- Stores connected agents in a map keyed by remote address.
- Presents a menu for the operator to:
  - lock all connected devices
  - shutdown all connected devices
  - change wallpaper on all connected devices
  - show connected devices
  - exit
- Broadcasts JSON command objects to all agents and prints results.

The controller uses these structures:

- `Request`: carries a `command` plus optional wallpaper fields.
- `Response`: returns `ok` and `message` from the agent.
- `Agent`: wraps each connection with JSON encoder/decoder and a mutex.

Wallpaper handling on the controller side:

- Prompts the user to choose an image file using a GUI file picker on Windows (`PowerShell`) or Linux (`zenity`, `kdialog`), if available.
- Falls back to a manual path prompt if no picker is available.
- Reads the image file and sends it as Base64-encoded text in the request.

## agent.go

The agent is a client program that connects to a controller address.

Key behaviors:

- Takes a `-controller` flag for the controller address (default `10.251.91.224:9000`).
- Takes an optional `-name` flag for the device name.
- If no name is provided, it uses the host machine name or `unknown-device`.
- Connects to the controller and sends the handshake JSON.
- Waits for commands from the controller and executes them.
- Sends a JSON response back after executing each command.

Supported commands on the agent:

- `lock`
  - Windows: calls `rundll32.exe user32.dll,LockWorkStation`
  - Linux: calls `loginctl lock-session`

- `shutdown`
  - Windows: calls `shutdown /s /t 0`
  - Linux: calls `shutdown now`
  - macOS: calls `sudo shutdown -h now`

- `wallpaper`
  - Decodes Base64 image bytes received in `WallpaperData`.
  - Saves the image to a temporary file.
  - Sets the desktop wallpaper based on OS support.
  - Windows uses PowerShell and `SystemParametersInfo`.
  - Linux tries several desktop environment tools (`gsettings`, `xfconf-query`, `feh`).

## How to build

From the project directory, run:

```bash
go build -o controller controller.go
go build -o agent agent.go
```

## How to run

1. Start the controller:

```bash
go run controller.go
```

2. Start one or more agents:

```bash
go run agent.go -controller 127.0.0.1:9000 -name agent1
```

3. Use the controller menu to send commands to all connected agents.

## Notes

- The controller broadcasts all commands to every connected agent.
- Agents reconnect automatically if the TCP connection is lost.
- Wallpaper command uses Base64 encoding to transfer image data.
- The code currently has basic error handling and no authentication.
- The controller stores agent connections in memory only for the lifetime of the process.
