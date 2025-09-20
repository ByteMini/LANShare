package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// P2P节点结构
type P2PNode struct {
	LocalIP   string
	LocalPort int
	Name      string
	ID        string

	Listener   net.Listener
	Peers      map[string]*Peer
	PeersMutex sync.RWMutex

	MessageChan chan Message
	Running     bool

	DiscoveryPort int
	BroadcastConn *net.UDPConn

	// Web GUI相关
	WebPort      int
	Messages     []ChatMessage
	MessagesMutex sync.RWMutex
	WebEnabled   bool
}

type Peer struct {
	ID       string
	Name     string
	Address  string
	Conn     net.Conn
	IsActive bool
	LastSeen time.Time
}

type Message struct {
	Type      string      `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

type DiscoveryMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type ChatMessage struct {
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsOwn     bool      `json:"isOwn"`
	IsPrivate bool      `json:"isPrivate"`
}

// 创建新的P2P节点
func NewP2PNode(name string, webEnabled bool) *P2PNode {
	localIP := getLocalIP()
	nodeID := fmt.Sprintf("%s_%d", localIP, time.Now().Unix())
	
	return &P2PNode{
		LocalIP:       localIP,
		LocalPort:     8888,
		Name:          name,
		ID:            nodeID,
		Peers:         make(map[string]*Peer),
		MessageChan:   make(chan Message, 100),
		Running:       false,
		DiscoveryPort: 9999,
		WebPort:       8080,
		Messages:      make([]ChatMessage, 0),
		WebEnabled:    webEnabled,
	}
}

// 获取本地IP地址
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	var availableIPs []string
	var interfaceNames []string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ip := ipnet.IP.String()
					availableIPs = append(availableIPs, ip)
					interfaceNames = append(interfaceNames, iface.Name)
				}
			}
		}
	}

	if len(availableIPs) == 0 {
		return "127.0.0.1"
	}

	if len(availableIPs) == 1 {
		fmt.Printf("使用网络接口: %s (%s)\n", interfaceNames[0], availableIPs[0])
		return availableIPs[0]
	}

	// 多个网卡时让用户选择
	fmt.Println("检测到的网络接口:")
	for i, ip := range availableIPs {
		fmt.Printf("  %d. %s: %s\n", i+1, interfaceNames[i], ip)
	}

	for {
		fmt.Printf("发现多个网络接口，请选择要使用的接口 (1-%d): ", len(availableIPs))
		var choice int
		_, err := fmt.Scanf("%d", &choice)
		if err != nil || choice < 1 || choice > len(availableIPs) {
			fmt.Println("无效选择，请重新输入")
			continue
		}

		selectedIP := availableIPs[choice-1]
		selectedInterface := interfaceNames[choice-1]
		fmt.Printf("已选择网络接口: %s (%s)\n", selectedInterface, selectedIP)
		return selectedIP
	}
}

// 添加聊天消息
func (node *P2PNode) addChatMessage(sender, content string, isOwn, isPrivate bool) {
	if node.WebEnabled {
		node.MessagesMutex.Lock()
		defer node.MessagesMutex.Unlock()
		
		msg := ChatMessage{
			Sender:    sender,
			Content:   content,
			Timestamp: time.Now(),
			IsOwn:     isOwn,
			IsPrivate: isPrivate,
		}
		
		node.Messages = append(node.Messages, msg)
		
		// 保持最近100条消息
		if len(node.Messages) > 100 {
			node.Messages = node.Messages[1:]
		}
	}

	// 命令行显示
	timestamp := time.Now().Format("15:04:05")
	if isPrivate {
		fmt.Printf("[%s] %s (私聊): %s\n", timestamp, sender, content)
	} else {
		fmt.Printf("[%s] %s: %s\n", timestamp, sender, content)
	}
}

// 启动Web GUI服务器
func (node *P2PNode) startWebGUI() {
	if !node.WebEnabled {
		return
	}

	// 读取HTML模板文件
	tmpl, err := template.ParseFiles("web/index.html")
	if err != nil {
		log.Printf("读取HTML模板失败: %v", err)
		return
	}

	// 主页处理器
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.Execute(w, node); err != nil {
			log.Printf("执行模板失败: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// 静态文件服务器
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		// 移除 /static/ 前缀
		path := strings.TrimPrefix(r.URL.Path, "/static/")
		
		// 安全检查，防止目录遍历攻击
		if strings.Contains(path, "..") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		
		// 根据文件扩展名设置Content-Type
		switch {
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript")
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html")
		}
		
		// 提供文件服务
		http.ServeFile(w, r, "web/"+path)
	})

	// 发送消息处理器
	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Message string `json:"message"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		node.handleWebMessage(req.Message)
		w.WriteHeader(http.StatusOK)
	})

	// 获取消息处理器
	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		node.MessagesMutex.RLock()
		messages := make([]ChatMessage, len(node.Messages))
		copy(messages, node.Messages)
		node.MessagesMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"messages": messages,
		})
	})

	// 获取用户列表处理器
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		users := []string{node.Name + " (自己)"}
		
		node.PeersMutex.RLock()
		for _, peer := range node.Peers {
			if peer.IsActive {
				users = append(users, peer.Name)
			}
		}
		node.PeersMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": users,
		})
	})

	// 启动Web服务器
	go func() {
		webURL := fmt.Sprintf("http://%s:%d", node.LocalIP, node.WebPort)
		fmt.Printf("Web界面已启动: %s\n", webURL)
		
		// 自动打开浏览器
		go func() {
			time.Sleep(2 * time.Second) // 等待服务器启动
			openBrowser(webURL)
		}()
		
		if err := http.ListenAndServe(fmt.Sprintf(":%d", node.WebPort), nil); err != nil {
			log.Printf("Web服务器启动失败: %v", err)
		}
	}()
}

// 打开浏览器
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		fmt.Printf("请手动在浏览器中打开: %s\n", url)
		return
	}
	if err != nil {
		fmt.Printf("无法自动打开浏览器，请手动访问: %s\n", url)
	}
}

// 处理Web消息
func (node *P2PNode) handleWebMessage(text string) {
	if strings.HasPrefix(text, "/") {
		node.handleCommand(text)
	} else {
		// 公聊消息
		msg := Message{
			Type:      "chat",
			From:      node.ID,
			To:        "all",
			Content:   text,
			Timestamp: time.Now(),
		}
		node.broadcastMessage(msg)
		node.addChatMessage("我", text, true, false)
	}
}

// 启动P2P节点
func (node *P2PNode) Start() error {
	// 启动TCP监听器
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", node.LocalIP, node.LocalPort))
	if err != nil {
		return fmt.Errorf("启动TCP监听失败: %v", err)
	}
	node.Listener = listener
	node.Running = true

	fmt.Printf("P2P节点启动成功: %s:%d\n", node.LocalIP, node.LocalPort)
	fmt.Printf("节点ID: %s\n", node.ID)
	fmt.Printf("用户名: %s\n", node.Name)

	// 启动Web GUI
	if node.WebEnabled {
		node.startWebGUI()
	}

	// 启动服务发现
	go node.startDiscovery()

	// 启动消息处理
	go node.handleMessages()

	// 启动连接监听
	go node.acceptConnections()

	// 启动定期广播
	go node.periodicBroadcast()

	return nil
}

// 启动命令行界面
func (node *P2PNode) startCLI() {
	if node.WebEnabled {
		fmt.Println("\n===========================================")
		fmt.Println("           LANShare P2P 客户端")
		fmt.Println("===========================================")
		fmt.Println("命令说明:")
		fmt.Println("  直接输入消息 - 公聊")
		fmt.Println("  /to <用户名> <消息> - 私聊")
		fmt.Println("  /list - 查看在线用户")
		fmt.Println("  /name <新名称> - 更改用户名")
		fmt.Println("  /web - 打开Web界面")
		fmt.Println("  /quit - 退出程序")
		fmt.Println("===========================================")
	} else {
		fmt.Println("\n===========================================")
		fmt.Println("           LANShare P2P 客户端")
		fmt.Println("===========================================")
		fmt.Println("命令说明:")
		fmt.Println("  直接输入消息 - 公聊")
		fmt.Println("  /to <用户名> <消息> - 私聊")
		fmt.Println("  /list - 查看在线用户")
		fmt.Println("  /name <新名称> - 更改用户名")
		fmt.Println("  /quit - 退出程序")
		fmt.Println("===========================================")
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}

		if strings.HasPrefix(text, "/") {
			if text == "/quit" {
				break
			}
			node.handleCommand(text)
		} else {
			// 公聊消息
			msg := Message{
				Type:      "chat",
				From:      node.ID,
				To:        "all",
				Content:   text,
				Timestamp: time.Now(),
			}
			node.broadcastMessage(msg)
			node.addChatMessage("我", text, true, false)
		}
	}

	node.Stop()
}

// 处理命令
func (node *P2PNode) handleCommand(command string) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/to":
		if len(parts) < 3 {
			fmt.Println("用法: /to <用户名> <消息>")
			return
		}
		
		targetName := parts[1]
		message := strings.Join(parts[2:], " ")
		
		// 查找目标用户
		var targetID string
		node.PeersMutex.RLock()
		for id, peer := range node.Peers {
			if peer.Name == targetName {
				targetID = id
				break
			}
		}
		node.PeersMutex.RUnlock()
		
		if targetID == "" {
			fmt.Printf("用户 %s 不在线\n", targetName)
			return
		}
		
		msg := Message{
			Type:      "chat",
			From:      node.ID,
			To:        targetID,
			Content:   message,
			Timestamp: time.Now(),
		}
		
		if peer, exists := node.Peers[targetID]; exists {
			node.sendMessageToPeer(peer, msg)
			node.addChatMessage("我 -> "+targetName, message, true, true)
		}
		
	case "/list":
		fmt.Println("在线用户:")
		fmt.Printf("  %s (自己)\n", node.Name)
		
		node.PeersMutex.RLock()
		for _, peer := range node.Peers {
			if peer.IsActive {
				fmt.Printf("  %s (%s)\n", peer.Name, peer.Address)
			}
		}
		node.PeersMutex.RUnlock()
		
	case "/name":
		if len(parts) < 2 {
			fmt.Println("用法: /name <新名称>")
			return
		}
		oldName := node.Name
		node.Name = parts[1]
		fmt.Printf("用户名已从 %s 更改为 %s\n", oldName, node.Name)
		
	case "/web":
		if node.WebEnabled {
			webURL := fmt.Sprintf("http://%s:%d", node.LocalIP, node.WebPort)
			fmt.Printf("Web界面地址: %s\n", webURL)
			openBrowser(webURL)
		} else {
			fmt.Println("Web界面未启用")
		}
		
	default:
		fmt.Printf("未知命令: %s\n", parts[0])
	}
}

// 以下方法与之前的实现相同...

// 启动服务发现
func (node *P2PNode) startDiscovery() {
	go node.listenBroadcast()
	time.Sleep(1 * time.Second)
	node.sendDiscoveryBroadcast("announce")
}

// 监听UDP广播
func (node *P2PNode) listenBroadcast() {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", node.DiscoveryPort))
	if err != nil {
		fmt.Printf("解析UDP地址失败: %v\n", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("监听UDP失败: %v\n", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for node.Running {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		var discoveryMsg DiscoveryMessage
		if err := json.Unmarshal(buffer[:n], &discoveryMsg); err != nil {
			continue
		}

		if discoveryMsg.ID == node.ID {
			continue
		}

		node.handleDiscoveryMessage(discoveryMsg, remoteAddr)
	}
}

// 处理服务发现消息
func (node *P2PNode) handleDiscoveryMessage(msg DiscoveryMessage, remoteAddr *net.UDPAddr) {
	switch msg.Type {
	case "announce":
		fmt.Printf("发现新节点: %s (%s:%d)\n", msg.Name, msg.IP, msg.Port)
		go node.connectToPeer(msg.IP, msg.Port, msg.ID, msg.Name)
		node.sendDiscoveryResponse(remoteAddr.IP.String())
		
	case "response":
		go node.connectToPeer(msg.IP, msg.Port, msg.ID, msg.Name)
	}
}

// 发送服务发现广播
func (node *P2PNode) sendDiscoveryBroadcast(msgType string) {
	msg := DiscoveryMessage{
		Type: msgType,
		ID:   node.ID,
		Name: node.Name,
		IP:   node.LocalIP,
		Port: node.LocalPort,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	broadcastAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", node.DiscoveryPort))
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, broadcastAddr)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.Write(data)
}

// 发送服务发现响应
func (node *P2PNode) sendDiscoveryResponse(targetIP string) {
	msg := DiscoveryMessage{
		Type: "response",
		ID:   node.ID,
		Name: node.Name,
		IP:   node.LocalIP,
		Port: node.LocalPort,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, node.DiscoveryPort))
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.Write(data)
}

// 连接到对等节点
func (node *P2PNode) connectToPeer(ip string, port int, id, name string) {
	node.PeersMutex.RLock()
	if _, exists := node.Peers[id]; exists {
		node.PeersMutex.RUnlock()
		return
	}
	node.PeersMutex.RUnlock()

	if id == node.ID {
		return
	}

	address := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return
	}

	peer := &Peer{
		ID:       id,
		Name:     name,
		Address:  address,
		Conn:     conn,
		IsActive: true,
		LastSeen: time.Now(),
	}

	node.PeersMutex.Lock()
	node.Peers[id] = peer
	node.PeersMutex.Unlock()

	fmt.Printf("成功连接到节点: %s (%s)\n", name, address)

	handshakeMsg := Message{
		Type:      "handshake",
		From:      node.ID,
		Content:   node.Name,
		Timestamp: time.Now(),
	}
	node.sendMessageToPeer(peer, handshakeMsg)

	go node.handlePeerConnection(peer)
}

// 接受连接
func (node *P2PNode) acceptConnections() {
	for node.Running {
		conn, err := node.Listener.Accept()
		if err != nil {
			if node.Running {
				fmt.Printf("接受连接失败: %v\n", err)
			}
			continue
		}

		go node.handleIncomingConnection(conn)
	}
}

// 处理传入连接
func (node *P2PNode) handleIncomingConnection(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	var handshakeMsg Message
	
	if err := decoder.Decode(&handshakeMsg); err != nil {
		conn.Close()
		return
	}

	if handshakeMsg.Type != "handshake" {
		conn.Close()
		return
	}

	peer := &Peer{
		ID:       handshakeMsg.From,
		Name:     handshakeMsg.Content,
		Address:  conn.RemoteAddr().String(),
		Conn:     conn,
		IsActive: true,
		LastSeen: time.Now(),
	}

	node.PeersMutex.Lock()
	node.Peers[peer.ID] = peer
	node.PeersMutex.Unlock()

	fmt.Printf("接受来自节点的连接: %s (%s)\n", peer.Name, peer.Address)

	go node.handlePeerConnection(peer)
}

// 处理对等节点连接
func (node *P2PNode) handlePeerConnection(peer *Peer) {
	decoder := json.NewDecoder(peer.Conn)
	
	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				fmt.Printf("从节点 %s 读取消息失败: %v\n", peer.Name, err)
			}
			break
		}

		peer.LastSeen = time.Now()
		node.MessageChan <- msg
	}

	node.PeersMutex.Lock()
	delete(node.Peers, peer.ID)
	node.PeersMutex.Unlock()
	
	peer.Conn.Close()
	fmt.Printf("节点 %s 断开连接\n", peer.Name)
}

// 处理消息
func (node *P2PNode) handleMessages() {
	for msg := range node.MessageChan {
		switch msg.Type {
		case "chat":
			if msg.To == "" || msg.To == "all" {
				// 公聊消息
				node.addChatMessage(node.getPeerName(msg.From), msg.Content, false, false)
			} else if msg.To == node.ID {
				// 私聊消息
				node.addChatMessage(node.getPeerName(msg.From), msg.Content, false, true)
			}
		case "handshake":
			// 握手消息已在连接处理中处理
		}
	}
}

// 获取对等节点名称
func (node *P2PNode) getPeerName(peerID string) string {
	node.PeersMutex.RLock()
	defer node.PeersMutex.RUnlock()
	
	if peer, exists := node.Peers[peerID]; exists {
		return peer.Name
	}
	return peerID
}

// 发送消息到对等节点
func (node *P2PNode) sendMessageToPeer(peer *Peer, msg Message) error {
	encoder := json.NewEncoder(peer.Conn)
	return encoder.Encode(msg)
}

// 广播消息到所有对等节点
func (node *P2PNode) broadcastMessage(msg Message) {
	node.PeersMutex.RLock()
	defer node.PeersMutex.RUnlock()

	for _, peer := range node.Peers {
		if peer.IsActive {
			go node.sendMessageToPeer(peer, msg)
		}
	}
}

// 定期广播
func (node *P2PNode) periodicBroadcast() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if node.Running {
				node.sendDiscoveryBroadcast("announce")
			}
		}
	}
}

// 停止节点
func (node *P2PNode) Stop() {
	node.Running = false
	
	if node.Listener != nil {
		node.Listener.Close()
	}
	
	if node.BroadcastConn != nil {
		node.BroadcastConn.Close()
	}
	
	node.PeersMutex.Lock()
	for _, peer := range node.Peers {
		peer.Conn.Close()
	}
	node.PeersMutex.Unlock()
	
	close(node.MessageChan)
	fmt.Println("P2P节点已停止")
}

func main() {
	var name string
	var webMode bool
	var cliMode bool
	var showHelp bool
	
	flag.StringVar(&name, "name", "", "指定用户名")
	flag.BoolVar(&webMode, "web", false, "启用Web界面模式")
	flag.BoolVar(&cliMode, "cli", false, "仅使用命令行模式")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.Parse()

	// 显示帮助信息
	if showHelp {
		fmt.Println("LANShare P2P - 局域网即时通信工具")
		fmt.Println()
		fmt.Println("用法:")
		fmt.Printf("  %s [选项] [用户名]\n", os.Args[0])
		fmt.Println()
		fmt.Println("选项:")
		fmt.Println("  -name string    指定用户名")
		fmt.Println("  -web            启用Web界面模式")
		fmt.Println("  -cli            仅使用命令行模式")
		fmt.Println("  -help           显示此帮助信息")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Printf("  %s                    # 交互式选择模式\n", os.Args[0])
		fmt.Printf("  %s -web               # Web模式\n", os.Args[0])
		fmt.Printf("  %s -cli               # 命令行模式\n", os.Args[0])
		fmt.Printf("  %s -name 张三         # 指定用户名\n", os.Args[0])
		fmt.Printf("  %s -web -name 李四    # Web模式，指定用户名\n", os.Args[0])
		fmt.Println()
		fmt.Println("网络端口:")
		fmt.Println("  P2P通信: 8888 (TCP)")
		fmt.Println("  服务发现: 9999 (UDP)")
		fmt.Println("  Web界面: 8080 (HTTP，仅Web模式)")
		return
	}

	fmt.Println("===========================================")
	fmt.Println("           LANShare P2P 启动器")
	fmt.Println("===========================================")

	// 如果没有指定模式，让用户选择
	if !webMode && !cliMode {
		fmt.Println("请选择运行模式:")
		fmt.Println("  1. Web界面模式 (推荐)")
		fmt.Println("  2. 命令行模式")
		fmt.Print("请输入选择 (1-2，默认1): ")
		
		var choice string
		fmt.Scanln(&choice)
		
		switch choice {
		case "2":
			cliMode = true
		case "1", "":
			webMode = true
		default:
			fmt.Println("无效选择，使用默认Web模式")
			webMode = true
		}
	}

	// 默认启用Web模式
	if !cliMode {
		webMode = true
	}

	// 获取用户名
	if name == "" {
		if len(flag.Args()) > 0 {
			name = flag.Args()[0]
		} else {
			fmt.Print("请输入您的用户名 (留空使用默认): ")
			var inputName string
			fmt.Scanln(&inputName)
			if inputName != "" {
				name = inputName
			} else {
				name = "用户_" + strconv.Itoa(int(time.Now().Unix()%10000))
			}
		}
	}
	
	if webMode {
		fmt.Printf("启动 P2P 客户端 (Web模式)，用户名: %s\n", name)
		fmt.Println("程序启动后将自动打开浏览器")
	} else {
		fmt.Printf("启动 P2P 客户端 (命令行模式)，用户名: %s\n", name)
	}
	fmt.Println("===========================================")

	node := NewP2PNode(name, webMode)
	
	if err := node.Start(); err != nil {
		fmt.Printf("启动P2P节点失败: %v\n", err)
		return
	}

	// 启动命令行界面
	node.startCLI()
}
