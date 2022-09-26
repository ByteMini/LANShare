package server

import (
	"net"
	"strings"
)

// 用户模型
type User struct {
	Name   string
	Addr   string
	C      chan string
	conn   net.Conn
	server *Server
}

// 创建一个新的用户实例
func NewUser(conn net.Conn, server *Server) *User {
	userAddr := conn.RemoteAddr().String()

	user := &User{
		Name:   userAddr,
		Addr:   userAddr,
		C:      make(chan string),
		conn:   conn,
		server: server,
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

// 处理用户下线
func (u *User) Online() {
	// 处理用户上线
	u.server.MapLock.Lock()
	u.server.OnlineMap[u.Name] = u
	u.server.MapLock.Unlock()

	// 广播用户上线
	u.server.BroadCast(u, "已上线")
}

// 处理用户下线
func (u *User) Offline() {
	// 处理用户下线
	u.server.MapLock.Lock()
	delete(u.server.OnlineMap, u.Name)
	u.server.MapLock.Unlock()

	// 广播用户下线消息
	u.server.BroadCast(u, "已下线")
}

// 处理用户的消息
func (u *User) DoMessage(msg string) {
	if msg == "who" {
		u.server.MapLock.Lock()
		for _, user := range u.server.OnlineMap {
			onlineMsg := "[" + user.Addr + "]" + user.Name + ":" + "在线\n"
			u.ReceiveMsg(onlineMsg)
		}
		u.server.MapLock.Unlock()
	} else if len(msg) > 7 && msg[:7] == "rename|" {
		// 要更新的名字
		newName := strings.Split(msg, "rename|")[1]

		// 判断用户名是否已经被使用
		_, ok := u.server.OnlineMap[newName]
		if ok {
			u.ReceiveMsg("当前用户名已经被使用\n")
		} else {
			u.server.MapLock.Lock()
			delete(u.server.OnlineMap, u.Name)
			u.server.OnlineMap[newName] = u
			u.server.MapLock.Unlock()

			u.Name = newName
			u.ReceiveMsg("你已更新用户名: " + u.Name + "\n")
		}
	} else if len(msg) > 4 && msg[:3] == "to|" {
		// 获取对方的用户名
		remoteName := strings.Split(msg, "|")[1]
		if remoteName == "" {
			u.ReceiveMsg("消息格式不正确，请使用\"to|zs|nihao\"的消息格式。\n")
			return
		}

		// 根据用户名得到user对象
		remoteUser, ok := u.server.OnlineMap[remoteName]
		if !ok {
			u.ReceiveMsg("该用户名不存在\n")
			return
		}

		// 获取要发送的消息内容
		content := strings.Split(msg, "|")[2]
		if content == "" {
			u.ReceiveMsg("消息内容为空\n")
			return
		}

		remoteUser.ReceiveMsg(u.Name + "对你说: " + content + "\n")
	} else {
		u.server.BroadCast(u, msg)
	}

}

// 给当前用户发达消息
func (u *User) ReceiveMsg(msg string) {
	u.conn.Write([]byte(msg))
}
