package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// 生成文件ID
func generateFileID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// 发送文件传输请求
func (node *P2PNode) sendFileTransferRequest(filePath string, targetName string) {
	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("文件不存在或无法访问: %s\n", filePath)
		return
	}

	// 检查文件大小
	if fileInfo.Size() > 100*1024*1024 { // 100MB限制
		fmt.Printf("文件大小超过限制 (最大100MB): %s\n", formatFileSize(fileInfo.Size()))
		return
	}

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

	// 生成文件ID
	fileID := generateFileID()

	// 创建文件传输请求
	request := FileTransferRequest{
		Type:      "file_request",
		FileID:    fileID,
		FileName:  filepath.Base(filePath),
		FileSize:  fileInfo.Size(),
		From:      node.ID,
		To:        targetID,
		Timestamp: time.Now(),
	}

	// 添加到传输状态
	node.FileTransfersMutex.Lock()
	node.FileTransfers[fileID] = &FileTransferStatus{
		FileID:    fileID,
		FileName:  request.FileName,
		FilePath:  filePath, // 保存完整路径
		FileSize:  request.FileSize,
		Progress:  0,
		Status:    "pending",
		Direction: "send",
		PeerName:  targetName,
		PeerID:    targetID, // 存储目标用户的peer ID
		StartTime: time.Now(),
	}
	node.FileTransfersMutex.Unlock()

	fmt.Printf("向 %s 发送文件传输请求: %s (%s)\n", 
		targetName, request.FileName, formatFileSize(request.FileSize))

	// 发送请求
	if peer, exists := node.Peers[targetID]; exists {
		msg := Message{
			Type:      "file_request",
			From:      node.ID,
			To:        targetID,
			Timestamp: time.Now(),
			Data:      request,
		}
		node.sendMessageToPeer(peer, msg)
	}
}

// 处理文件传输请求
func (node *P2PNode) handleFileTransferRequest(request FileTransferRequest) {
	fmt.Printf("\n收到来自 %s 的文件传输请求: %s (%s)\n",
		node.getPeerName(request.From), request.FileName, formatFileSize(request.FileSize))

	// 添加到传输状态
	node.FileTransfersMutex.Lock()
	node.FileTransfers[request.FileID] = &FileTransferStatus{
		FileID:    request.FileID,
		FileName:  request.FileName,
		FileSize:  request.FileSize,
		Progress:  0,
		Status:    "pending",
		Direction: "receive",
		PeerName:  node.getPeerName(request.From),
		PeerID:    request.From, // 存储发送方的peer ID
		StartTime: time.Now(),
	}
	node.FileTransfersMutex.Unlock()

	// 通知用户
	fmt.Printf("要接受，请输入: /accept %s\n", request.FileID)
	fmt.Printf("要拒绝，请输入: /reject %s\n", request.FileID)
}

// 响应文件传输请求
func (node *P2PNode) respondToFileTransfer(fileID string, accepted bool) {
	node.FileTransfersMutex.Lock()
	transfer, exists := node.FileTransfers[fileID]
	if !exists || transfer.Direction != "receive" {
		node.FileTransfersMutex.Unlock()
		fmt.Println("无效的文件传输ID")
		return
	}
	node.FileTransfersMutex.Unlock()

	// 发送响应
	responseMsg := FileTransferResponse{
		Type:      "file_response",
		FileID:    fileID,
		Accepted:  accepted,
		Message:   "",
		Timestamp: time.Now(),
	}

	// 找到请求来源的Peer
	var fromPeerID string
	node.PeersMutex.RLock()
	for id, peer := range node.Peers {
		if peer.Name == transfer.PeerName {
			fromPeerID = id
			break
		}
	}
	node.PeersMutex.RUnlock()

	if fromPeerID == "" {
		fmt.Println("找不到文件发送方")
		return
	}

	if accepted {
		responseMsg.Message = "文件传输已接受"
		fmt.Printf("已接受文件传输，准备接收文件...\n")
		node.FileTransfersMutex.Lock()
		transfer.Status = "transferring"
		node.FileTransfersMutex.Unlock()
	} else {
		responseMsg.Message = "文件传输被拒绝"
		fmt.Printf("已拒绝文件传输\n")
		
		// 清理状态
		node.FileTransfersMutex.Lock()
		delete(node.FileTransfers, fileID)
		node.FileTransfersMutex.Unlock()
	}

	if peer, exists := node.Peers[fromPeerID]; exists {
		msg := Message{
			Type:      "file_response",
			From:      node.ID,
			To:        fromPeerID,
			Timestamp: time.Now(),
			Data:      responseMsg,
		}
		node.sendMessageToPeer(peer, msg)
	}
}

// 处理文件传输响应
func (node *P2PNode) handleFileTransferResponse(response FileTransferResponse) {
	node.FileTransfersMutex.RLock()
	transfer, exists := node.FileTransfers[response.FileID]
	node.FileTransfersMutex.RUnlock()

	if !exists {
		return
	}

	if response.Accepted {
		fmt.Printf("文件传输请求已被接受，开始发送文件: %s\n", transfer.FileName)
		// 更新状态
		node.FileTransfersMutex.Lock()
		transfer.Status = "transferring"
		node.FileTransfersMutex.Unlock()

		// 开始发送文件
		go node.sendFile(transfer.FileID, transfer.FilePath)
	} else {
		fmt.Printf("文件传输请求被拒绝: %s\n", response.Message)
		// 清理状态
		node.FileTransfersMutex.Lock()
		delete(node.FileTransfers, response.FileID)
		node.FileTransfersMutex.Unlock()
	}
}

// 发送文件
func (node *P2PNode) sendFile(fileID string, filePath string) {
	const chunkSize = 64 * 1024 // 64KB

	// 查找目标用户
	node.FileTransfersMutex.RLock()
	transfer, exists := node.FileTransfers[fileID]
	if !exists {
		node.FileTransfersMutex.RUnlock()
		fmt.Printf("发送文件失败: 无效的文件ID %s\n", fileID)
		return
	}
	targetName := transfer.PeerName
	node.FileTransfersMutex.RUnlock()

	var targetPeer *Peer
	node.PeersMutex.RLock()
	for _, p := range node.Peers {
		if p.Name == targetName {
			targetPeer = p
			break
		}
	}
	node.PeersMutex.RUnlock()

	if targetPeer == nil {
		fmt.Printf("发送文件失败: 用户 %s 不在线\n", targetName)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("发送文件失败: 无法打开文件 %s: %v\n", filePath, err)
		return
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	totalChunks := (int(fileInfo.Size()) + chunkSize - 1) / chunkSize
	
	buffer := make([]byte, chunkSize)
	chunkNum := 0

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break // 文件读取完毕
			}
			fmt.Printf("发送文件失败: 读取文件时出错: %v\n", err)
			return
		}
		if bytesRead == 0 {
			break
		}

		chunkNum++
		chunkData := buffer[:bytesRead]

		chunk := FileChunk{
			Type:        "file_chunk",
			FileID:      fileID,
			ChunkNum:    chunkNum,
			TotalChunks: totalChunks,
			Data:        chunkData,
			Timestamp:   time.Now(),
		}

		// 加密 chunk Data
		if len(targetPeer.SharedKey) == 32 {
			ciphertext, nonce, err := encryptMessage([32]byte(targetPeer.SharedKey), chunkData)
			if err == nil {
				chunk.Encrypted = true
				chunk.Nonce = nonce
				chunk.Ciphertext = ciphertext
				chunk.Data = nil // 清空明文
			} else {
				fmt.Printf("加密文件块失败: %v，将尝试不加密传输\n", err)
				// 如果加密失败，保持明文传输
				chunk.Encrypted = false
				chunk.Data = chunkData
			}
		} else {
			// 密钥无效，保持明文传输
			chunk.Encrypted = false
			chunk.Data = chunkData
		}

		msg := Message{
			Type: "file_chunk",
			From: node.ID,
			To:   targetPeer.ID,
			Data: chunk,
		}

		if err := node.sendMessageToPeer(targetPeer, msg); err != nil {
			fmt.Printf("发送文件块失败: %v\n", err)
			// 标记传输失败
			node.FileTransfersMutex.Lock()
			if transfer, exists := node.FileTransfers[fileID]; exists {
				transfer.Status = "failed"
				transfer.EndTime = time.Now()
				fmt.Printf("文件传输失败: %s\n", transfer.FileName)
			}
			node.FileTransfersMutex.Unlock()
			return
		}

		// 更新进度
		node.updateTransferProgress(fileID, int64(bytesRead))
	}

	// 发送完成
	node.FileTransfersMutex.Lock()
	if transfer, exists := node.FileTransfers[fileID]; exists {
		transfer.Status = "completed"
		transfer.EndTime = time.Now()
	}
	node.FileTransfersMutex.Unlock()

	fmt.Printf("文件发送完成: %s\n", filePath)
}

// 处理文件数据块
func (node *P2PNode) handleFileChunk(chunk FileChunk) {
	node.FileTransfersMutex.Lock()
	transfer, exists := node.FileTransfers[chunk.FileID]
	if !exists {
		node.FileTransfersMutex.Unlock()
		return
	}
	node.FileTransfersMutex.Unlock()

	// 创建下载目录
	downloadDir := "downloads"
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		fmt.Printf("创建下载目录失败: %v\n", err)
		return
	}

	filePath := filepath.Join(downloadDir, transfer.FileName)

	// 以追加模式打开文件
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}
	defer file.Close()

	// 解密 Data
	var chunkData []byte
	if chunk.Encrypted && len(chunk.Nonce) > 0 && len(chunk.Ciphertext) > 0 {
		node.PeersMutex.RLock()
		senderPeer, exists := node.Peers[transfer.PeerID] // 使用存储的peer ID查找发送方
		node.PeersMutex.RUnlock()

		if exists && len(senderPeer.SharedKey) > 0 {
			plaintext, err := decryptMessage([32]byte(senderPeer.SharedKey), chunk.Ciphertext, chunk.Nonce)
			if err == nil {
				chunkData = plaintext
			} else {
				fmt.Printf("解密文件块失败: %v (文件: %s, 发送方: %s, PeerID: %s)\n",
					err, transfer.FileName, transfer.PeerName, transfer.PeerID)
				fmt.Printf("调试信息 - 密文长度: %d, Nonce长度: %d, 密钥长度: %d\n",
					len(chunk.Ciphertext), len(chunk.Nonce), len(senderPeer.SharedKey))
				return
			}
		} else {
			fmt.Printf("无密钥解密文件块 (文件: %s, PeerID: %s, Peer存在: %v, 有密钥: %v)\n",
				transfer.FileName, transfer.PeerID, exists, exists && len(senderPeer.SharedKey) > 0)

			// 列出所有可用的peers用于调试
			node.PeersMutex.RLock()
			fmt.Printf("可用Peers: ")
			for id, peer := range node.Peers {
				fmt.Printf("%s(%s, 密钥长度:%d) ", peer.Name, id, len(peer.SharedKey))
			}
			fmt.Printf("\n")
			node.PeersMutex.RUnlock()

			return
		}
	} else {
		chunkData = chunk.Data
	}

	// 写入数据
	if _, err := file.Write(chunkData); err != nil {
		fmt.Printf("写入文件块失败: %v\n", err)
		return
	}

	// 更新进度
	node.updateTransferProgress(chunk.FileID, int64(len(chunkData)))

	// 检查是否完成
	node.FileTransfersMutex.Lock()
	if transfer.Progress >= transfer.FileSize {
		transfer.Status = "completed"
		transfer.EndTime = time.Now()
		fmt.Printf("\n文件接收完成: %s，已保存到 %s 目录\n", transfer.FileName, downloadDir)
	}
	node.FileTransfersMutex.Unlock()
}

// 更新文件传输状态（计算速度和ETA）
func (node *P2PNode) updateTransferProgress(fileID string, bytesAdded int64) {
	node.FileTransfersMutex.Lock()
	defer node.FileTransfersMutex.Unlock()

	transfer, exists := node.FileTransfers[fileID]
	if !exists {
		return
	}

	// 更新进度
	transfer.Progress += bytesAdded
	now := time.Now()

	// 计算速度和ETA
	if transfer.LastUpdateTime.IsZero() {
		transfer.LastUpdateTime = transfer.StartTime
	}

	elapsed := now.Sub(transfer.LastUpdateTime).Seconds()
	if elapsed > 0 {
		// 计算瞬时速度（最近一段时间的速度）
		transfer.Speed = float64(bytesAdded) / elapsed

		// 计算ETA
		remaining := transfer.FileSize - transfer.Progress
		if transfer.Speed > 0 {
			transfer.ETA = int64(float64(remaining) / transfer.Speed)
		} else {
			transfer.ETA = -1 // 无法计算
		}
	}

	transfer.LastUpdateTime = now
	transfer.Status = "transferring"
}

// 格式化文件大小
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// 显示文件传输列表
func (node *P2PNode) showFileTransfers() {
	node.FileTransfersMutex.RLock()
	defer node.FileTransfersMutex.RUnlock()

	if len(node.FileTransfers) == 0 {
		fmt.Println("没有进行中的文件传输")
		return
	}

	fmt.Println("\n文件传输列表:")
	fmt.Println("===========================================")
	for _, transfer := range node.FileTransfers {
		progressPercent := float64(transfer.Progress) / float64(transfer.FileSize) * 100
		duration := time.Since(transfer.StartTime)
		
		fmt.Printf("文件: %s\n", transfer.FileName)
		fmt.Printf("大小: %s\n", formatFileSize(transfer.FileSize))
		fmt.Printf("进度: %.1f%% (%s/%s)\n", 
			progressPercent, 
			formatFileSize(transfer.Progress), 
			formatFileSize(transfer.FileSize))
		fmt.Printf("状态: %s\n", transfer.Status)
		fmt.Printf("方向: %s\n", transfer.Direction)
		fmt.Printf("对方: %s\n", transfer.PeerName)
		fmt.Printf("时长: %v\n", duration.Round(time.Second))
		fmt.Println("-------------------------------------------")
	}
}
