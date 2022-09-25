package server

import (
	"LANShare/client"
	"fmt"
	"io"
	"net"
	"sync"
)

// 服务器模型
type Server struct {
	Ip   string
	Port int
	// 在线用户列表
	OnlineMap map[string]*client.User
	mapLock   sync.RWMutex

	// 消息广播的channel
	Message chan string
}

// 创建新的服务器实例
func NewServer(ip string, port int) *Server {
	return &Server{
		Ip:        ip,
		Port:      port,
		OnlineMap: make(map[string]*client.User),
		Message:   make(chan string),
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

	// 启动广播消息监听
	go s.ListenMessage()

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
	// 为新的连接创建一个新的用户实例
	user := client.NewUser(conn)

	// 将新的用户写入用户列表中
	s.mapLock.Lock()
	s.OnlineMap[user.Name] = user
	s.mapLock.Unlock()

	// 用户上线消息写入广播消息管道
	go s.BroadCast(user, "已上线")

	// 读取客户端发送的消息
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n == 0 {
				s.BroadCast(user, "下线")
				return
			}

			if err != nil && err != io.EOF {
				fmt.Println("conn read err: ", err)
				return
			}

			// 读取用户消息内容，去除'\n'
			msg := string(buf[:n-1])
			s.BroadCast(user, msg)
		}
	}()

	// 阻塞当前handler
	select {}

}

// 向广播消息管道写入内容
func (s *Server) BroadCast(user *client.User, msg string) {
	sendMsg := "[" + user.Addr + "]" + user.Name + ":" + msg

	s.Message <- sendMsg
}

// 监听广播消息
func (s *Server) ListenMessage() {
	for {
		// 取出广播消息管道中的内容
		msg := <-s.Message

		// 将消息发给所有在线用户
		s.mapLock.Lock()
		for _, cli := range s.OnlineMap {
			cli.C <- msg
		}
		s.mapLock.Unlock()
	}
}
