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
        // 添加震动效果
        input.style.animation = 'shake 0.3s ease-in-out';
        setTimeout(() => {
            input.style.animation = '';
        }, 300);
        return;
    }
    
    // 显示发送状态
    const button = document.getElementById('sendButton');
    const buttonText = button.querySelector('.button-text');
    const loadingIndicator = button.querySelector('.loading-indicator');
    
    buttonText.style.display = 'none';
    loadingIndicator.style.display = 'inline';
    button.disabled = true;
    
    // 添加发送动画
    button.style.transform = 'scale(0.95)';
    
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
            // 添加发送成功反馈
            button.style.background = 'var(--success-color)';
            setTimeout(() => {
                button.style.background = 'var(--primary-color)';
            }, 200);
        } else {
            throw new Error('发送失败');
        }
    })
    .catch(error => {
        console.error('发送消息失败:', error);
        showNotification('发送消息失败，请重试', 'error');
    })
    .finally(() => {
        buttonText.style.display = 'inline';
        loadingIndicator.style.display = 'none';
        button.disabled = false;
        button.style.transform = '';
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
    const statusDiv = document.getElementById('statusIndicator');
    
    if (connected !== isConnected) {
        isConnected = connected;
        
        if (connected) {
            statusDiv.textContent = '状态: 已连接 ✅';
            statusDiv.className = 'status';
            // 添加连接成功动画
            statusDiv.style.animation = 'pulse 0.5s ease-in-out';
            setTimeout(() => {
                statusDiv.style.animation = '';
            }, 500);
        } else {
            statusDiv.textContent = '状态: 连接断开 ❌';
            statusDiv.className = 'status offline';
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

// 文件传输相关功能
let selectedFile = null;

// 初始化文件传输功能
function initFileTransfer() {
    const fileInput = document.getElementById('fileInput');
    const sendFileBtn = document.getElementById('sendFileBtn');
    const targetUserSelect = document.getElementById('fileTargetUser');
    
    // 文件选择事件
    fileInput.addEventListener('change', function(event) {
        const fileNameDisplay = document.getElementById('fileNameDisplay');
        if (event.target.files.length > 0) {
            selectedFile = event.target.files[0];
            fileNameDisplay.textContent = selectedFile.name;
        } else {
            selectedFile = null;
            fileNameDisplay.textContent = '';
        }
        updateSendFileButton();
    });
    
    // 用户选择变化事件
    targetUserSelect.addEventListener('change', updateSendFileButton);
    
    // 初始化用户选择框
    updateUserSelect();
}

// 更新用户选择框
function updateUserSelect() {
    const targetUserSelect = document.getElementById('fileTargetUser');
    
    // 清空现有选项（保留"选择用户"选项）
    while (targetUserSelect.options.length > 1) {
        targetUserSelect.remove(1);
    }
    
    // 从服务器获取最新的用户列表
    fetch('/users')
        .then(response => {
            if (!response.ok) {
                throw new Error('获取用户列表失败');
            }
            return response.json();
        })
        .then(data => {
            const users = data.users || [];
            
            // 添加在线用户（排除自己）
            users.forEach(user => {
                if (!user.includes('(自己)')) {
                    const username = user.split(' ')[0]; // 提取用户名
                    const option = document.createElement('option');
                    option.value = username;
                    option.textContent = username;
                    targetUserSelect.appendChild(option);
                }
            });
        })
        .catch(error => {
            console.error('更新用户选择框失败:', error);
        });
}

// 更新发送文件按钮状态
function updateSendFileButton() {
    const sendFileBtn = document.getElementById('sendFileBtn');
    const targetUserSelect = document.getElementById('fileTargetUser');
    
    sendFileBtn.disabled = !selectedFile || !targetUserSelect.value;
}

// 发送文件
function sendFile() {
    if (!selectedFile) {
        showNotification('请先选择文件', 'error');
        return;
    }
    
    const targetUserSelect = document.getElementById('fileTargetUser');
    const targetUser = targetUserSelect.value;
    
    if (!targetUser) {
        showNotification('请选择目标用户', 'error');
        return;
    }
    
    // 检查文件大小（100MB限制）
    if (selectedFile.size > 100 * 1024 * 1024) {
        showNotification('文件大小超过限制（最大100MB）', 'error');
        return;
    }
    
    const sendFileBtn = document.getElementById('sendFileBtn');
    sendFileBtn.disabled = true;
    sendFileBtn.textContent = '发送中...';
    
    // 创建FormData对象
    const formData = new FormData();
    formData.append('file', selectedFile);
    formData.append('targetName', targetUser);
    
    fetch('/sendfile', {
        method: 'POST',
        body: formData
    })
    .then(response => {
        if (response.ok) {
            showNotification('文件传输请求已发送', 'success');
            // 重置文件选择
            document.getElementById('fileInput').value = '';
            selectedFile = null;
            updateSendFileButton();
        } else {
            throw new Error('文件发送失败');
        }
    })
    .catch(error => {
        console.error('发送文件失败:', error);
        showNotification('文件发送失败，请重试', 'error');
    })
    .finally(() => {
        sendFileBtn.disabled = false;
        sendFileBtn.textContent = '发送文件';
    });
}

// 加载文件传输列表
function loadFileTransfers() {
    fetch('/filetransfers')
        .then(response => {
            if (!response.ok) {
                throw new Error('获取文件传输列表失败');
            }
            return response.json();
        })
        .then(data => {
            displayFileTransfers(data.transfers || []);
        })
        .catch(error => {
            console.error('加载文件传输列表失败:', error);
        });
}

// 用一个集合来跟踪已经处理过的待处理文件请求，防止重复弹窗
let shownPendingTransfers = new Set();

// 显示文件传输列表
function displayFileTransfers(transfers) {
    // 检查是否有待处理的接收文件
    const pendingReceive = transfers.find(t => t.direction === 'receive' && t.status === 'pending');
    
    if (pendingReceive && !shownPendingTransfers.has(pendingReceive.fileId)) {
        showFileConfirmDialog(pendingReceive);
        shownPendingTransfers.add(pendingReceive.fileId);
    }
}

// 显示文件确认弹窗
function showFileConfirmDialog(transfer) {
    const dialog = document.getElementById('file-confirm-dialog');
    document.getElementById('dialog-filename').textContent = transfer.fileName;
    document.getElementById('dialog-filesize').textContent = formatBytes(transfer.fileSize);
    document.getElementById('dialog-sender').textContent = transfer.peerName;

    const acceptBtn = document.getElementById('dialog-accept-btn');
    const rejectBtn = document.getElementById('dialog-reject-btn');

    const onAccept = () => {
        sendFileResponse(transfer.fileId, true);
        hideDialog();
    };

    const onReject = () => {
        sendFileResponse(transfer.fileId, false);
        hideDialog();
    };

    acceptBtn.onclick = onAccept;
    rejectBtn.onclick = onReject;

    dialog.style.display = 'flex';
    setTimeout(() => dialog.classList.add('visible'), 10);

    function hideDialog() {
        dialog.classList.remove('visible');
        setTimeout(() => {
            dialog.style.display = 'none';
            // 清理事件监听器
            acceptBtn.onclick = null;
            rejectBtn.onclick = null;
        }, 300);
    }
}

// 发送文件传输响应
function sendFileResponse(fileId, accepted) {
    fetch('/fileresponse', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fileId, accepted })
    })
    .then(response => {
        if (!response.ok) throw new Error('响应失败');
        showNotification(`文件传输已${accepted ? '接受' : '拒绝'}`, 'success');
    })
    .catch(error => {
        console.error('发送文件响应失败:', error);
        showNotification('发送响应失败', 'error');
    });
}

// 格式化字节大小
function formatBytes(bytes, decimals = 2) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}


// 初始化函数
function init() {
    // 设置输入框焦点
    document.getElementById('messageInput').focus();
    
    // 初始加载数据
    loadMessages();
    loadUsers();
    loadFileTransfers();
    
    // 设置定时器
    setInterval(loadMessages, 1000);
    setInterval(loadUsers, 2000);
    setInterval(loadFileTransfers, 3000);
    setInterval(updateUserSelect, 5000); // 每5秒更新用户选择框
    
    // 初始化文件传输功能
    initFileTransfer();
    
    console.log('LANShare P2P Web客户端已初始化');
}

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
