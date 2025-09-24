package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

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
		FileTransfers: make(map[string]*FileTransferStatus),
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

// 显示命令帮助信息
func (node *P2PNode) showCommandHelp() {
	fmt.Println("\n===========================================")
	fmt.Println("           LANShare P2P 客户端 - 帮助")
	fmt.Println("===========================================")
	fmt.Println("命令说明:")
	fmt.Println("  直接输入消息 - 公聊")
	fmt.Println("  /to <用户名> <消息> - 私聊")
	fmt.Println("  /send <用户名> <文件路径> - 发送文件")
	fmt.Println("  /accept <文件ID> - 接受文件")
	fmt.Println("  /reject <文件ID> - 拒绝文件")
	fmt.Println("  /transfers - 查看文件传输列表")
	fmt.Println("  /list - 查看在线用户")
	fmt.Println("  /name <新名称> - 更改用户名")
	fmt.Println("  /web - 打开Web界面")
	fmt.Println("  /webstop - 关闭Web界面")
	fmt.Println("  /help - 显示帮助信息")
	fmt.Println("  /quit - 退出程序")
	fmt.Println("===========================================")
}

// 启动命令行界面
func (node *P2PNode) startCLI() {
	fmt.Println("\n===========================================")
	fmt.Println("           LANShare P2P 客户端")
	fmt.Println("===========================================")
	node.showCommandHelp()

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
			node.addChatMessage("我", "all", text, true, false)
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
			node.addChatMessage(node.Name, targetName, message, true, true)
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

		// 广播名称更新消息
		updateMsg := Message{
			Type:    "update_name",
			From:    node.ID,
			To:      "all",
			Content: node.Name,
		}
		node.broadcastMessage(updateMsg)
		
	case "/web":
		if !node.WebEnabled {
			// 动态启用Web界面
			node.WebEnabled = true
			node.startWebGUI()
			fmt.Println("Web界面已启用")
		}
		webURL := fmt.Sprintf("http://%s:%d", node.LocalIP, node.WebPort)
		fmt.Printf("Web界面地址: %s\n", webURL)
		// openBrowser(webURL)  // 注释掉自动打开浏览器的功能
		
	case "/webstop":
		if !node.WebEnabled {
			fmt.Println("Web界面未启用")
			return
		}
		node.stopWebGUI()
		
	case "/send":
		if len(parts) < 3 {
			fmt.Println("用法: /send <用户名> <文件路径>")
			return
		}
		targetName := parts[1]
		filePath := strings.Join(parts[2:], " ")
		node.sendFileTransferRequest(filePath, targetName)
		
	case "/transfers":
		node.showFileTransfers()

	case "/accept":
		if len(parts) < 2 {
			fmt.Println("用法: /accept <文件ID>")
			return
		}
		node.respondToFileTransfer(parts[1], true)

	case "/reject":
		if len(parts) < 2 {
			fmt.Println("用法: /reject <文件ID>")
			return
		}
		node.respondToFileTransfer(parts[1], false)
		
	case "/help":
		node.showCommandHelp()
		
	default:
		fmt.Printf("未知命令: %s\n", parts[0])
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
	var cliMode bool
	var showHelp bool
	
	flag.StringVar(&name, "name", "", "指定用户名")
	flag.BoolVar(&cliMode, "cli", false, "仅使用命令行模式")
	flag.BoolVar(&showHelp, "help", false, "显示此帮助信息")
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
		fmt.Println("  -cli            仅使用命令行模式")
		fmt.Println("  -help           显示此帮助信息")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Printf("  %s                    # 交互式选择模式\n", os.Args[0])
		fmt.Printf("  %s -cli               # 命令行模式\n", os.Args[0])
		fmt.Printf("  %s -name 张三         # 指定用户名\n", os.Args[0])
		fmt.Println()
		fmt.Println("网络端口:")
		fmt.Println("  P2P通信: 8888 (TCP)")
		fmt.Println("  服务发现: 9999 (UDP)")
		return
	}

	fmt.Println("===========================================")
	fmt.Println("           LANShare P2P 启动器")
	fmt.Println("===========================================")

	// 默认使用命令行模式
	webMode := false

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
	
	fmt.Printf("启动 P2P 客户端 (命令行模式)，用户名: %s\n", name)
	fmt.Println("===========================================")

	node := NewP2PNode(name, webMode)
	
	if err := node.Start(); err != nil {
		fmt.Printf("启动P2P节点失败: %v\n", err)
		return
	}

	// 启动命令行界面
	node.startCLI()
}
