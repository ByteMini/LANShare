package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
	"golang.org/x/crypto/curve25519"
)

// ECDH密钥生成
func generateECDHKeyPair() (privateKey [32]byte, publicKey [32]byte, err error) {
	_, err = rand.Read(privateKey[:])
	if err != nil {
		return
	}
	var publicKeyBytes [32]byte
	curve25519.ScalarBaseMult(&publicKeyBytes, &privateKey)
	copy(publicKey[:], publicKeyBytes[:])
	return
}

// 派生共享密钥
func deriveSharedKey(privateKey [32]byte, remotePubKey [32]byte) [32]byte {
	var shared [32]byte
	var remotePub [32]byte
	copy(remotePub[:], remotePubKey[:])
	curve25519.ScalarMult(&shared, &privateKey, &remotePub)
	return shared
}

// 加密消息
func encryptMessage(key [32]byte, plaintext []byte) (ciphertext []byte, nonce []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return
	}
	nonce = make([]byte, aesGCM.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return
	}
	ciphertext = aesGCM.Seal(nil, nonce, plaintext, nil)
	return
}

// 解密消息
func decryptMessage(key [32]byte, ciphertext []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aesGCM.Open(nil, nonce, ciphertext, nil)
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

	// 生成密钥对
	privateKey, publicKey, err := generateECDHKeyPair()
	if err != nil {
		fmt.Printf("密钥生成失败: %v\n", err)
		conn.Close()
		return
	}
	peer.PrivateKey = privateKey
	peer.PublicKey = publicKey

	handshakeMsg := Message{
		Type:        "handshake",
		From:        node.ID,
		Content:     node.Name,
		Timestamp:   time.Now(),
		SenderPubKey: publicKey[:],
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

	// 提取远程公钥
	var remotePubKey [32]byte
	if len(handshakeMsg.SenderPubKey) == 32 {
		copy(remotePubKey[:], handshakeMsg.SenderPubKey)
	} else {
		conn.Close()
		return
	}

	peer := &Peer{
		Conn: conn,
	}

	// 生成自己的密钥对
	privateKey, publicKey, err := generateECDHKeyPair()
	if err != nil {
		fmt.Printf("密钥生成失败: %v\n", err)
		conn.Close()
		return
	}
	peer.PrivateKey = privateKey
	peer.PublicKey = publicKey

	// 派生共享密钥
	shared := deriveSharedKey(privateKey, remotePubKey)
	peer.SharedKey = shared[:]

	peer.ID = handshakeMsg.From
	peer.Name = handshakeMsg.Content
	peer.Address = conn.RemoteAddr().String()
	peer.IsActive = true
	peer.LastSeen = time.Now()

	node.PeersMutex.Lock()
	node.Peers[peer.ID] = peer
	node.PeersMutex.Unlock()

	fmt.Printf("接受来自节点的连接: %s (%s)\n", peer.Name, peer.Address)

	// 发送握手响应
	responseMsg := Message{
		Type:        "handshake_response",
		From:        node.ID,
		Content:     node.Name,
		Timestamp:   time.Now(),
		SenderPubKey: publicKey[:],
	}
	node.sendMessageToPeer(peer, responseMsg)

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
			// 解密聊天消息
			var content string
			if msg.Encrypted && len(msg.Nonce) > 0 && len(msg.Ciphertext) > 0 {
				// 查找发送方 peer 以获取共享密钥
				node.PeersMutex.RLock()
				senderPeer, exists := node.Peers[msg.From]
				node.PeersMutex.RUnlock()
				if exists && len(senderPeer.SharedKey) > 0 {
					plaintext, err := decryptMessage([32]byte(senderPeer.SharedKey), msg.Ciphertext, msg.Nonce)
					if err == nil {
						content = string(plaintext)
					} else {
						fmt.Printf("解密失败: %v\n", err)
						content = "[解密失败]"
					}
				} else {
					content = "[无密钥]"
				}
			} else {
				content = msg.Content
			}

			senderPeer, exists := node.Peers[msg.From]
			if !exists {
				continue
			}
			senderName := node.getPeerName(msg.From)
			if msg.To == "" || msg.To == "all" {
				// 公聊消息
				if node.isBlocked(senderPeer.Address) {
					continue
				}
				node.addChatMessage(senderName, "all", content, false, false)
			} else if msg.To == node.ID {
				// 私聊消息
				if node.isBlocked(senderPeer.Address) {
					continue
				}
				node.addChatMessage(senderName, node.Name, content, false, true)
			}
		case "handshake":
			// 握手消息已在连接处理中处理
		case "handshake_response":
			// 握手响应 - 派生共享密钥
			node.PeersMutex.Lock()
			var peer *Peer
			var exists bool
			peer, exists = node.Peers[msg.From]
			if exists && len(msg.SenderPubKey) == 32 {
				var remotePub [32]byte
				copy(remotePub[:], msg.SenderPubKey)
				shared := deriveSharedKey(peer.PrivateKey, remotePub)
				peer.SharedKey = shared[:]
				fmt.Printf("与 %s 建立加密连接\n", peer.Name)
			}
			node.PeersMutex.Unlock()
		case "file_request":
			// 文件传输请求
			if data, ok := msg.Data.(map[string]interface{}); ok {
				jsonData, _ := json.Marshal(data)
				var request FileTransferRequest
				if err := json.Unmarshal(jsonData, &request); err == nil {
					node.handleFileTransferRequest(request)
				}
			}
		case "file_response":
			// 文件传输响应
			if data, ok := msg.Data.(map[string]interface{}); ok {
				jsonData, _ := json.Marshal(data)
				var response FileTransferResponse
				if err := json.Unmarshal(jsonData, &response); err == nil {
					node.handleFileTransferResponse(response)
				}
			}
		case "file_chunk":
			// 文件数据块
			if data, ok := msg.Data.(map[string]interface{}); ok {
				jsonData, _ := json.Marshal(data)
				var chunk FileChunk
				if err := json.Unmarshal(jsonData, &chunk); err == nil {
					node.handleFileChunk(chunk)
				}
			}
		case "update_name":
			// 用户名更新
			node.PeersMutex.Lock()
			if peer, exists := node.Peers[msg.From]; exists {
				oldName := peer.Name
				peer.Name = msg.Content
				fmt.Printf("用户 %s 已更名为 %s\n", oldName, peer.Name)
			}
			node.PeersMutex.Unlock()
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
	if len(peer.SharedKey) > 0 && msg.Type == "chat" {
		// 加密聊天消息
		plaintext := []byte(msg.Content)
		ciphertext, nonce, err := encryptMessage([32]byte(peer.SharedKey), plaintext)
		if err != nil {
			return err
		}
		msg.Encrypted = true
		msg.Nonce = nonce
		msg.Ciphertext = ciphertext
		msg.Content = "" // 清空明文
	}

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

	var choice int
	for {
		fmt.Print("请选择网络接口 (1-" + strconv.Itoa(len(availableIPs)) + "): ")
		_, err := fmt.Scanln(&choice)
		if err == nil && choice >= 1 && choice <= len(availableIPs) {
			break
		}
		fmt.Println("无效选择，请重试。")
	}

	selectedIP := availableIPs[choice-1]
	selectedName := interfaceNames[choice-1]
	fmt.Printf("使用网络接口: %s (%s)\n", selectedName, selectedIP)
	return selectedIP
}
