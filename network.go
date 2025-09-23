package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

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
