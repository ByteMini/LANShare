package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

//go:embed all:web emoji_gifs.json
var webFS embed.FS

// 启动Web GUI服务器
func (node *P2PNode) startWebGUI() {
	if !node.WebEnabled {
		return
	}

	// 创建新的路由器
	mux := http.NewServeMux()

	// 创建一个子文件系统，根目录为 'web'
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("无法创建嵌入式文件子系统: %v", err)
	}

	// 从嵌入式文件系统读取HTML模板
	tmpl, err := template.ParseFS(subFS, "index.html")
	if err != nil {
		log.Printf("从嵌入式文件系统读取HTML模板失败: %v", err)
		return
	}

	// 主页处理器
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.Execute(w, node); err != nil {
			log.Printf("执行模板失败: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// 静态文件服务器
	// 请求 /static/style.css -> 在 subFS 中查找 style.css
	staticServer := http.FileServer(http.FS(subFS))
	mux.Handle("/static/", http.StripPrefix("/static/", staticServer))

	// GIF 表情文件服务器
	// 请求 /emoji-gifs/heart.gif -> 从 assets/emoji-gifs/heart.gif 服务
	emojiGifServer := http.FileServer(http.Dir("assets/emoji-gifs"))
	mux.Handle("/emoji-gifs/", http.StripPrefix("/emoji-gifs/", emojiGifServer))

	// 图片文件服务器
	// 请求 /images/filename.jpg -> 从 images/filename.jpg 服务
	imageServer := http.FileServer(http.Dir("images"))
	mux.Handle("/images/", http.StripPrefix("/images/", imageServer))

	// 获取 GIF 表情列表处理器
	mux.HandleFunc("/emoji-gifs-list", func(w http.ResponseWriter, r *http.Request) {
		// 读取嵌入的 emoji_gifs.json 文件
		data, err := webFS.Open("emoji_gifs.json")
		if err != nil {
			// 如果文件不存在，返回空列表
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		defer data.Close()

		// 直接转发文件内容
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, data)
	})

	// 加载历史消息处理器 (for web frontend)
	mux.HandleFunc("/loadhistory", func(w http.ResponseWriter, r *http.Request) {
		if node.DB == nil {
			http.Error(w, "Database not available", http.StatusInternalServerError)
			return
		}

		chatId := r.URL.Query().Get("chatId")
		if chatId == "" {
			chatId = "all"
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 20
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
		offsetStr := r.URL.Query().Get("offset")
		offset := 0
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}

		var rows *sql.Rows
		var err error
		var query string
		var args []interface{}

		if chatId == "all" {
			query = `
				SELECT sender, recipient, content, nonce, is_private, is_own, timestamp,
					   message_type, message_id, reply_to_id, reply_to_content, reply_to_sender,
					   file_name, file_size, file_type, file_url, file_data
				FROM messages
				WHERE recipient = 'all' AND is_private = FALSE
				ORDER BY timestamp ASC
				LIMIT ? OFFSET ?
			`
			args = []interface{}{limit, offset}
		} else {
			query = `
				SELECT sender, recipient, content, nonce, is_private, is_own, timestamp,
					   message_type, message_id, reply_to_id, reply_to_content, reply_to_sender,
					   file_name, file_size, file_type, file_url, file_data
				FROM messages
				WHERE is_private = TRUE AND (
					(sender = ? AND recipient = ?) OR
					(sender = ? AND recipient = ?)
				)
				ORDER BY timestamp ASC
				LIMIT ? OFFSET ?
			`
			args = []interface{}{node.Name, chatId, chatId, node.Name, limit, offset}
		}

		rows, err = node.DB.Query(query, args...)
		if err != nil {
			http.Error(w, "Query failed", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type HistoryMsg struct {
			ChatMessage
			SenderName string `json:"senderName"`
		}

		var history []HistoryMsg
		for rows.Next() {
			var sender, recipient string
			var content, nonce []byte
			var isPrivate, isOwn bool
			var tsStr string
			var messageType, messageID, replyToID, replyToContent, replyToSender, fileName, fileType, fileURL, fileData string
			var fileSize int64

			err = rows.Scan(&sender, &recipient, &content, &nonce, &isPrivate, &isOwn, &tsStr,
				&messageType, &messageID, &replyToID, &replyToContent, &replyToSender,
				&fileName, &fileSize, &fileType, &fileURL, &fileData)
			if err != nil {
				continue
			}

			plaintext, err := decryptMessage(node.LocalDBKey, content, nonce)
			if err != nil {
				fmt.Printf("解密历史消息失败: %v\n", err)
				continue
			}

			ts, err := time.Parse("2006-01-02 15:04:05", tsStr)
			if err != nil {
				ts = time.Now()
			}

			senderName := sender
			if sender == node.Name {
				senderName = "我"
			}

			hm := HistoryMsg{
				ChatMessage: ChatMessage{
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
				},
				SenderName: senderName,
			}
			history = append(history, hm)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"messages": history,
		})
	})


	// Ping处理器，用于检查Web服务器是否在线
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 发送消息处理器
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		users := []string{node.Name + " (自己)"}
		
		node.PeersMutex.RLock()
		for _, peer := range node.Peers {
			if peer.IsActive {
				status := ""
				if node.isBlocked(peer.Address) {
					status = " (屏蔽)"
				}
				users = append(users, peer.Name + status)
			}
		}
		node.PeersMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": users,
		})
	})

	// 获取屏蔽列表处理器
	mux.HandleFunc("/acl", func(w http.ResponseWriter, r *http.Request) {
		node.ACLMutex.RLock()
		defer node.ACLMutex.RUnlock()
		
		blocked := []string{}
		if acl, exists := node.ACLs[node.Address]; exists {
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
					blocked = append(blocked, displayName)
				}
			}
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"blocked": blocked,
		})
	})

	// 发送文件处理器
	mux.HandleFunc("/sendfile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 解析multipart表单
		err := r.ParseMultipartForm(10 << 20) // 10MB限制
		if err != nil {
			http.Error(w, "文件太大或格式错误", http.StatusBadRequest)
			return
		}

		// 获取文件
		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "无法获取文件", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// 获取目标用户
		targetName := r.FormValue("targetName")
		if targetName == "" {
			http.Error(w, "请选择目标用户", http.StatusBadRequest)
			return
		}

		// 创建uploads目录
		uploadDir := "uploads"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			http.Error(w, "无法创建上传目录", http.StatusInternalServerError)
			return
		}

		// 创建临时文件
		tempFile, err := os.Create(filepath.Join(uploadDir, handler.Filename))
		if err != nil {
			http.Error(w, "无法创建临时文件", http.StatusInternalServerError)
			return
		}
		defer tempFile.Close()

		// 将上传的文件内容复制到临时文件
		if _, err := io.Copy(tempFile, file); err != nil {
			http.Error(w, "无法保存上传的文件", http.StatusInternalServerError)
			return
		}
		
		// 发送文件传输请求，使用临时文件的路径
		node.sendFileTransferRequest(tempFile.Name(), targetName)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("文件传输请求已发送"))
	})

	// 发送图片处理器
	mux.HandleFunc("/sendimage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 解析multipart表单
		err := r.ParseMultipartForm(10 << 20) // 10MB限制
		if err != nil {
			http.Error(w, "文件太大或格式错误", http.StatusBadRequest)
			return
		}

		// 获取图片文件
		file, handler, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "无法获取图片文件", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// 检查文件类型
		contentType := handler.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			http.Error(w, "只支持图片文件", http.StatusBadRequest)
			return
		}

		// 检查文件大小（限制为5MB）
		if handler.Size > 5<<20 {
			http.Error(w, "图片文件不能超过5MB", http.StatusBadRequest)
			return
		}

		// 获取目标用户
		targetName := r.FormValue("targetName")
		if targetName == "" {
			http.Error(w, "请选择目标用户", http.StatusBadRequest)
			return
		}

		// 创建images目录
		imageDir := "images"
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			http.Error(w, "无法创建图片目录", http.StatusInternalServerError)
			return
		}

		// 生成唯一文件名
		ext := filepath.Ext(handler.Filename)
		if ext == "" {
			ext = ".jpg" // 默认扩展名
		}
		imageFileName := fmt.Sprintf("%s_%d%s", generateMessageID(), time.Now().Unix(), ext)
		imagePath := filepath.Join(imageDir, imageFileName)

		// 保存图片文件
		imageFile, err := os.Create(imagePath)
		if err != nil {
			http.Error(w, "无法保存图片文件", http.StatusInternalServerError)
			return
		}
		defer imageFile.Close()

		// 读取图片数据用于base64编码
		imageData, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "读取图片数据失败", http.StatusInternalServerError)
			return
		}

		// 保存到文件
		if _, err := imageFile.Write(imageData); err != nil {
			http.Error(w, "保存图片失败", http.StatusInternalServerError)
			return
		}

		// 编码为base64
		imageBase64 := base64.StdEncoding.EncodeToString(imageData)

		// 发送图片消息
		imageURL := fmt.Sprintf("/images/%s", imageFileName)
		messageID := generateMessageID()

		// 创建图片消息
		imageMsg := Message{
			Type:        "chat",
			From:        node.ID,
			Content:     fmt.Sprintf("发送了图片: %s", handler.Filename),
			Timestamp:   time.Now(),
			MessageType: MessageTypeImage,
			MessageID:   messageID,
			FileName:    handler.Filename,
			FileSize:    handler.Size,
			FileType:    contentType,
			FileURL:     imageURL,
			FileData:    imageBase64, // 包含base64编码的图片数据
		}

		// 根据目标用户设置消息接收者
		if targetName == "all" {
			// 公聊消息
			imageMsg.To = "all"
			node.broadcastMessage(imageMsg)
		} else {
			// 私聊消息
			// 查找目标用户ID
			var targetID string
			node.PeersMutex.RLock()
			for id, peer := range node.Peers {
				if peer.Name == targetName && peer.IsActive {
					targetID = id
					break
				}
			}
			node.PeersMutex.RUnlock()

			if targetID == "" {
				http.Error(w, "目标用户不在线", http.StatusBadRequest)
				return
			}

			imageMsg.To = targetID

			// 发送消息
			if peer, exists := node.Peers[targetID]; exists {
				node.sendMessageToPeer(peer, imageMsg)
			}
		}

		// 添加到本地消息列表
		isPrivate := targetName != "all"
		node.addChatMessageWithType(
			node.Name, targetName, imageMsg.Content, true, isPrivate,
			MessageTypeImage, messageID, "", "", "", handler.Filename, handler.Size, contentType, imageURL,
		)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "success",
			"imageUrl": imageURL,
			"messageId": messageID,
		})
	})

	// 发送文件消息处理器（用于文件预览）
	mux.HandleFunc("/sendfilemsg", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			TargetName string `json:"targetName"`
			FileName   string `json:"fileName"`
			FileSize   int64  `json:"fileSize"`
			FileType   string `json:"fileType"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.TargetName == "" || req.FileName == "" {
			http.Error(w, "缺少必要参数", http.StatusBadRequest)
			return
		}

		// 查找目标用户ID
		var targetID string
		node.PeersMutex.RLock()
		for id, peer := range node.Peers {
			if peer.Name == req.TargetName && peer.IsActive {
				targetID = id
				break
			}
		}
		node.PeersMutex.RUnlock()

		if targetID == "" {
			http.Error(w, "目标用户不在线", http.StatusBadRequest)
			return
		}

		messageID := generateMessageID()
		content := fmt.Sprintf("分享了文件: %s", req.FileName)

		// 创建文件消息
		fileMsg := Message{
			Type:        "chat",
			From:        node.ID,
			To:          targetID,
			Content:     content,
			Timestamp:   time.Now(),
			MessageType: MessageTypeFile,
			MessageID:   messageID,
			FileName:    req.FileName,
			FileSize:    req.FileSize,
			FileType:    req.FileType,
		}

		// 发送消息
		if peer, exists := node.Peers[targetID]; exists {
			node.sendMessageToPeer(peer, fileMsg)
		}

		// 添加到本地消息列表
		isPrivate := targetID != "all"
		node.addChatMessageWithType(
			node.Name, req.TargetName, content, true, isPrivate,
			MessageTypeFile, messageID, "", "", "", req.FileName, req.FileSize, req.FileType, "",
		)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "success",
			"messageId": messageID,
		})
	})

	// 发送回复消息处理器
	mux.HandleFunc("/sendreply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			TargetName     string `json:"targetName"`
			ReplyContent   string `json:"replyContent"`
			OriginalMsgID  string `json:"originalMsgId"`
			OriginalSender string `json:"originalSender"`
			OriginalContent string `json:"originalContent"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.TargetName == "" || req.ReplyContent == "" || req.OriginalMsgID == "" {
			http.Error(w, "缺少必要参数", http.StatusBadRequest)
			return
		}

		// 查找目标用户ID
		var targetID string
		node.PeersMutex.RLock()
		for id, peer := range node.Peers {
			if peer.Name == req.TargetName && peer.IsActive {
				targetID = id
				break
			}
		}
		node.PeersMutex.RUnlock()

		if targetID == "" {
			http.Error(w, "目标用户不在线", http.StatusBadRequest)
			return
		}

		messageID := generateMessageID()
		content := req.ReplyContent

		// 创建回复消息
		replyMsg := Message{
			Type:            "chat",
			From:            node.ID,
			To:              targetID,
			Content:         content,
			Timestamp:       time.Now(),
			MessageType:     MessageTypeReply,
			MessageID:       messageID,
			ReplyToID:       req.OriginalMsgID,
			ReplyToContent:  req.OriginalContent,
			ReplyToSender:   req.OriginalSender,
		}

		// 发送消息
		if peer, exists := node.Peers[targetID]; exists {
			node.sendMessageToPeer(peer, replyMsg)
		}

		// 添加到本地消息列表
		isPrivate := targetID != "all"
		node.addChatMessageWithType(
			node.Name, req.TargetName, content, true, isPrivate,
			MessageTypeReply, messageID, req.OriginalMsgID, req.OriginalContent, req.OriginalSender,
			"", 0, "", "",
		)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "success",
			"messageId": messageID,
		})
	})

	// 获取文件传输列表处理器
	mux.HandleFunc("/filetransfers", func(w http.ResponseWriter, r *http.Request) {
		node.FileTransfersMutex.RLock()
		transfers := make([]*FileTransferStatus, 0, len(node.FileTransfers))
		for _, transfer := range node.FileTransfers {
			transfers = append(transfers, transfer)
		}
		node.FileTransfersMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"transfers": transfers,
		})
	})

	// 处理文件传输响应处理器
	mux.HandleFunc("/fileresponse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			FileID   string `json:"fileId"`
			Accepted bool   `json:"accepted"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// 调用核心逻辑来处理响应
		node.respondToFileTransfer(req.FileID, req.Accepted)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// 关闭Web服务器处理器
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 简单的身份验证 - 只允许本地访问
		if r.RemoteAddr != "127.0.0.1" && !strings.HasPrefix(r.RemoteAddr, "[::1]") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Web服务器正在关闭..."))
		
		// 异步关闭Web服务器
		go node.stopWebGUI()
	})

	// 检查表情目录处理器
	mux.HandleFunc("/check-emoji-dir", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := os.Stat("assets/emoji-gifs"); os.IsNotExist(err) {
			json.NewEncoder(w).Encode(map[string]bool{"exists": false})
		} else {
			json.NewEncoder(w).Encode(map[string]bool{"exists": true})
		}
	})

	// 创建HTTP服务器
	node.WebServer = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", node.WebPort),
		Handler: mux,
	}

	// 启动Web服务器
		go func() {
			webURL := fmt.Sprintf("http://127.0.0.1:%d", node.WebPort)
			fmt.Printf("Web界面已启动: %s\n", webURL)
			fmt.Println("请手动在浏览器中打开上述URL访问Web界面")
			
			if err := node.WebServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Web服务器启动失败: %v", err)
			}
		}()
}

// 停止Web GUI服务器
func (node *P2PNode) stopWebGUI() {
	if node.WebServer != nil {
		fmt.Println("正在关闭Web服务器...")
		
		// 创建关闭上下文，最多等待5秒
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := node.WebServer.Shutdown(ctx); err != nil {
			log.Printf("Web服务器关闭失败: %v", err)
		} else {
			fmt.Println("Web服务器已关闭")
		}
		
		node.WebEnabled = false
		node.WebServer = nil
	}
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
		node.addChatMessage("我", "all", text, true, false)
	}
}

// 添加聊天消息（扩展版）
func (node *P2PNode) addChatMessage(sender, recipient, content string, isOwn, isPrivate bool) {
	node.addChatMessageWithType(sender, recipient, content, isOwn, isPrivate, MessageTypeText, "", "", "", "", "", 0, "", "")
}

// 添加聊天消息（完整版）
func (node *P2PNode) addChatMessageWithType(sender, recipient, content string, isOwn, isPrivate bool,
	messageType, messageID, replyToID, replyToContent, replyToSender, fileName string, fileSize int64, fileType, fileURL string) {

	// 生成消息ID（如果未提供）
	if messageID == "" {
		messageID = generateMessageID()
	}

	if node.WebEnabled {
		node.MessagesMutex.Lock()
		defer node.MessagesMutex.Unlock()

		msg := ChatMessage{
			Sender:        sender,
			Recipient:     recipient,
			Content:       content,
			Timestamp:     time.Now(),
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

		node.Messages = append(node.Messages, msg)

		// 保持最近100条消息
		if len(node.Messages) > 100 {
			node.Messages = node.Messages[1:]
		}
	}

	// 保存到数据库
	if node.DB != nil {
		ciphertext, nonce, err := encryptMessage(node.LocalDBKey, []byte(content))
		if err != nil {
			fmt.Printf("加密消息失败: %v\n", err)
		} else {
			_, err = node.DB.Exec(`
				INSERT INTO messages (
					sender, recipient, content, nonce, is_private, is_own,
					message_type, message_id, reply_to_id, reply_to_content,
					reply_to_sender, file_name, file_size, file_type, file_url, file_data
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				sender, recipient, ciphertext, nonce, isPrivate, isOwn,
				messageType, messageID, replyToID, replyToContent,
				replyToSender, fileName, fileSize, fileType, fileURL, "")
			if err != nil {
				fmt.Printf("保存消息到数据库失败: %v\n", err)
			}
		}
	}

	// 命令行显示
	timestamp := time.Now().Format("15:04:05")
	displayContent := content
	if strings.HasPrefix(content, "emoji:") {
		emojiId := strings.TrimPrefix(content, "emoji:")
		if strings.HasPrefix(emojiId, "gif-") {
			emojiId = strings.TrimPrefix(emojiId, "gif-")
		}
		displayContent = "[发送了表情]"
		
		// 从 emoji_gifs.json 查找表情名称
		data, err := webFS.Open("emoji_gifs.json")
		if err == nil {
			defer data.Close()
			type EmojiEntry struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			var entries []EmojiEntry
			if json.NewDecoder(data).Decode(&entries) == nil {
				for _, e := range entries {
					if e.ID == emojiId {
						displayContent = fmt.Sprintf("[emoji: %s]", e.Name)
						break
					}
				}
			}
		}
	}
	if isPrivate {
		fmt.Printf("[%s] %s (私聊): %s\n", timestamp, sender, displayContent)
	} else {
		fmt.Printf("[%s] %s: %s\n", timestamp, sender, displayContent)
	}
}
