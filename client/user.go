package client

import "net"

// 用户模型
type User struct {
	Name string
	Addr string
	C    chan string
	conn net.Conn
}

// 创建一个新的用户实例
func NewUser(conn net.Conn) *User {
	userAddr := conn.RemoteAddr().String()

	user := &User{
		Name: userAddr,
		Addr: userAddr,
		C:    make(chan string),
		conn: conn,
	}

	// 启动一个goroutine监听channel的消息
	go user.ListenMessage()

	return user
}

// 监听channel的消息
func (u *User) ListenMessage() {
	for {
		msg := <-u.C

		u.conn.Write([]byte(msg + "\n"))
	}
}
