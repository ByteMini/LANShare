// LANShare P2P Web客户端JavaScript代码

// =================================
// 全局状态变量
// =================================
let localUsername = '';
let currentChat = { id: 'all', name: '公聊' };
let allMessages = [];
let shownPendingTransfers = new Set();
let shownFailedTransfers = new Set();
let shownCompletedTransfers = new Set();
let blockedUsers = new Set();
let replyingToMessage = null; // 当前正在回复的消息

// =================================
async function loadBlockedUsers() {
    try {
        const response = await fetch('/acl');
        const data = await response.json();
        blockedUsers = new Set(data.blocked || []);
    } catch (error) {
        console.error('加载屏蔽列表失败:', error);
        blockedUsers = new Set();
    }
}

// =================================
async function init() {
    console.log('初始化开始');
    document.getElementById('messageInput').focus();
    
    // 先加载表情列表
    await loadGifEmojis();
    allEmojis = [...gifEmojis];
    createEmojiGrid();
    
    // 初始加载数据
    await loadBlockedUsers();
    loadUsers();
    loadHistory(); // 加载历史消息
    loadMessages();
    loadFileTransfers();
    
    // 设置定时器
    setInterval(loadMessages, 2000); // 消息可以稍微慢一点
    setInterval(() => {
        loadBlockedUsers();
        loadUsers();
    }, 3000);    // 用户列表不需要太频繁
    setInterval(loadFileTransfers, 3000);
    setInterval(checkConnection, 5000); // 添加连接检查
    
    // 初始化功能
    initFileTransfer();
    initChatSwitching();
    initEmojiPicker();
    initHistoryLoading();
    
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

    // 如果是私聊，检查是否屏蔽
    if (currentChat.id !== 'all') {
        if (blockedUsers.has(currentChat.name)) {
            showNotification(`请先解除对${currentChat.name}的屏蔽`, 'warning');
            return;
        }
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

let historyOffset = 0;
const HISTORY_LIMIT = 50;

function loadHistory() {
    const url = new URL('/loadhistory', window.location.origin);
    url.searchParams.append('chatId', currentChat.id);
    url.searchParams.append('limit', HISTORY_LIMIT);
    url.searchParams.append('offset', historyOffset);

    fetch(url)
        .then(response => response.json())
        .then(data => {
            if (data.messages && data.messages.length > 0) {
                // 历史消息按时间升序，已处理
                allMessages = data.messages.concat(allMessages);
                historyOffset += data.messages.length;
                displayMessages();
            }
        })
        .catch(error => console.error('加载历史消息失败:', error));
}

function initHistoryLoading() {
    // 添加加载更多按钮
    const messagesDiv = document.getElementById('messages');
    const loadMoreBtn = document.createElement('button');
    loadMoreBtn.id = 'loadMoreHistory';
    loadMoreBtn.textContent = '加载更多历史消息';
    loadMoreBtn.style.display = 'none';
    loadMoreBtn.onclick = () => {
        loadHistory();
    };
    messagesDiv.parentNode.insertBefore(loadMoreBtn, messagesDiv);

    // 监听滚动，如果滚动到顶部，加载更多
    messagesDiv.addEventListener('scroll', () => {
        if (messagesDiv.scrollTop === 0 && loadMoreBtn.style.display !== 'none') {
            loadHistory();
        }
    });

    // 更新按钮显示
    function updateLoadMoreButton() {
        if (historyOffset > 0) {
            loadMoreBtn.style.display = 'block';
        } else {
            loadMoreBtn.style.display = 'none';
        }
    }

    // 在 switchChat 时重置
    const originalSwitchChat = switchChat;
    switchChat = function(targetElement) {
        originalSwitchChat.call(this, targetElement);
        historyOffset = 0;
        allMessages = []; // 清空当前聊天消息
        loadHistory(); // 加载新聊天的历史
        updateLoadMoreButton();
    };

    updateLoadMoreButton();
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
    messageDiv.dataset.messageId = msg.messageId || '';

    // 添加回复指示器
    if (msg.messageType === 'reply' && msg.replyToSender && msg.replyToContent) {
        const replyIndicator = document.createElement('div');
        replyIndicator.className = 'reply-indicator-inline';
        replyIndicator.innerHTML = `
            <div class="reply-line"></div>
            <div class="reply-content">
                <strong>${msg.replyToSender}:</strong> ${msg.replyToContent.substring(0, 100)}${msg.replyToContent.length > 100 ? '...' : ''}
            </div>
        `;
        messageDiv.appendChild(replyIndicator);
    }

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    // 根据消息类型显示不同内容
    if (msg.messageType === 'image' && (msg.fileUrl || msg.fileName)) {
        // 图片消息
        const imageContainer = document.createElement('div');
        imageContainer.className = 'image-message';
        const imageUrl = msg.fileUrl || `/images/${msg.fileName}`;
        imageContainer.innerHTML = `
            <img src="${imageUrl}" alt="${msg.fileName}" class="message-image" onclick="openImageModal(this.src)">
            <div class="image-caption">${msg.content}</div>
        `;
        contentDiv.appendChild(imageContainer);
    } else if (msg.messageType === 'file' && msg.fileName) {
        // 文件消息
        const fileContainer = document.createElement('div');
        fileContainer.className = 'file-message';
        const fileIcon = getFileIcon(msg.fileType || msg.fileName);
        const fileSize = formatBytes(msg.fileSize || 0);
        fileContainer.innerHTML = `
            <div class="file-info">
                <span class="file-icon">${fileIcon}</span>
                <div class="file-details">
                    <div class="file-name">${msg.fileName}</div>
                    <div class="file-size">${fileSize}</div>
                </div>
            </div>
            <div class="file-caption">${msg.content}</div>
        `;
        contentDiv.appendChild(fileContainer);
    } else if (msg.content.startsWith('emoji:')) {
        // 表情消息
        const emojiId = msg.content.split(':')[1];
        const emoji = allEmojis.find(e => e.id === emojiId);
        if (emoji) {
            const emojiContainer = document.createElement('div');
            emojiContainer.className = 'emoji-message';

            if (emoji.type === 'gif') {
                emojiContainer.innerHTML = `<img class="emoji-large-gif" src="/emoji-gifs/${emoji.filename}" alt="${emoji.name}">`;
            }

            contentDiv.appendChild(emojiContainer);
        } else {
            contentDiv.textContent = msg.content;
        }
    } else {
        // 普通文本消息
        contentDiv.textContent = msg.content;
    }

    const timeDiv = document.createElement('div');
    timeDiv.className = 'message-time';
    timeDiv.textContent = formatTime(new Date(msg.timestamp));

    // 添加回复按钮（非自己的消息）
    if (!msg.isOwn && msg.messageId) {
        const replyBtn = document.createElement('button');
        replyBtn.className = 'reply-btn';
        replyBtn.textContent = '↩️';
        replyBtn.title = '回复此消息';
        replyBtn.onclick = () => replyToMessage(messageDiv);
        timeDiv.appendChild(replyBtn);
    }

    messageDiv.appendChild(contentDiv);
    messageDiv.appendChild(timeDiv);

    return messageDiv;
}

function getFileIcon(fileType) {
    if (fileType.startsWith('image/')) return '🖼️';
    if (fileType.startsWith('video/')) return '🎥';
    if (fileType.startsWith('audio/')) return '🎵';
    if (fileType.includes('pdf')) return '📄';
    if (fileType.includes('zip') || fileType.includes('rar')) return '📦';
    if (fileType.includes('doc') || fileType.includes('txt')) return '📝';
    return '📎';
}

function openImageModal(src) {
    const modal = document.createElement('div');
    modal.className = 'image-modal';
    modal.innerHTML = `
        <div class="image-modal-content">
            <img src="${src}" class="modal-image">
            <button class="close-modal" onclick="this.parentElement.parentElement.remove()">✕</button>
        </div>
    `;
    modal.onclick = (e) => {
        if (e.target === modal) modal.remove();
    };
    document.body.appendChild(modal);
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
            
            const isBlocked = blockedUsers.has(username);
            const buttonText = isBlocked ? '🔓' : '🚫';
            const buttonTitle = isBlocked ? '解除屏蔽' : '屏蔽用户';
            const liClass = isBlocked ? 'blocked' : '';

            let li;
            if (existingUsers.has(username)) {
                // 更新现有用户
                li = usersList.querySelector(`li[data-chat-id="${username}"]`);
                li.className = liClass;
                const btn = li.querySelector('.block-btn');
                btn.textContent = buttonText;
                btn.title = buttonTitle;
            } else {
                // 添加新用户
                li = document.createElement('li');
                li.className = liClass;
                li.dataset.chatId = username;
                li.dataset.chatName = username;
                li.innerHTML = `👤 ${username} <button class="block-btn" onclick="blockUser('${username}', event)" title="${buttonTitle}">${buttonText}</button>`;
                li.addEventListener('click', (e) => {
                    if (!e.target.classList.contains('block-btn')) {
                        switchChat(li);
                    }
                });
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
    const imageInput = document.getElementById('imageInput');
    const fileControls = document.getElementById('file-transfer-controls');
    const fileNameDisplay = document.getElementById('fileNameDisplay');

    // 文件选择处理
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

    // 图片选择处理
    imageInput.addEventListener('change', function(event) {
        if (event.target.files.length > 0) {
            const imageFile = event.target.files[0];
            sendImage(imageFile);
        }
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
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            return response.json();
        })
        .then(data => {
            const transfers = data.transfers || [];
            displayFileTransfers(transfers);

            // 处理待接收的文件确认对话框
            const pendingReceive = transfers.find(t => t.direction === 'receive' && t.status === 'pending');
            if (pendingReceive && !shownPendingTransfers.has(pendingReceive.fileId)) {
                showFileConfirmDialog(pendingReceive);
                shownPendingTransfers.add(pendingReceive.fileId);
            }

            // 检查是否有失败的传输并显示通知
            const failedTransfers = transfers.filter(t => t.status === 'failed');
            failedTransfers.forEach(transfer => {
                if (!shownFailedTransfers.has(transfer.fileId)) {
                    showNotification(`文件传输失败: ${transfer.fileName}`, 'error');
                    shownFailedTransfers.add(transfer.fileId);
                }
            });

            // 检查是否有完成的传输并显示通知
            const completedTransfers = transfers.filter(t => t.status === 'completed');
            completedTransfers.forEach(transfer => {
                if (!shownCompletedTransfers.has(transfer.fileId)) {
                    const directionText = transfer.direction === 'send' ? '发送' : '接收';
                    showNotification(`文件${directionText}完成: ${transfer.fileName}`, 'success');
                    shownCompletedTransfers.add(transfer.fileId);
                }
            });
        })
        .catch(error => {
            console.error('加载文件传输列表失败:', error);
            showNotification('无法加载文件传输状态，请检查连接', 'error');
        });
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

function displayFileTransfers(transfers) {
    const section = document.getElementById('fileTransfersSection');
    const list = document.getElementById('fileTransfersList');

    // 检查是否有活跃的传输（非完成状态）
    const activeTransfers = transfers.filter(t => t.status !== 'completed' && t.status !== 'failed');

    if (activeTransfers.length === 0) {
        // 如果没有活跃传输，隐藏区域
        section.style.display = 'none';
        return;
    }

    section.style.display = 'block';
    list.innerHTML = '';

    transfers.forEach(transfer => {
        const transferDiv = createFileTransferElement(transfer);
        list.appendChild(transferDiv);
    });
}

function createFileTransferElement(transfer) {
    const div = document.createElement('div');
    div.className = 'file-transfer-status';
    div.dataset.fileId = transfer.fileId;

    const progressPercent = transfer.fileSize > 0 ? (transfer.progress / transfer.fileSize * 100) : 0;
    const progressText = `${formatBytes(transfer.progress)} / ${formatBytes(transfer.fileSize)}`;
    const speedText = transfer.speed > 0 ? formatSpeed(transfer.speed) : '--';
    const etaText = transfer.eta > 0 ? formatETA(transfer.eta) : '--';

    const statusText = getStatusText(transfer.status);
    const directionIcon = transfer.direction === 'send' ? '📤' : '📥';

    div.innerHTML = `
        <div class="file-name">${directionIcon} ${transfer.fileName}</div>
        <div class="file-progress">
            <div class="progress-bar">
                <div class="progress-fill" style="width: ${progressPercent}%"></div>
            </div>
            <div class="progress-text">${progressPercent.toFixed(1)}%</div>
        </div>
        <div class="file-details">
            <div class="file-size">${progressText}</div>
            <div class="file-speed">速度: ${speedText}</div>
            <div class="file-eta">剩余: ${etaText}</div>
            <div class="file-status">状态: ${statusText}</div>
            <div class="file-peer">对方: ${transfer.peerName}</div>
        </div>
    `;

    return div;
}

function formatSpeed(bytesPerSecond) {
    if (bytesPerSecond < 1024) {
        return `${bytesPerSecond.toFixed(0)} B/s`;
    } else if (bytesPerSecond < 1024 * 1024) {
        return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`;
    } else {
        return `${(bytesPerSecond / (1024 * 1024)).toFixed(1)} MB/s`;
    }
}

function formatETA(seconds) {
    if (seconds < 60) {
        return `${seconds}秒`;
    } else if (seconds < 3600) {
        const minutes = Math.floor(seconds / 60);
        const remainingSeconds = seconds % 60;
        return `${minutes}分${remainingSeconds}秒`;
    } else {
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        return `${hours}时${minutes}分`;
    }
}

function getStatusText(status) {
    switch (status) {
        case 'pending': return '等待中';
        case 'transferring': return '传输中';
        case 'completed': return '已完成';
        case 'failed': return '失败';
        default: return status;
    }
}

// =================================
// 表情处理
// =================================
let gifEmojis = [];
let allEmojis = [];

function initEmojiPicker() {
    const emojiButton = document.getElementById('emoji-button');
    const emojiPicker = document.getElementById('emoji-picker');

    emojiButton.addEventListener('click', async (e) => {
        e.stopPropagation();
        
        // 检查表情资源是否存在
        try {
            const response = await fetch('/check-emoji-dir');
            const data = await response.json();
            if (!data.exists) {
                showEmojiAlert('当前表情资源缺少，若需表情资源请到https://github.com/ByteMini/telegram-emoji-gifs/releases/download/1.0.0/emoji.zip下载');
                return;
            }
        } catch (error) {
            console.error('检查表情目录失败:', error);
            // 检查失败时仍尝试显示（假设存在）
        }
        
        const isVisible = emojiPicker.style.display === 'grid';
        emojiPicker.style.display = isVisible ? 'none' : 'grid';
    });

    // 点击其他地方关闭表情选择器
    document.addEventListener('click', (e) => {
        if (!emojiPicker.contains(e.target) && !emojiButton.contains(e.target)) {
            emojiPicker.style.display = 'none';
        }
    });
}

function loadGifEmojis() {
    return fetch('/emoji-gifs-list')
        .then(response => response.json())
        .then(data => {
            if (Array.isArray(data)) {
                // 如果直接返回数组
                gifEmojis = data.map(emoji => ({
                    id: `gif-${emoji.id}`,
                    name: emoji.name,
                    filename: emoji.filename,
                    type: 'gif'
                }));
            } else if (data.emojis && Array.isArray(data.emojis)) {
                // 如果返回包装对象
                gifEmojis = data.emojis.map(emoji => ({
                    id: `gif-${emoji.id}`,
                    name: emoji.name,
                    filename: emoji.filename,
                    type: 'gif'
                }));
            } else {
                console.warn('无法加载 GIF 表情列表');
                gifEmojis = [];
            }
            console.log(`已加载 ${gifEmojis.length} 个 GIF 表情`);
        })
        .catch(error => {
            console.error('加载 GIF 表情失败:', error);
            gifEmojis = [];
        });
}

function createEmojiGrid() {
    const emojiPicker = document.getElementById('emoji-picker');
    emojiPicker.innerHTML = ''; // 清空现有内容

    // 如果有 GIF 表情，添加表情项
    if (gifEmojis.length > 0) {
        gifEmojis.forEach(emoji => {
            const emojiDiv = createEmojiElement(emoji);
            emojiPicker.appendChild(emojiDiv);
        });
    }
}

function createEmojiElement(emoji) {
    const emojiDiv = document.createElement('div');
    emojiDiv.className = 'emoji-item';
    emojiDiv.dataset.emojiId = emoji.id;
    emojiDiv.title = emoji.name;

    if (emoji.type === 'static') {
        emojiDiv.innerHTML = `<span class="emoji-char">${emoji.emoji}</span>`;
    } else if (emoji.type === 'gif') {
        emojiDiv.innerHTML = `<img class="emoji-gif" src="/emoji-gifs/${emoji.filename}" alt="${emoji.name}" loading="lazy">`;
    }

    emojiDiv.addEventListener('click', () => {
        sendEmojiMessage(emoji.id);
        document.getElementById('emoji-picker').style.display = 'none';
    });

    return emojiDiv;
}

function sendEmojiMessage(emojiId) {
    // 检查表情资源是否存在
    fetch('/check-emoji-dir')
        .then(response => response.json())
        .then(data => {
            if (!data.exists) {
                showEmojiAlert('当前表情资源缺少，若需表情资源请到https://github.com/ByteMini/telegram-emoji-gifs/releases/download/1.0.0/emoji.zip下载');
                return;
            }
            
            // 资源存在，继续发送
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
        })
        .catch(error => {
            console.error('检查表情目录失败:', error);
            // 即使检查失败，也尝试发送（假设资源存在）
            let message = `emoji:${emojiId}`;
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
        });
    
    document.getElementById('emoji-picker').style.display = 'none';
}

// =================================
// 连接状态检查
// =================================
function checkConnection() {
    fetch('/ping')
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            showConnectedState();
        })
        .catch(error => {
            console.error('连接检查失败:', error);
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
    if (!statusIndicator.classList.contains('offline')) {
        statusIndicator.textContent = '状态: 未连接';
        statusIndicator.classList.add('offline');
        statusIndicator.classList.remove('online');
        showNotification('与服务器断开连接，正在尝试重连...', 'warning');
    }
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
// 屏蔽用户功能
// =================================
function blockUser(username, event) {
    event.stopPropagation(); // 防止触发li的click事件

    const isCurrentlyBlocked = blockedUsers.has(username);
    const command = isCurrentlyBlocked ? `/unblock ${username}` : `/block ${username}`;
    const action = isCurrentlyBlocked ? '解除屏蔽' : '屏蔽';
    const message = `${action}用户 ${username}`;

    fetch('/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: command })
    })
    .then(response => {
        if (response.ok) {
            // 重新加载屏蔽列表以同步状态
            loadBlockedUsers().then(() => {
                loadUsers();
                showNotification(message + '成功', 'success');
            });
        } else {
            throw new Error(`${action}失败`);
        }
    })
    .catch(error => {
        console.error(`${action}用户失败:`, error);
        showNotification(`${action}用户失败，请重试`, 'error');
    });
}

// 添加自定义警报函数
function showEmojiAlert(message) {
    const dialog = document.getElementById('emoji-alert-dialog');
    const messageEl = document.getElementById('alert-message');
    const okBtn = document.getElementById('alert-ok-btn');

    messageEl.textContent = message;
    dialog.style.display = 'flex';
    setTimeout(() => dialog.classList.add('visible'), 10);

    const hideDialog = () => {
        dialog.classList.remove('visible');
        setTimeout(() => {
            dialog.style.display = 'none';
        }, 300);
    };

    okBtn.onclick = hideDialog;

    // 点击遮罩关闭
    dialog.onclick = (e) => {
        if (e.target === dialog) hideDialog();
    };
}

// =================================
// 图片消息功能
// =================================
function sendImage(imageFile) {
    if (!imageFile) {
        showNotification('请选择图片文件', 'error');
        return;
    }

    // 对于公聊，直接使用'all'作为目标；对于私聊，使用当前聊天对象
    const targetName = currentChat.id === 'all' ? 'all' : currentChat.name;

    const formData = new FormData();
    formData.append('image', imageFile);
    formData.append('targetName', targetName);

    fetch('/sendimage', {
        method: 'POST',
        body: formData
    })
    .then(response => response.json())
    .then(data => {
        if (data.status === 'success') {
            showNotification('图片发送成功', 'success');
        } else {
            throw new Error('发送失败');
        }
    })
    .catch(error => {
        console.error('发送图片失败:', error);
        showNotification('发送图片失败，请重试', 'error');
    });
}

function getFirstOnlineUser() {
    // 获取第一个在线用户（除了自己）
    const usersList = document.getElementById('usersList');
    const userElements = usersList.querySelectorAll('li[data-chat-id]:not(.own)');
    for (let userEl of userElements) {
        const userName = userEl.dataset.chatId;
        if (!blockedUsers.has(userName)) {
            return userName;
        }
    }
    return null;
}

// =================================
// 消息回复功能
// =================================
function replyToMessage(messageElement) {
    const messageId = messageElement.dataset.messageId;
    const sender = messageElement.querySelector('.message-sender').textContent;
    const content = messageElement.querySelector('.message-content').textContent;

    replyingToMessage = {
        id: messageId,
        sender: sender,
        content: content
    };

    const input = document.getElementById('messageInput');
    input.placeholder = `回复 ${sender}: ${content.substring(0, 20)}...`;
    input.focus();

    // 添加回复UI提示
    showReplyIndicator(sender, content);
}

function showReplyIndicator(sender, content) {
    // 移除现有的回复指示器
    const existingIndicator = document.querySelector('.reply-indicator');
    if (existingIndicator) {
        existingIndicator.remove();
    }

    const indicator = document.createElement('div');
    indicator.className = 'reply-indicator';
    indicator.innerHTML = `
        <div class="reply-info">
            <strong>回复 ${sender}:</strong> ${content.substring(0, 50)}${content.length > 50 ? '...' : ''}
            <button onclick="cancelReply()" class="cancel-reply-btn">✕</button>
        </div>
    `;

    const inputArea = document.querySelector('.input-area');
    inputArea.insertBefore(indicator, inputArea.firstChild);
}

function cancelReply() {
    replyingToMessage = null;
    const input = document.getElementById('messageInput');
    input.placeholder = currentChat.id === 'all' ? '输入公共消息...' : `私聊 ${currentChat.name}...`;

    const indicator = document.querySelector('.reply-indicator');
    if (indicator) {
        indicator.remove();
    }
}

// 修改发送消息函数以支持回复
function sendMessage() {
    const input = document.getElementById('messageInput');
    let message = input.value.trim();

    if (message === '') {
        input.style.animation = 'shake 0.3s ease-in-out';
        setTimeout(() => { input.style.animation = ''; }, 300);
        return;
    }

    // 如果是回复消息
    if (replyingToMessage) {
        sendReplyMessage(message);
        return;
    }

    // 检查是否是私聊
    if (currentChat.id !== 'all') {
        if (blockedUsers.has(currentChat.name)) {
            showNotification(`请先解除对${currentChat.name}的屏蔽`, 'warning');
            return;
        }
        message = `/to ${currentChat.name} ${message}`;
    }

    fetch('/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: message })
    })
    .then(response => {
        if (response.ok) {
            input.value = '';
            cancelReply(); // 清除回复状态
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

function sendReplyMessage(replyContent) {
    if (!replyingToMessage) return;

    const targetName = currentChat.id === 'all' ? 'all' : currentChat.name;

    fetch('/sendreply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            targetName: targetName,
            replyContent: replyContent,
            originalMsgId: replyingToMessage.id,
            originalSender: replyingToMessage.sender,
            originalContent: replyingToMessage.content
        })
    })
    .then(response => {
        if (response.ok) {
            document.getElementById('messageInput').value = '';
            cancelReply();
            showNotification('回复发送成功', 'success');
        } else {
            throw new Error('发送失败');
        }
    })
    .catch(error => {
        console.error('发送回复失败:', error);
        showNotification('发送回复失败，请重试', 'error');
    });
}
