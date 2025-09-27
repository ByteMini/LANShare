package main

import (
	"context"
	"embed"
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

	// 添加聊天消息
func (node *P2PNode) addChatMessage(sender, recipient, content string, isOwn, isPrivate bool) {
	if node.WebEnabled {
		node.MessagesMutex.Lock()
		defer node.MessagesMutex.Unlock()
		
		msg := ChatMessage{
			Sender:    sender,
			Recipient: recipient,
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
