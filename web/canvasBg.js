const canvas = document.getElementById("canvas");
const ctx = canvas.getContext("2d");
canvas.width = window.innerWidth;
canvas.height = window.innerHeight;

const stars = [];
const numStars = 100;
let meteors = [];
let meteorInterval;
let animationFrameId;

class Star {
    constructor(x, y, speedX, speedY, size, color) {
        this.x = x;
        this.y = y;
        this.speedX = speedX;
        this.speedY = speedY;
        this.size = size;
        this.color = color;
    }

    update() {
        this.x += this.speedX;
        this.y += this.speedY;

        if (this.x > canvas.width || this.y < 0) {
            if (Math.random() > 0.6) {
                this.x = 0;
                this.y = Math.random() * canvas.height;
            } else {
                this.x = Math.random() * canvas.width;
                this.y = canvas.height;
            }
        }
    }

    draw() {
        ctx.beginPath();
        ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
        ctx.fillStyle = this.color;
        ctx.fill();
    }
}

class Meteor {
    constructor(x, y, speedX, speedY, length, color) {
        this.x = x;
        this.y = y;
        this.speedX = speedX;
        this.speedY = speedY;
        this.length = length;
        this.color = color;
        this.opacity = 1.0;
        this.fadeOut = false;
    }

    update() {
        this.x += this.speedX;
        this.y += this.speedY;

        if (this.x > canvas.width || this.y < 0) {
            this.fadeOut = true;
        }

        if (this.fadeOut) {
            this.opacity -= 0.02;
            if (this.opacity < 0) this.opacity = 0;
        }
    }

    draw() {
        // 阴影
        ctx.shadowBlur = 10;
        ctx.shadowColor = "rgba(255,255,255,1)";

        const gradient = ctx.createLinearGradient(
            this.x,
            this.y,
            this.x - this.length * this.speedX,
            this.y - this.length * this.speedY
        );
        gradient.addColorStop(0, this.color);
        gradient.addColorStop(1, "rgba(255,255,255,0)");

        ctx.beginPath();
        ctx.moveTo(this.x, this.y);
        ctx.lineTo(
            this.x - this.length * this.speedX,
            this.y - this.length * this.speedY
        );
        ctx.strokeStyle = gradient;
        ctx.globalAlpha = this.opacity;
        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.globalAlpha = 1.0;

        // 清除阴影
        ctx.shadowBlur = 0;
        ctx.shadowColor = "rgba(0,0,0,0)";
    }

    isAlive() {
        return this.opacity > 0;
    }
}

function initStars() {
    for (let i = 0; i < numStars; i++) {
        const x = Math.random() * canvas.width;
        const y = Math.random() * canvas.height;
        const speedX = Math.random() * 0.3 + 0.3;
        const speedY = -(Math.random() * 0.3 + 0.3);
        const size = Math.random() * 1.5 + 0.5;
        const color = "#ffffff";
        stars.push(new Star(x, y, speedX, speedY, size, color));
    }
}

function initMeteors() {
    meteorInterval = setInterval(() => {
        const x = Math.random() * canvas.width;
        const y = canvas.height;
        const speedX = Math.random() * 2 + 2;
        const speedY = -(Math.random() * 2 + 2);
        const length = Math.random() * 10 + 10;
        const color = "white";
        meteors.push(new Meteor(x, y, speedX, speedY, length, color));
    }, 4000); // 4秒一个流星
}

function animate() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    // stars
    for (const star of stars) {
        star.update();
        star.draw();
    }

    // meteors
    for (let i = meteors.length - 1; i >= 0; i--) {
        const meteor = meteors[i];
        meteor.update();
        if (!meteor.isAlive()) {
            meteors.splice(i, 1);
        } else {
            meteor.draw();
        }
    }

    animationFrameId = requestAnimationFrame(animate);
}

function resizeCanvas() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
}

window.addEventListener("resize", resizeCanvas);

window.addEventListener("load", () => {
    initStars();
    initMeteors();
    animate();
});

// 页面关闭时清理
window.addEventListener("beforeunload", () => {
    cancelAnimationFrame(animationFrameId);
    clearInterval(meteorInterval);
});
