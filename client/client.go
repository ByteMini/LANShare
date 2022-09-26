package main

import (
	"flag"
	"fmt"
	"net"
)

type Client struct {
	ServerIp   string
	ServerPort int
	Name       string
	Conn       net.Conn
}

// 创建一个新的客户端实例
func NewClient(serverIp string, serverPort int) *Client {
	// 创建客户端对象
	client := &Client{
		ServerIp:   serverIp,
		ServerPort: serverPort,
	}

	// 连接server
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:%d", serverIp, serverPort))
	if err != nil {
		fmt.Println("net dial err: ", err)
		return nil
	}

	client.Conn = conn

	// 返回对象实例
	return client
}

var (
	serverIp   string
	serverPort int
)

func init() {
	flag.StringVar(&serverIp, "ip", "127.0.0.1", "设置服务器IP,默认是127.0.0.1")
	flag.IntVar(&serverPort, "port", 8888, "设置服务器端口,默认是8888")
}

func main() {
	// 解析参数
	flag.Parse()

	client := NewClient(serverIp, serverPort)

	if client == nil {
		fmt.Println("服务器连接失败\n")
		return
	}

	fmt.Println("服务器连接成功\n")

	// 客户端业务
	select {}
}
