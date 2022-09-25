package server

import (
	"fmt"
	"net"
)

// 服务器模型
type Server struct {
	Ip   string
	Port int
}

// 创建新的服务器实例
func NewServer(ip string, port int) *Server {
	return &Server{
		Ip:   ip,
		Port: port,
	}
}

// 启动服务器实例
func (s *Server) Start() {
	// 开启socket
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.Ip, s.Port))
	if err != nil {
		fmt.Println("socket listen err: ", err)
		return
	}

	// 关闭socket
	defer listener.Close()

	// 监听连接
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("socket accept err: ", err)
			continue
		}

		// 处理连接
		go s.Handler(conn)
	}
}

// 处理服务器连接
func (s *Server) Handler(conn net.Conn) {
	fmt.Println("连接成功")
}
