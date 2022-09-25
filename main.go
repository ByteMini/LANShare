package main

import "LANShare/server"

func main() {
	server := server.NewServer("192.168.3.79", 8888)
	server.Start()
}
