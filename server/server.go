package server

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// 服务器模型
type Server struct {
	Ip   string
	Port int
	// 在线用户列表
	OnlineMap map[string]*User
	MapLock   sync.RWMutex

	// 消息广播的channel
	Message chan string
}

// 创建新的服务器实例
func NewServer(ip string, port int) *Server {
	return &Server{
		Ip:        ip,
		Port:      port,
		OnlineMap: make(map[string]*User),
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
	user := NewUser(conn, s)

	// 用户上线
	user.Online()

	// 监听用户是否活跃的channel
	isLive := make(chan bool)

	// 读取客户端发送的消息
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n == 0 {
				user.Offline()
				return
			}

			if err != nil && err != io.EOF {
				fmt.Println("conn read err: ", err)
				return
			}

			// 读取用户消息内容，去除'\n'
			msg := string(buf[:n-1])
			user.DoMessage(msg)

			// 用户的任意消息，代表用户活跃
			isLive <- true
		}
	}()

	// 阻塞当前handler
	for {
		select {
		case <-isLive:

		case <-time.After(300 * time.Second):
			user.ReceiveMsg("你被踢了")

			// 销毁资源
			close(user.C)

			// 关闭连接
			user.conn.Close()

			// 返回
			return
		}
	}
}

// 向广播消息管道写入内容
func (s *Server) BroadCast(user *User, msg string) {
	sendMsg := "[" + user.Addr + "]" + user.Name + ":" + msg

	s.Message <- sendMsg
}

// 监听广播消息
func (s *Server) ListenMessage() {
	for {
		// 取出广播消息管道中的内容
		msg := <-s.Message

		// 将消息发给所有在线用户
		s.MapLock.Lock()
		for _, cli := range s.OnlineMap {
			cli.C <- msg
		}
		s.MapLock.Unlock()
	}
}
