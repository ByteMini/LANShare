#!/bin/bash

# LANShare P2P 跨平台构建脚本

echo "==========================================="
echo "         LANShare P2P 构建脚本"
echo "==========================================="

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "错误: 未找到Go编译器，请先安装Go"
    exit 1
fi

# 创建构建目录
mkdir -p build

# 清理旧的构建文件
echo "正在清理旧的构建文件..."
rm -f build/*

# 源文件列表
SOURCE_FILES="main.go types.go network.go discovery.go web.go filetransfer.go"

# 检查所有源文件是否存在
for file in $SOURCE_FILES; do
    if [ ! -f "$file" ]; then
        echo "错误: 源文件 $file 不存在"
        exit 1
    fi
done

echo "正在构建所有平台版本..."
echo ""

# 构建函数
build_platform() {
    local GOOS=$1
    local GOARCH=$2
    local OUTPUT_NAME=$3
    local PLATFORM_NAME=$4
    
    echo "构建 $PLATFORM_NAME ($GOOS/$GOARCH)..."
    
    if [[ "$GOOS" == "windows" ]]; then
        # 根据 arch 选择 mingw 工具链
        local CC_TOOL=""
        local CXX_TOOL=""
        local MINGW_INSTALL_MSG=""
        case $GOARCH in
            amd64)
                CC_TOOL="x86_64-w64-mingw32-gcc"
                CXX_TOOL="x86_64-w64-mingw32-g++"
                MINGW_INSTALL_MSG="sudo apt install mingw-w64 gcc-mingw-w64-x86-64"
                ;;
            386)
                CC_TOOL="i686-w64-mingw32-gcc"
                CXX_TOOL="i686-w64-mingw32-g++"
                MINGW_INSTALL_MSG="sudo apt install mingw-w64-i686 gcc-mingw-w64-i686"
                ;;
            arm64)
                CC_TOOL="aarch64-w64-mingw32-gcc"
                CXX_TOOL="aarch64-w64-mingw32-g++"
                MINGW_INSTALL_MSG="sudo apt install mingw-w64-arm64 gcc-mingw-w64-aarch64"
                ;;
            *)
                echo "  ✗ 构建失败: $PLATFORM_NAME (不支持的 Windows arch: $GOARCH)"
                return 1
                ;;
        esac
        
        # 检查 mingw 工具链
        if ! command -v $CC_TOOL &> /dev/null; then
            echo "  ✗ 构建失败: $PLATFORM_NAME (请安装 $GOARCH mingw-w64: $MINGW_INSTALL_MSG)"
            return 1
        fi
        # Windows 构建使用 CGO_ENABLED=1 和相应 mingw
        if GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=1 CC=$CC_TOOL CXX=$CXX_TOOL go build -ldflags="-s -w" -o "build/$OUTPUT_NAME" $SOURCE_FILES; then
            # 获取文件大小
            if command -v stat &> /dev/null; then
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    SIZE=$(stat -f%z "build/$OUTPUT_NAME")
                else
                    SIZE=$(stat -c%s "build/$OUTPUT_NAME")
                fi
                SIZE_MB=$(echo "scale=2; $SIZE/1024/1024" | bc 2>/dev/null || echo "N/A")
                echo "  ✓ 构建成功: build/$OUTPUT_NAME (${SIZE_MB}MB) - CGO 支持启用"
            else
                echo "  ✓ 构建成功: build/$OUTPUT_NAME - CGO 支持启用"
            fi
        else
            echo "  ✗ 构建失败: $PLATFORM_NAME (检查 $GOARCH mingw-w64 安装和 CGO)"
            return 1
        fi
    else
        if GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "build/$OUTPUT_NAME" $SOURCE_FILES; then
            # 获取文件大小
            if command -v stat &> /dev/null; then
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    SIZE=$(stat -f%z "build/$OUTPUT_NAME")
                else
                    SIZE=$(stat -c%s "build/$OUTPUT_NAME")
                fi
                SIZE_MB=$(echo "scale=2; $SIZE/1024/1024" | bc 2>/dev/null || echo "N/A")
                echo "  ✓ 构建成功: build/$OUTPUT_NAME (${SIZE_MB}MB)"
            else
                echo "  ✓ 构建成功: build/$OUTPUT_NAME"
            fi
        else
            echo "  ✗ 构建失败: $PLATFORM_NAME"
            return 1
        fi
    fi
}

# 构建所有平台
echo "开始构建多平台版本..."
echo ""

# macOS
build_platform "darwin" "amd64" "lanshare-macos-amd64" "macOS (Intel)"
build_platform "darwin" "arm64" "lanshare-macos-arm64" "macOS (Apple Silicon)"

# Linux
build_platform "linux" "amd64" "lanshare-linux-amd64" "Linux (x86_64)"
build_platform "linux" "arm64" "lanshare-linux-arm64" "Linux (ARM64)"
build_platform "linux" "386" "lanshare-linux-386" "Linux (x86)"
build_platform "linux" "arm" "lanshare-linux-arm" "Linux (ARM)"

# Windows (支持 amd64, 386, arm64 - 需要相应 mingw 工具链)
build_platform "windows" "amd64" "lanshare-windows-amd64.exe" "Windows (x86_64)"
build_platform "windows" "386" "lanshare-windows-386.exe" "Windows (x86)"
build_platform "windows" "arm64" "lanshare-windows-arm64.exe" "Windows (ARM64)"

# FreeBSD
build_platform "freebsd" "amd64" "lanshare-freebsd-amd64" "FreeBSD (x86_64)"
build_platform "freebsd" "arm64" "lanshare-freebsd-arm64" "FreeBSD (ARM64)"

echo ""
echo "==========================================="
echo "           构建完成"
echo "==========================================="

# 显示构建结果
echo "构建的文件列表:"
ls -la build/lanshare* 2>/dev/null | while read line; do
    echo "  $line"
done

echo ""
echo "使用说明:"
echo "  macOS (Intel):     ./build/lanshare-macos-amd64"
echo "  macOS (M1/M2):     ./build/lanshare-macos-arm64"
echo "  Linux (x86_64):    ./build/lanshare-linux-amd64"
echo "  Linux (ARM64):     ./build/lanshare-linux-arm64"
echo "  Windows (x86_64):  ./build/lanshare-windows-amd64.exe"
echo "  Windows (ARM64):   ./build/lanshare-windows-arm64.exe"
echo ""

# 创建当前平台的默认链接
CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m)

case $CURRENT_ARCH in
    x86_64|amd64)
        CURRENT_ARCH="amd64"
        ;;
    aarch64|arm64)
        CURRENT_ARCH="arm64"
        ;;
    i386|i686)
        CURRENT_ARCH="386"
        ;;
    armv7l)
        CURRENT_ARCH="arm"
        ;;
esac

case $CURRENT_OS in
    darwin)
        DEFAULT_BINARY="lanshare-macos-$CURRENT_ARCH"
        ;;
    linux)
        DEFAULT_BINARY="lanshare-linux-$CURRENT_ARCH"
        ;;
    mingw*|cygwin*|msys*)
        DEFAULT_BINARY="lanshare-windows-$CURRENT_ARCH.exe"
        ;;
    freebsd)
        DEFAULT_BINARY="lanshare-freebsd-$CURRENT_ARCH"
        ;;
    *)
        DEFAULT_BINARY=""
        ;;
esac

if [ -n "$DEFAULT_BINARY" ] && [ -f "build/$DEFAULT_BINARY" ]; then
    ln -sf "$DEFAULT_BINARY" "build/lanshare"
    echo "已创建当前平台的默认链接: build/lanshare -> build/$DEFAULT_BINARY"
    echo ""
    echo "快速启动当前平台版本:"
    echo "  ./build/lanshare"
fi

echo ""
echo "构建完成！所有平台的可执行文件已保存在 build/ 目录中。"
