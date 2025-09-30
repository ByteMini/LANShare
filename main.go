package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 生成消息ID
func generateMessageID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// 创建新的P2P节点
func NewP2PNode(name string, webEnabled bool, localIP string) *P2PNode {
	if localIP == "" {
		localIP = getLocalIP()
	}
	nodeID := fmt.Sprintf("%s_%d", localIP, time.Now().Unix())
	address := fmt.Sprintf("%s:%d", localIP, 8888)
	
	node := &P2PNode{
		LocalIP:       localIP,
		LocalPort:     8888,
		Name:          name,
		ID:            nodeID,
		Address:       address,
		Peers:         make(map[string]*Peer),
		MessageChan:   make(chan Message, 100),
		Running:       false,
		DiscoveryPort: 9999,
		WebPort:       8080,
		Messages:      make([]ChatMessage, 0),
		WebEnabled:    webEnabled,
		FileTransfers: make(map[string]*FileTransferStatus),
		ACLs:          make(map[string]map[string]bool),
		ACLMutex:      sync.RWMutex{},
	}

	// 初始化数据库
	db, err := sql.Open("sqlite3", "message.db")
	if err != nil {
		fmt.Printf("打开数据库失败: %v\n", err)
		node.DB = nil
		return node
	}
	node.DB = db

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			sender TEXT NOT NULL,
			recipient TEXT,
			content BLOB NOT NULL,
			nonce BLOB,
			is_private BOOLEAN DEFAULT FALSE,
			is_own BOOLEAN DEFAULT FALSE,
			message_type TEXT DEFAULT 'text',
			message_id TEXT,
			reply_to_id TEXT,
			reply_to_content TEXT,
			reply_to_sender TEXT,
			file_name TEXT,
			file_size INTEGER DEFAULT 0,
			file_type TEXT,
			file_url TEXT,
			file_data TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON messages(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_chat ON messages(recipient, is_private);
		CREATE INDEX IF NOT EXISTS idx_message_type ON messages(message_type);
		CREATE INDEX IF NOT EXISTS idx_message_id ON messages(message_id);
	`)
	if err != nil {
		fmt.Printf("创建数据库表失败: %v\n", err)
		db.Close()
		node.DB = nil
		return node
	}

	// 清理旧消息（保留30天）
	_, err = db.Exec("DELETE FROM messages WHERE timestamp < DATETIME('now', '-30 days')")
	if err != nil {
		fmt.Printf("清理旧消息失败: %v\n", err)
	}

	// Now load history with proper key
	node.loadHistoryFromDB()

	// 设置 WAL 模式以提高并发
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		fmt.Printf("设置 WAL 模式失败: %v\n", err)
	}
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		fmt.Printf("设置 WAL 模式失败: %v\n", err)
	}

	return node
}

// ACL 方法实现
func (node *P2PNode) isBlocked(targetAddress string) bool {
	node.ACLMutex.RLock()
	defer node.ACLMutex.RUnlock()
	if acl, exists := node.ACLs[node.Address]; exists {
		if val, ok := acl[targetAddress]; ok {
			return !val
		}
	}
	return false
}

func (node *P2PNode) blockUser(targetAddress string) {
	node.ACLMutex.Lock()
	defer node.ACLMutex.Unlock()
	if node.ACLs[node.Address] == nil {
		node.ACLs[node.Address] = make(map[string]bool)
	}
	node.ACLs[node.Address][targetAddress] = false
	// 查找用户名显示
	displayName := targetAddress
	node.PeersMutex.RLock()
	for _, peer := range node.Peers {
		if peer.Address == targetAddress {
			displayName = peer.Name
			break
		}
	}
	node.PeersMutex.RUnlock()
	fmt.Printf("已屏蔽用户 %s (%s)\n", displayName, targetAddress)
}

func (node *P2PNode) unblockUser(targetAddress string) {
	node.ACLMutex.Lock()
	defer node.ACLMutex.Unlock()
	if node.ACLs[node.Address] == nil {
		node.ACLs[node.Address] = make(map[string]bool)
	}
	node.ACLs[node.Address][targetAddress] = true
	// 查找用户名显示
	displayName := targetAddress
	node.PeersMutex.RLock()
	for _, peer := range node.Peers {
		if peer.Address == targetAddress {
			displayName = peer.Name
			break
		}
	}
	node.PeersMutex.RUnlock()
	fmt.Printf("已解除屏蔽用户 %s (%s)\n", displayName, targetAddress)
}

func (node *P2PNode) showACL() {
	node.ACLMutex.RLock()
	defer node.ACLMutex.RUnlock()
	
	fmt.Println("屏蔽列表:")
	if acl, exists := node.ACLs[node.Address]; exists {
		blocked := 0
		for addr, allowed := range acl {
			if !allowed {
				// 查找用户名
				displayName := addr
				node.PeersMutex.RLock()
				for _, peer := range node.Peers {
					if peer.Address == addr {
						displayName = peer.Name
						break
					}
				}
				node.PeersMutex.RUnlock()
				fmt.Printf(" - %s (%s)\n", displayName, addr)
				blocked++
			}
		}
		if blocked == 0 {
			fmt.Println("  无屏蔽用户")
		}
	} else {
		fmt.Println("  无屏蔽用户")
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
	fmt.Println("  /web [端口] - 打开Web界面 (默认8080)")
	fmt.Println("  /webstop - 关闭Web界面")
	fmt.Println("  /webstatus - 查看Web状态")
	fmt.Println("  /block <用户名> - 屏蔽用户")
	fmt.Println("  /unblock <用户名> - 解除屏蔽")
	fmt.Println("  /acl - 查看屏蔽列表")
	fmt.Println("  /history [用户名] [数量] - 查看历史消息 (默认20条)")
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
		var targetID, targetAddress string
		node.PeersMutex.RLock()
		for id, peer := range node.Peers {
			if peer.Name == targetName && peer.IsActive {
				targetID = id
				targetAddress = peer.Address
				break
			}
		}
		node.PeersMutex.RUnlock()
		
		if targetID == "" {
			fmt.Printf("错误: 用户 '%s' 不在线或不存在\n", targetName)
			fmt.Println("提示: 使用 /list 命令查看在线用户")
			return
		}

		if node.isBlocked(targetAddress) {
			fmt.Printf("错误: 用户 '%s' 被屏蔽，无法发送私聊\n", targetName)
			fmt.Println("提示: 使用 /unblock 命令解除屏蔽")
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
				blocked := node.isBlocked(peer.Address)
				status := ""
				if blocked {
					status = " (屏蔽)"
				}
				fmt.Printf("  %s%s (%s)\n", peer.Name, status, peer.Address)
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
			
			// 支持指定端口
			if len(parts) > 1 {
				if port, err := strconv.Atoi(parts[1]); err == nil && port > 0 && port < 65536 {
					node.WebPort = port
					fmt.Printf("Web端口设置为: %d\n", node.WebPort)
				} else {
					fmt.Println("无效端口号，使用默认端口 8080")
				}
			}
			
			node.startWebGUI()
		}
		
	case "/webstop":
		if !node.WebEnabled {
			fmt.Println("Web界面未启用")
			return
		}
		node.stopWebGUI()
		
	case "/block":
		if len(parts) < 2 {
			fmt.Println("用法: /block <用户名>")
			return
		}
		targetName := parts[1]
		// 查找目标地址
		var targetAddress string
		node.PeersMutex.RLock()
		for _, peer := range node.Peers {
			if peer.Name == targetName && peer.IsActive {
				targetAddress = peer.Address
				break
			}
		}
		node.PeersMutex.RUnlock()
		
		if targetAddress == "" {
			fmt.Printf("用户 %s 不在线，无法屏蔽\n", targetName)
			return
		}
		node.blockUser(targetAddress)
		
	case "/unblock":
		if len(parts) < 2 {
			fmt.Println("用法: /unblock <用户名>")
			return
		}
		targetName := parts[1]
		// 查找目标地址
		var targetAddress string
		node.PeersMutex.RLock()
		for _, peer := range node.Peers {
			if peer.Name == targetName && peer.IsActive {
				targetAddress = peer.Address
				break
			}
		}
		node.PeersMutex.RUnlock()
		
		if targetAddress == "" {
			fmt.Printf("用户 %s 不在线，但仍可解除屏蔽\n", targetName)
			// 尝试直接用用户名作为地址（向后兼容，但不推荐）
			node.unblockUser(targetName)
			return
		}
		node.unblockUser(targetAddress)
		
	case "/acl":
		node.showACL()
		
	case "/send":
		if len(parts) < 3 {
			fmt.Println("用法: /send <用户名> <文件路径>")
			return
		}
		targetName := parts[1]
		filePath := strings.Join(parts[2:], " ")
		
		// 查找目标用户
		var targetID, targetAddress string
		node.PeersMutex.RLock()
		for id, peer := range node.Peers {
			if peer.Name == targetName && peer.IsActive {
				targetID = id
				targetAddress = peer.Address
				break
			}
		}
		node.PeersMutex.RUnlock()
		
		if targetID == "" {
			fmt.Printf("错误: 用户 '%s' 不在线或不存在\n", targetName)
			fmt.Println("提示: 使用 /list 命令查看在线用户")
			return
		}

		if node.isBlocked(targetAddress) {
			fmt.Printf("错误: 用户 '%s' 被屏蔽，无法发送文件\n", targetName)
			fmt.Println("提示: 使用 /unblock 命令解除屏蔽")
			return
		}
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
		
	case "/webstatus":
		if node.WebEnabled {
			webURL := fmt.Sprintf("http://127.0.0.1:%d", node.WebPort)
			fmt.Printf("Web界面已启用\n地址: %s\n端口: %d\n", webURL, node.WebPort)
		} else {
			fmt.Println("Web界面未启用")
		}
		
	case "/help":
		node.showCommandHelp()
		
	case "/history":
		chatId := "all"
		limit := 20
		if len(parts) > 1 {
			chatId = parts[1]
		}
		if len(parts) > 2 {
			if l, err := strconv.Atoi(parts[2]); err == nil && l > 0 {
				limit = l
			}
		}

		if node.DB == nil {
			fmt.Println("数据库未初始化")
			return
		}

		var rows *sql.Rows
		var err error

		if chatId == "all" {
			rows, err = node.DB.Query(`
				SELECT sender, recipient, content, nonce, is_private, is_own, timestamp,
					   message_type, message_id, reply_to_id, reply_to_content, reply_to_sender,
					   file_name, file_size, file_type
				FROM messages
				WHERE recipient = 'all' AND is_private = FALSE
				ORDER BY timestamp DESC
				LIMIT ?
			`, limit)
		} else {
			rows, err = node.DB.Query(`
				SELECT sender, recipient, content, nonce, is_private, is_own, timestamp,
					   message_type, message_id, reply_to_id, reply_to_content, reply_to_sender,
					   file_name, file_size, file_type
				FROM messages
				WHERE is_private = TRUE AND (
					(sender = ? AND recipient = ?) OR
					(sender = ? AND recipient = ?)
				)
				ORDER BY timestamp DESC
				LIMIT ?
			`, node.Name, chatId, chatId, node.Name, limit)
		}

		if err != nil {
			fmt.Printf("查询历史消息失败: %v\n", err)
			return
		}
		defer rows.Close()

		fmt.Printf("历史消息 (%s, 最近 %d 条):\n", chatId, limit)
		count := 0
		for rows.Next() {
			var sender, recipient string
			var content, nonce []byte
			var isPrivate, isOwn bool
			var timestamp time.Time
			var messageType, messageID, replyToID, replyToContent, replyToSender, fileName, fileType string
			var fileSize int64

			err = rows.Scan(&sender, &recipient, &content, &nonce, &isPrivate, &isOwn, &timestamp,
				&messageType, &messageID, &replyToID, &replyToContent, &replyToSender,
				&fileName, &fileSize, &fileType)
			if err != nil {
				continue
			}

			// 解密
			plaintext, err := decryptMessage(node.LocalDBKey, content, nonce)
			if err != nil {
				fmt.Printf("解密消息失败: %v\n", err)
				continue
			}

			displayContent := string(plaintext)
			if strings.HasPrefix(displayContent, "emoji:") {
				displayContent = "[表情]"
			} else if messageType == "image" && fileName != "" {
				displayContent = fmt.Sprintf("[图片: %s]", fileName)
			} else if messageType == "file" && fileName != "" {
				displayContent = fmt.Sprintf("[文件: %s (%s)]", fileName, formatFileSize(fileSize))
			} else if messageType == "reply" && replyToSender != "" {
				displayContent = fmt.Sprintf("[回复 %s]: %s", replyToSender, displayContent)
			}

			prefix := ""
			if isPrivate {
				prefix = "(私聊) "
			}
			if isOwn {
				fmt.Printf("[%s] 我 %s%s: %s\n", timestamp.Format("15:04:05"), prefix, recipient, displayContent)
			} else {
				fmt.Printf("[%s] %s %s: %s\n", timestamp.Format("15:04:05"), sender, prefix, displayContent)
			}
			count++
		}
		if count == 0 {
			fmt.Println("无历史消息")
		}
		
	default:
		fmt.Printf("未知命令: %s\n", parts[0])
	}
}

// 清理内存 - 移除旧的完成/失败传输
func (node *P2PNode) cleanupMemory() {
	now := time.Now()

	// 每5分钟清理一次
	if now.Sub(node.lastCleanupTime) < 5*time.Minute {
		return
	}
	node.lastCleanupTime = now

	node.FileTransfersMutex.Lock()
	defer node.FileTransfersMutex.Unlock()

	// 清理完成或失败超过10分钟的传输
	var toDelete []string
	for id, transfer := range node.FileTransfers {
		if (transfer.Status == "completed" || transfer.Status == "failed") &&
		   now.Sub(transfer.EndTime) > 10*time.Minute {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(node.FileTransfers, id)
	}

	if len(toDelete) > 0 {
		fmt.Printf("已清理 %d 个旧文件传输记录\n", len(toDelete))
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
	if node.DB != nil {
		node.DB.Close()
	}
	fmt.Println("P2P节点已停止")
}

func (node *P2PNode) loadHistoryFromDB() {
	if node.DB == nil {
		return
	}

	rows, err := node.DB.Query(`
		SELECT sender, recipient, content, nonce, is_private, is_own, timestamp,
			   message_type, message_id, reply_to_id, reply_to_content, reply_to_sender,
			   file_name, file_size, file_type, file_url, file_data
		FROM messages
		ORDER BY timestamp DESC
		LIMIT 20
	`)
	if err != nil {
		fmt.Printf("加载历史消息失败: %v\n", err)
		return
	}
	defer rows.Close()

	var dbMsgs []ChatMessage
	for rows.Next() {
		var sender, recipient string
		var content, nonce []byte
		var isPrivate, isOwn bool
		var ts time.Time
		var messageType, messageID, replyToID, replyToContent, replyToSender, fileName, fileType string
		var fileSize int64

		var fileURL string
		var fileData string
		if err := rows.Scan(&sender, &recipient, &content, &nonce, &isPrivate, &isOwn, &ts,
			&messageType, &messageID, &replyToID, &replyToContent, &replyToSender,
			&fileName, &fileSize, &fileType, &fileURL, &fileData); err != nil {
			continue
		}

		plaintext, err := decryptMessage(node.LocalDBKey, content, nonce)
		if err != nil {
			fmt.Printf("解密历史消息失败: %v\n", err)
			continue
		}

		cm := ChatMessage{
			Sender:        sender,
			Recipient:     recipient,
			Content:       string(plaintext),
			Timestamp:     ts,
			IsOwn:         isOwn,
			IsPrivate:     isPrivate,
			MessageType:   messageType,
			MessageID:     messageID,
			ReplyToID:     replyToID,
			ReplyToContent: replyToContent,
			ReplyToSender: replyToSender,
			FileName:      fileName,
			FileSize:      fileSize,
			FileType:      fileType,
			FileURL:       fileURL,
		}
		dbMsgs = append(dbMsgs, cm)
	}

	// Reverse to get oldest first (ASC)
	for i, j := 0, len(dbMsgs)-1; i < j; i, j = i+1, j-1 {
		dbMsgs[i], dbMsgs[j] = dbMsgs[j], dbMsgs[i]
	}

	node.MessagesMutex.Lock()
	node.Messages = dbMsgs
	node.MessagesMutex.Unlock()
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
		fmt.Println("Web 界面: 在 CLI 模式下使用 /web 命令启用")
		fmt.Println("网络端口:")
		fmt.Println("  P2P通信: 8888 (TCP)")
		fmt.Println("  服务发现: 9999 (UDP)")
		return
	}

	fmt.Println("===========================================")
	fmt.Println("           LANShare P2P 启动器")
	fmt.Println("===========================================")

	// 默认不启用 Web 模式
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

	// 先选择网络接口
	localIP := getLocalIP()

	// 然后选择Web模式
	webMode = false
	fmt.Print("是否启用Web界面? (y/N): ")
	var webInput string
	fmt.Scanln(&webInput)
	if strings.ToLower(strings.TrimSpace(webInput)) == "y" {
		webMode = true
	}

	node := NewP2PNode(name, webMode, localIP)
	
	if webMode {
		fmt.Print("请输入Web端口 (默认8080): ")
		var portInput string
		fmt.Scanln(&portInput)
		if portInput != "" {
			if port, err := strconv.Atoi(portInput); err == nil && port > 0 && port < 65536 {
				node.WebPort = port
				fmt.Printf("Web端口设置为: %d\n", port)
			} else {
				fmt.Println("无效端口，使用默认8080")
			}
		}
	}
	
	if err := node.Start(); err != nil {
		fmt.Printf("启动P2P节点失败: %v\n", err)
		return
	}

	// 启动命令行界面
	node.startCLI()
}
