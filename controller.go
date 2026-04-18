package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

var agents =  []string{
		"1010",
		"1011",
		"1012",
		"1013",
	}
var url string = "http://localhost:" 



func send(url, cmd string) {
	resp, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte(cmd)))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Response: ", string(body))
}

func oneAgent(c int){

	fmt.Println("========== One Agent ==========")
	fmt.Println("Choose which agent you wanna deal with:")
	for i := 0; i<len(agents);i++{
		fmt.Println(i+1,". ",agents[i])
	}

	fmt.Scan(&c)

	fmt.Println("========== One Agent:["+agents[c-1]+"] ==========")
	fmt.Println("Choose what you wanna do:")
	fmt.Println("1. Lock")
	fmt.Println("2. Shutdown")
	fmt.Println("3. Change Wallpaper")
	fmt.Println("0. Back")

	switch c{
	case 1:
		send(url,"lock")
	case 2:
		send(url,"shutdown")
	case 3:
		send(url,"change wallpaper")

	}

	
		
}

func menu(){
	
	loop:
	for{

		fmt.Println("========== Start Menu ==========")
		fmt.Println("Do you wanna deal with:")
		fmt.Println("1. One agent")
		fmt.Println("2. More than one agent")
		fmt.Println("3. All agents")
		fmt.Println("0. Exit")
		
		var c int 
		fmt.Scan(&c)
		
		switch c{
		case 1:
			oneAgent(c)
		case 2:
			moreAgents()
		case 3:
			allAgents()
		case 0:
			break loop
		}
	
	}

}

func main() {

	
	menu()



	// sendCommand("http://localhost:9000/command", "lock")
	// sendCommand("http://localhost:9000/command", "shutdown")
	// sendCommand("http://localhost:9000/command", "wallpaper")
}