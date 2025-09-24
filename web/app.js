// LANShare P2P Web客户端JavaScript代码

// =================================
// 全局状态变量
// =================================
let localUsername = '';
let currentChat = { id: 'all', name: '公聊' };
let allMessages = [];
let shownPendingTransfers = new Set();

// =================================
// 初始化
// =================================
function init() {
    document.getElementById('messageInput').focus();
    
    // 初始加载数据
    loadUsers();
    loadMessages();
    loadFileTransfers();
    
    // 设置定时器
    setInterval(loadMessages, 2000); // 消息可以稍微慢一点
    setInterval(loadUsers, 3000);    // 用户列表不需要太频繁
    setInterval(loadFileTransfers, 3000);
    setInterval(checkConnection, 5000); // 添加连接检查
    
    // 初始化功能
    initFileTransfer();
    initChatSwitching();
    initEmojiPicker();
    
    console.log('LANShare P2P Web客户端已初始化');
}

// =================================
// 聊天上下文切换
// =================================
function initChatSwitching() {
    const publicChatBtn = document.getElementById('public-chat-btn');
    publicChatBtn.addEventListener('click', () => switchChat(publicChatBtn));
}

function switchChat(targetElement) {
    // 更新全局状态
    currentChat.id = targetElement.dataset.chatId;
    currentChat.name = targetElement.dataset.chatName;

    // 更新UI高亮状态
    document.querySelectorAll('.users-list li').forEach(li => li.classList.remove('active'));
    targetElement.classList.add('active');

    // 更新消息输入框
    const input = document.getElementById('messageInput');
    input.value = ''; // 始终清空输入框
    if (currentChat.id === 'all') {
        input.placeholder = '输入公共消息...';
    } else {
        input.placeholder = `私聊 ${currentChat.name}...`;
    }
    input.focus();

    // 重新渲染消息列表
    displayMessages();
}

// =================================
// 消息处理
// =================================
function handleKeyPress(event) {
    if (event.key === 'Enter') {
        sendMessage();
    }
}

function sendMessage() {
    const input = document.getElementById('messageInput');
    let message = input.value.trim();
    
    if (message === '') {
        input.style.animation = 'shake 0.3s ease-in-out';
        setTimeout(() => { input.style.animation = ''; }, 300);
        return;
    }

    // 如果是私聊，隐式地添加命令前缀
    if (currentChat.id !== 'all') {
        message = `/to ${currentChat.name} ${message}`;
    }
    
    fetch('/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: message })
    })
    .then(response => {
        if (response.ok) {
            input.value = ''; // 总是清空输入框
            input.focus();
        } else {
            throw new Error('发送失败');
        }
    })
    .catch(error => {
        console.error('发送消息失败:', error);
        showNotification('发送消息失败，请重试', 'error');
    });
}

function loadMessages() {
    fetch('/messages')
        .then(response => response.json())
        .then(data => {
            if (data.messages.length !== allMessages.length) {
                allMessages = data.messages || [];
                displayMessages(); // 数据变化时才重新渲染
            }
        })
        .catch(error => console.error('加载消息失败:', error));
}

function displayMessages() {
    const messagesDiv = document.getElementById('messages');
    const shouldScroll = isScrolledToBottom(messagesDiv);

    const filteredMessages = allMessages.filter(msg => {
        if (currentChat.id === 'all') {
            return !msg.isPrivate;
        } else {
            // 私聊消息：发送者是对方且接收者是我，或者发送者是我且接收者是对方
            return msg.isPrivate && 
                   ((msg.sender === currentChat.name && msg.recipient === localUsername) || 
                    (msg.isOwn && msg.recipient === currentChat.name));
        }
    });

    messagesDiv.innerHTML = '';
    if (filteredMessages.length === 0) {
        const placeholder = document.createElement('div');
        placeholder.className = 'message-placeholder';
        placeholder.textContent = `开始与 ${currentChat.name} 对话吧！`;
        messagesDiv.appendChild(placeholder);
    } else {
        filteredMessages.forEach(msg => {
            const messageDiv = createMessageElement(msg);
            messagesDiv.appendChild(messageDiv);
        });
    }
    
    if (shouldScroll) {
        scrollToBottom(messagesDiv);
    }
}

function createMessageElement(msg) {
    const messageDiv = document.createElement('div');
    messageDiv.className = 'message ' + (msg.isOwn ? 'own' : 'other') + (msg.isPrivate ? ' private' : '');

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    if (msg.content.startsWith('emoji:')) {
        const emojiId = msg.content.split(':')[1];
        const emoji = emojis.find(e => e.id === emojiId);
        if (emoji) {
            // Telegram风格的大表情显示
            const emojiContainer = document.createElement('div');
            emojiContainer.className = 'emoji-message';
            emojiContainer.innerHTML = `<span class="emoji-large">${emoji.emoji}</span>`;
            contentDiv.appendChild(emojiContainer);
        } else {
            contentDiv.textContent = msg.content; // 如果找不到表情，则显示原始文本
        }
    } else {
        contentDiv.textContent = msg.content;
    }
    
    const timeDiv = document.createElement('div');
    timeDiv.className = 'message-time';
    timeDiv.textContent = formatTime(new Date(msg.timestamp));
    
    messageDiv.appendChild(contentDiv);
    messageDiv.appendChild(timeDiv);
    
    return messageDiv;
}

// =================================
// 用户列表处理
// =================================
function loadUsers() {
    fetch('/users')
        .then(response => response.json())
        .then(data => {
            displayUsers(data.users || []);
        })
        .catch(error => console.error('加载用户列表失败:', error));
}

function displayUsers(users) {
    const usersList = document.getElementById('usersList');
    const existingUsers = new Set([...usersList.querySelectorAll('li[data-chat-id]')].map(li => li.dataset.chatId));
    existingUsers.delete('all'); // 公聊频道不在此处管理

    const newUsers = new Set();

    // 提取自己的用户名
    const selfUser = users.find(u => u.includes('(自己)'));
    if (selfUser) {
        localUsername = selfUser.replace(' (自己)', '').trim();
    }

    users.forEach(user => {
        if (!user.includes('(自己)')) {
            const username = user.split(' ')[0];
            newUsers.add(username);
            
            if (!existingUsers.has(username)) {
                // 添加新用户
                const li = document.createElement('li');
                li.dataset.chatId = username;
                li.dataset.chatName = username;
                li.textContent = `👤 ${username}`;
                li.onclick = () => switchChat(li);
                usersList.appendChild(li);
            }
        }
    });

    // 移除已离线的用户
    existingUsers.forEach(oldUser => {
        if (!newUsers.has(oldUser)) {
            const userElement = usersList.querySelector(`li[data-chat-id="${oldUser}"]`);
            if (userElement) {
                userElement.remove();
            }
        }
    });
}

// =================================
// 文件传输
// =================================
let selectedFile = null;

function initFileTransfer() {
    const fileInput = document.getElementById('fileInput');
    const fileControls = document.getElementById('file-transfer-controls');
    const fileNameDisplay = document.getElementById('fileNameDisplay');

    fileInput.addEventListener('change', function(event) {
        if (event.target.files.length > 0) {
            selectedFile = event.target.files[0];
            fileNameDisplay.textContent = selectedFile.name;
            fileControls.style.display = 'flex';
        } else {
            cancelFileSelection();
        }
        updateSendFileButton();
    });
    
    document.getElementById('fileTargetUser').addEventListener('change', updateSendFileButton);
    updateUserSelect();
    setInterval(updateUserSelect, 5000);
}

function updateUserSelect() {
    const targetUserSelect = document.getElementById('fileTargetUser');
    const currentSelection = targetUserSelect.value;
    
    fetch('/users')
        .then(response => response.json())
        .then(data => {
            const users = data.users.filter(u => !u.includes('(自己)')).map(u => u.split(' ')[0]);
            
            // 清空
            while (targetUserSelect.options.length > 1) {
                targetUserSelect.remove(1);
            }

            // 填充
            users.forEach(username => {
                const option = document.createElement('option');
                option.value = username;
                option.textContent = username;
                targetUserSelect.appendChild(option);
            });
            targetUserSelect.value = currentSelection;
        });
}

function updateSendFileButton() {
    const sendFileBtn = document.getElementById('sendFileBtn');
    const targetUserSelect = document.getElementById('fileTargetUser');
    sendFileBtn.disabled = !selectedFile || !targetUserSelect.value;
}

function sendFile() {
    if (!selectedFile || !document.getElementById('fileTargetUser').value) {
        showNotification('请选择文件和目标用户', 'error');
        return;
    }
    
    const targetUser = document.getElementById('fileTargetUser').value;
    const formData = new FormData();
    formData.append('file', selectedFile);
    formData.append('targetName', targetUser);
    
    fetch('/sendfile', { method: 'POST', body: formData })
        .then(response => {
            if (response.ok) {
                showNotification('文件传输请求已发送', 'success');
                cancelFileSelection();
            } else {
                throw new Error('文件发送失败');
            }
        })
        .catch(error => showNotification(error.message, 'error'));
}

function cancelFileSelection() {
    const fileInput = document.getElementById('fileInput');
    const fileControls = document.getElementById('file-transfer-controls');
    
    selectedFile = null;
    fileInput.value = ''; // 重置文件输入
    fileControls.style.display = 'none';
    document.getElementById('fileNameDisplay').textContent = '';
    updateSendFileButton();
}

function loadFileTransfers() {
    fetch('/filetransfers')
        .then(response => response.json())
        .then(data => {
            const pendingReceive = (data.transfers || []).find(t => t.direction === 'receive' && t.status === 'pending');
            if (pendingReceive && !shownPendingTransfers.has(pendingReceive.fileId)) {
                showFileConfirmDialog(pendingReceive);
                shownPendingTransfers.add(pendingReceive.fileId);
            }
        })
        .catch(error => console.error('加载文件传输列表失败:', error));
}

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
            acceptBtn.onclick = null;
            rejectBtn.onclick = null;
        }, 300);
    }
}

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
    .catch(error => showNotification('发送响应失败', 'error'));
}

// =================================
// 表情功能 - Telegram风格
// =================================
const emojis = [
    { id: 'smile', emoji: '😊', name: '微笑' },
    { id: 'love', emoji: '😍', name: '爱心眼' },
    { id: 'laugh', emoji: '😂', name: '大笑' },
    { id: 'wow', emoji: '😮', name: '惊讶' },
    { id: 'cry', emoji: '😢', name: '哭泣' },
    { id: 'angry', emoji: '😠', name: '生气' },
    { id: 'cool', emoji: '😎', name: '酷' },
    { id: 'wink', emoji: '😉', name: '眨眼' },
    { id: 'kiss', emoji: '😘', name: '飞吻' },
    { id: 'thinking', emoji: '🤔', name: '思考' },
    { id: 'thumbsup', emoji: '👍', name: '点赞' },
    { id: 'thumbsdown', emoji: '👎', name: '点踩' },
    { id: 'clap', emoji: '👏', name: '鼓掌' },
    { id: 'fire', emoji: '🔥', name: '火' },
    { id: 'heart', emoji: '❤️', name: '红心' },
    { id: 'party', emoji: '🎉', name: '庆祝' },
    { id: 'rocket', emoji: '🚀', name: '火箭' },
    { id: 'star', emoji: '⭐', name: '星星' }
];

function initEmojiPicker() {
    const emojiButton = document.getElementById('emoji-button');
    const emojiPicker = document.getElementById('emoji-picker');

    emojiButton.addEventListener('click', (e) => {
        e.stopPropagation();
        const isVisible = emojiPicker.style.display === 'grid';
        emojiPicker.style.display = isVisible ? 'none' : 'grid';
    });

    // 点击其他地方关闭表情选择器
    document.addEventListener('click', (e) => {
        if (!emojiPicker.contains(e.target) && !emojiButton.contains(e.target)) {
            emojiPicker.style.display = 'none';
        }
    });

    // 创建表情网格
    emojis.forEach(emoji => {
        const emojiDiv = document.createElement('div');
        emojiDiv.className = 'emoji-item';
        emojiDiv.dataset.emojiId = emoji.id;
        emojiDiv.title = emoji.name;
        emojiDiv.innerHTML = `<span class="emoji-char">${emoji.emoji}</span>`;
        emojiPicker.appendChild(emojiDiv);

        emojiDiv.addEventListener('click', () => {
            sendEmojiMessage(emoji.id);
            emojiPicker.style.display = 'none';
        });
    });
}

function sendEmojiMessage(emojiId) {
    let message = `emoji:${emojiId}`;
    
    // 如果是私聊，添加命令前缀，就像sendMessage函数一样
    if (currentChat.id !== 'all') {
        message = `/to ${currentChat.name} ${message}`;
    }
    
    fetch('/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: message })
    })
    .catch(error => {
        console.error('发送表情失败:', error);
        showNotification('发送表情失败，请重试', 'error');
    });
}

// =================================
// 连接状态检查
// =================================
function checkConnection() {
    fetch('/ping')
        .then(response => {
            if (!response.ok) {
                throw new Error('服务器无响应');
            }
            showConnectedState();
        })
        .catch(() => {
            showDisconnectedState();
        });
}

function showConnectedState() {
    const statusIndicator = document.getElementById('statusIndicator');
    if (!statusIndicator.classList.contains('online')) {
        statusIndicator.textContent = '状态: 已连接';
        statusIndicator.classList.remove('offline');
        statusIndicator.classList.add('online');
    }
}

function showDisconnectedState() {
    const statusIndicator = document.getElementById('statusIndicator');
    statusIndicator.textContent = '状态: 未连接';
    statusIndicator.classList.add('offline');
    statusIndicator.classList.remove('online');
}

// =================================
// 工具函数
// =================================
function formatBytes(bytes, decimals = 2) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

function isScrolledToBottom(element) {
    return element.scrollHeight - element.clientHeight <= element.scrollTop + 1;
}

function scrollToBottom(element) {
    element.scrollTop = element.scrollHeight;
}

function formatTime(date) {
    return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit' });
}

function showNotification(message, type = 'info') {
    const notification = document.createElement('div');
    notification.className = `notification ${type}`;
    notification.textContent = message;
    document.body.appendChild(notification);
    setTimeout(() => {
        notification.style.animation = 'slideOutRight 0.3s ease-in forwards';
        setTimeout(() => notification.remove(), 300);
    }, 3000);
}

// =================================
// 启动
// =================================
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
