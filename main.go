package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	// server := server.NewServer("192.168.3.79", 8888)
	// server.Start()
	// port = 31539

	var netInfo map[string]string = make(map[string]string)

	ifaces, err := net.Interfaces()

	if err != nil {
		fmt.Println("获取网络接口信息失败", err)
		os.Exit(1)
	}

	for _, iface := range ifaces {	
		// 获取对应网络接口的IP
		addrs, err := iface.Addrs()

		if err != nil {
			fmt.Printf("获取%s接口的IP地址失败\n", iface.Name)
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					netInfo[iface.Name] = ipnet.IP.String()
				}
			}
		}
	}

	for name, ip := range netInfo {
		fmt.Println(name)
		fmt.Println(ip)
	}

}
