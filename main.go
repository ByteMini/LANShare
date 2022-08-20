package main

import "LANShare/scan"

func main() {
	device := "\\Device\\NPF_{3B81C1F8-E4DD-4C8A-BAF7-BC7DD760D800}"
	scan.GetPacket(device)
}
