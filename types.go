package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// P2PNode结构体 - 主节点结构
type P2PNode struct {
	LocalIP   string
	LocalPort int
	Name      string
	ID        string
	Address   string // 新增：本地地址 "IP:port"

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
	WebServer    *http.Server

	// 文件传输相关
	FileTransfers     map[string]*FileTransferStatus
	FileTransfersMutex sync.RWMutex
	ACLs              map[string]map[string]bool
	ACLMutex          sync.RWMutex
}

// Peer结构体 - 对等节点结构
type Peer struct {
	ID        string
	Name      string
	Address   string
	Conn      net.Conn
	IsActive  bool
	LastSeen  time.Time
	SharedKey []byte // 新增：共享密钥
	PrivateKey [32]byte // 临时私钥
	PublicKey  [32]byte // 临时公钥
}

// Message结构体 - 通用消息结构
type Message struct {
	Type        string      `json:"type"`
	From        string      `json:"from"`
	To          string      `json:"to"`
	Content     string      `json:"content"`
	Timestamp   time.Time   `json:"timestamp"`
	Data        interface{} `json:"data,omitempty"`
	Encrypted   bool        `json:"encrypted"`
	Nonce       []byte      `json:"nonce,omitempty"`
	Ciphertext  []byte      `json:"ciphertext,omitempty"`
	SenderPubKey []byte     `json:"sender_pubkey,omitempty"`
}

// DiscoveryMessage结构体 - 服务发现消息结构
type DiscoveryMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ChatMessage结构体 - 聊天消息结构
type ChatMessage struct {
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"` // "all" for public, or username for private
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsOwn     bool      `json:"isOwn"`
	IsPrivate bool      `json:"isPrivate"`
}

// FileTransferRequest结构体 - 文件传输请求
type FileTransferRequest struct {
	Type        string    `json:"type"`
	FileID      string    `json:"fileId"`
	FileName    string    `json:"fileName"`
	FileSize    int64     `json:"fileSize"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Timestamp   time.Time `json:"timestamp"`
}

// FileTransferResponse结构体 - 文件传输响应
type FileTransferResponse struct {
	Type      string    `json:"type"`
	FileID    string    `json:"fileId"`
	Accepted  bool      `json:"accepted"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// FileChunk结构体 - 文件数据块
type FileChunk struct {
	Type        string    `json:"type"`
	FileID      string    `json:"fileId"`
	ChunkNum    int       `json:"chunkNum"`
	TotalChunks int       `json:"totalChunks"`
	Data        []byte    `json:"data,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Encrypted   bool      `json:"encrypted"`
	Nonce       []byte    `json:"nonce,omitempty"`
	Ciphertext  []byte    `json:"ciphertext,omitempty"`
}

// ECDHKeyPair结构体 - ECDH密钥对
type ECDHKeyPair struct {
	PrivateKey [32]byte
	PublicKey  [32]byte
}

type FileTransferStatus struct {
	FileID      string    `json:"fileId"`
	FileName    string    `json:"fileName"`
	FilePath    string    `json:"-"` // 发送方的文件完整路径，不进行json序列化
	FileSize    int64     `json:"fileSize"`
	Progress    int64     `json:"progress"`
	Status      string    `json:"status"` // pending, transferring, completed, failed
	Direction   string    `json:"direction"` // send, receive
	PeerName    string    `json:"peerName"`
	FromID      string    `json:"-"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
}
