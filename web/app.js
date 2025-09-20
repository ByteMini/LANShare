// LANShare P2P Web客户端JavaScript代码

// 全局变量
let isConnected = true;
let lastMessageCount = 0;
let lastUserCount = 0;

// 初始化函数
function init() {
    // 设置输入框焦点
    document.getElementById('messageInput').focus();
    
    // 初始加载数据
    loadMessages();
    loadUsers();
    
    // 设置定时器
    setInterval(loadMessages, 1000);
    setInterval(loadUsers, 2000);
    
    console.log('LANShare P2P Web客户端已初始化');
}

// 处理键盘事件
function handleKeyPress(event) {
    if (event.key === 'Enter') {
        sendMessage();
    }
}

// 发送消息
function sendMessage() {
    const input = document.getElementById('messageInput');
    const message = input.value.trim();
    
    if (message === '') {
        return;
    }
    
    // 显示发送状态
    const button = document.querySelector('.input-group button');
    const originalText = button.textContent;
    button.textContent = '发送中...';
    button.disabled = true;
    
    fetch('/send', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({message: message})
    })
    .then(response => {
        if (response.ok) {
            input.value = '';
            input.focus();
        } else {
            throw new Error('发送失败');
        }
    })
    .catch(error => {
        console.error('发送消息失败:', error);
        showNotification('发送消息失败，请重试', 'error');
    })
    .finally(() => {
        button.textContent = originalText;
        button.disabled = false;
    });
}

// 加载消息
function loadMessages() {
    fetch('/messages')
        .then(response => {
            if (!response.ok) {
                throw new Error('获取消息失败');
            }
            return response.json();
        })
        .then(data => {
            updateConnectionStatus(true);
            displayMessages(data.messages || []);
        })
        .catch(error => {
            console.error('加载消息失败:', error);
            updateConnectionStatus(false);
        });
}

// 显示消息
function displayMessages(messages) {
    const messagesDiv = document.getElementById('messages');
    const shouldScroll = isScrolledToBottom(messagesDiv);
    
    // 检查是否有新消息
    if (messages.length !== lastMessageCount) {
        messagesDiv.innerHTML = '';
        
        messages.forEach(msg => {
            const messageDiv = createMessageElement(msg);
            messagesDiv.appendChild(messageDiv);
        });
        
        lastMessageCount = messages.length;
        
        // 如果之前滚动到底部，继续保持在底部
        if (shouldScroll) {
            scrollToBottom(messagesDiv);
        }
    }
}

// 创建消息元素
function createMessageElement(msg) {
    const messageDiv = document.createElement('div');
    messageDiv.className = 'message ' + (msg.isOwn ? 'own' : 'other') + (msg.isPrivate ? ' private' : '');
    
    const senderDiv = document.createElement('div');
    senderDiv.className = 'message-sender';
    senderDiv.textContent = msg.sender + (msg.isPrivate ? ' (私聊)' : '');
    
    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';
    contentDiv.textContent = msg.content;
    
    const timeDiv = document.createElement('div');
    timeDiv.className = 'message-time';
    timeDiv.textContent = formatTime(new Date(msg.timestamp));
    
    messageDiv.appendChild(senderDiv);
    messageDiv.appendChild(contentDiv);
    messageDiv.appendChild(timeDiv);
    
    return messageDiv;
}

// 加载用户列表
function loadUsers() {
    fetch('/users')
        .then(response => {
            if (!response.ok) {
                throw new Error('获取用户列表失败');
            }
            return response.json();
        })
        .then(data => {
            updateConnectionStatus(true);
            displayUsers(data.users || []);
        })
        .catch(error => {
            console.error('加载用户列表失败:', error);
            updateConnectionStatus(false);
        });
}

// 显示用户列表
function displayUsers(users) {
    const usersList = document.getElementById('usersList');
    
    // 检查是否有变化
    if (users.length !== lastUserCount) {
        usersList.innerHTML = '';
        
        users.forEach(user => {
            const li = document.createElement('li');
            li.textContent = user;
            
            if (user.includes('(自己)')) {
                li.className = 'own';
            } else {
                li.onclick = () => {
                    const username = user.split(' ')[0]; // 提取用户名
                    startPrivateChat(username);
                };
                li.title = '点击发起私聊';
            }
            
            usersList.appendChild(li);
        });
        
        lastUserCount = users.length;
    }
}

// 开始私聊
function startPrivateChat(username) {
    const input = document.getElementById('messageInput');
    input.value = '/to ' + username + ' ';
    input.focus();
    
    // 将光标移到末尾
    input.setSelectionRange(input.value.length, input.value.length);
}

// 更新连接状态
function updateConnectionStatus(connected) {
    const statusDiv = document.querySelector('.status');
    
    if (connected !== isConnected) {
        isConnected = connected;
        
        if (connected) {
            statusDiv.textContent = '状态: 已连接';
            statusDiv.className = 'status';
            statusDiv.style.background = '#d4edda';
            statusDiv.style.color = '#155724';
            statusDiv.style.borderColor = '#c3e6cb';
        } else {
            statusDiv.textContent = '状态: 连接断开';
            statusDiv.className = 'status';
            statusDiv.style.background = '#f8d7da';
            statusDiv.style.color = '#721c24';
            statusDiv.style.borderColor = '#f5c6cb';
        }
    }
}

// 显示通知
function showNotification(message, type = 'info') {
    // 创建通知元素
    const notification = document.createElement('div');
    notification.className = `notification ${type}`;
    notification.textContent = message;
    
    // 设置样式
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 12px 20px;
        border-radius: 4px;
        color: white;
        font-weight: bold;
        z-index: 1000;
        animation: slideIn 0.3s ease-out;
    `;
    
    // 根据类型设置背景色
    switch (type) {
        case 'error':
            notification.style.background = '#f44336';
            break;
        case 'success':
            notification.style.background = '#4caf50';
            break;
        case 'warning':
            notification.style.background = '#ff9800';
            break;
        default:
            notification.style.background = '#2196f3';
    }
    
    document.body.appendChild(notification);
    
    // 3秒后自动移除
    setTimeout(() => {
        notification.style.animation = 'slideOut 0.3s ease-in';
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, 300);
    }, 3000);
}

// 工具函数：检查是否滚动到底部
function isScrolledToBottom(element) {
    return element.scrollHeight - element.clientHeight <= element.scrollTop + 1;
}

// 工具函数：滚动到底部
function scrollToBottom(element) {
    element.scrollTop = element.scrollHeight;
}

// 工具函数：格式化时间
function formatTime(date) {
    return date.toLocaleTimeString('zh-CN', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

// 工具函数：转义HTML
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// 添加CSS动画
const style = document.createElement('style');
style.textContent = `
    @keyframes slideIn {
        from { transform: translateX(100%); opacity: 0; }
        to { transform: translateX(0); opacity: 1; }
    }
    
    @keyframes slideOut {
        from { transform: translateX(0); opacity: 1; }
        to { transform: translateX(100%); opacity: 0; }
    }
    
    .notification {
        box-shadow: 0 4px 12px rgba(0,0,0,0.15);
    }
`;
document.head.appendChild(style);

// 页面加载完成后初始化
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}

// 页面卸载时清理
window.addEventListener('beforeunload', function() {
    console.log('LANShare P2P Web客户端正在关闭');
});

// 处理网络错误
window.addEventListener('online', function() {
    showNotification('网络连接已恢复', 'success');
    loadMessages();
    loadUsers();
});

window.addEventListener('offline', function() {
    showNotification('网络连接已断开', 'warning');
    updateConnectionStatus(false);
});

// 导出函数供HTML使用
window.handleKeyPress = handleKeyPress;
window.sendMessage = sendMessage;
