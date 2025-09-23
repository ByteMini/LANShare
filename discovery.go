package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

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
	// 检查是否是已知节点
	node.PeersMutex.RLock()
	_, exists := node.Peers[msg.ID]
	node.PeersMutex.RUnlock()
	if exists {
		return // 如果是已知节点，则忽略
	}

	switch msg.Type {
	case "announce":
		fmt.Printf("发现新节点: %s (%s:%d)\n", msg.Name, msg.IP, msg.Port)
		go node.connectToPeer(msg.IP, msg.Port, msg.ID, msg.Name)
		node.sendDiscoveryResponse(remoteAddr.IP.String())
		
	case "response":
		// 对于响应消息，也只在对方是未知节点时才尝试连接
		fmt.Printf("收到来自 %s 的响应，尝试连接...\n", msg.Name)
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
