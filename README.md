# 🚀 IP 代理检测工具 (IP Proxy Checker)

[![Go Version](https://img.shields.io/badge/Go-1.19+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux-lightgrey.svg)]()

> 一个高效、功能完整的代理检测工具，支持多种协议，具备地理位置识别和 Telegram 通知功能。

## ✨ 特性

- 🔍 **多协议支持** - SOCKS5、HTTP、HTTPS 代理检测
- 🌍 **GeoIP 定位** - 自动识别代理服务器的地理位置
- ⚡ **高并发检测** - 支持多线程并发，大幅提升检测效率
- 📊 **详细统计** - 完整的检测报告和数据分析
- 📱 **Telegram 通知** - 实时推送检测结果到 Telegram
- 📁 **多格式输出** - 支持 TXT、CSV、Telegram 格式文件
- 🎯 **智能重试** - 自动重试机制，确保检测准确性
- 🛡️ **代理验证** - 严格的代理可用性验证

## 📋 系统要求

- **操作系统**: Windows 10/11, Linux, macOS
- **Go 版本**: 1.19 或更高版本
- **内存**: 最少 512MB RAM
- **网络**: 稳定的互联网连接
- **可选**: 代理服务器访问权限

## 🚀 快速开始

### 1. 下载预编译版本

从 [Releases](https://github.com/dazzlejc/ip-checker/releases) 页面下载对应平台的可执行文件。

### 2. 编译源码

```bash
# 克隆仓库
git clone https://github.com/dazzlejc/ip-checker.git
cd ip-proxy-checker

# 编译
go build -o ip-checker ip-checker.go

# 运行
./ip-checker
```

### 3. 配置文件

创建 `config.ini` 文件：

```ini
[telegram]
bot_token = YOUR_TELEGRAM_BOT_TOKEN
chat_id = YOUR_TELEGRAM_CHAT_ID

[settings]
preset_proxy = socks5://user:pass@host:port,socks5://user:pass@host2:port2
fdip_dir = FDIP
output_dir = OUTPUT
check_timeout = 10
max_concurrent = 100
speed_test_url = https://speed.cloudflare.com/__down?bytes=100000000
```

### 4. 准备代理文件

将待检测的代理文件放入 `FDIP` 目录，支持以下格式：

```
# URL 格式
socks5://username:password@ip:port
http://username:password@ip:port

# 传统格式
ip:port|username:password|protocol

# 逗号分隔格式
socks5://user:pass@ip:port, additional_info
```

## 📖 使用说明

### 基本用法

#### 交互式模式（推荐）
不带任何参数运行程序，将显示图形化菜单界面：

```bash
# Linux/macOS
./ip-checker

# Windows
ip-checker.exe
# 或
.\ip-checker.exe
```

#### 命令行模式
指定参数后直接运行检测，不显示交互菜单：

```bash
# Linux/macOS
./ip-checker -i /path/to/proxies -o /path/to/output

# Windows
ip-checker.exe -i FDIP -o OUTPUT
# 或
.\ip-checker.exe -i FDIP -o OUTPUT
```

### 命令行参数

| 参数 | 描述 | 默认值 |
|------|------|--------|
| `-c` | 指定配置文件路径 | `config.ini` |
| `-i` | 指定代理输入目录（覆盖配置文件设置） | 配置文件中的 fdip_dir |
| `-o` | 指定输出目录（覆盖配置文件设置） | 配置文件中的 output_dir |
| `-s` | 自定义测速文件URL（可选） | 配置文件中的值 |
| `-h` | 显示帮助信息 | - |

### 使用示例

#### 1. 交互式使用
```bash
# 运行程序，进入图形菜单
.\ip-checker.exe
```

#### 2. 命令行使用
```bash
# 指定输入输出目录（Windows）
.\ip-checker.exe -i FDIP -o OUTPUT

# 使用自定义配置文件
.\ip-checker.exe -c my_config.ini

# 自定义测速文件
.\ip-checker.exe -s https://example.com/test.dat

# 组合使用多个参数
.\ip-checker.exe -i "C:\proxies" -o "C:\results" -s https://speed.test/file.dat

# 查看帮助
.\ip-checker.exe -h
```

#### 3. 不同操作系统示例
```bash
# Windows
ip-checker.exe -i FDIP -o OUTPUT

# Linux
./ip-checker -i /home/user/proxies -o /home/user/results

# macOS
./ip-checker -i ./proxies -o ./results
```

### 模式说明

- **交互式模式**：不指定参数时启动，提供图形菜单界面，适合新手用户
- **命令行模式**：指定任意参数时启动，直接运行检测，适合自动化和脚本使用

## 📊 输出文件

程序会生成以下输出文件：

| 文件名 | 描述 | 格式 |
|--------|------|------|
| `socks5_auth.txt` | 认证 SOCKS5 代理 | 文本 |
| `socks5_noauth.txt` | 无认证 SOCKS5 代理 | 文本 |
| `socks5_auth_tg.txt` | Telegram 格式认证 SOCKS5 | 文本 |
| `socks5_noauth_tg.txt` | Telegram 格式无认证 SOCKS5 | 文本 |
| `http.txt` | HTTP 代理 | 文本 |
| `https.txt` | HTTPS 代理 | 文本 |
| `residential.txt` | 住宅IP代理 | 文本 |
| `residential_tg.txt` | Telegram 格式住宅IP | 文本 |
| `socks5.csv` | 详细统计报告 | CSV |

## 📱 Telegram 集成

### 设置 Telegram Bot

1. 与 [@BotFather](https://t.me/botfather) 对话创建机器人
2. 获取 Bot Token
3. 获取你的 Chat ID (与 [@userinfobot](https://t.me/userinfobot) 对话)
4. 在 `config.ini` 中配置 Token 和 Chat ID

### 通知内容

程序会自动发送以下通知：

- 🚀 **启动通知** - 程序开始运行时
- 📊 **检测报告** - 包含统计数据和分布情况
- 📁 **文件推送** - 自动推送结果文件
- 🎉 **完成通知** - 程序运行结束时

## 🔧 高级配置

### 性能调优

```ini
[settings]
# 检测超时时间（秒）
check_timeout = 15

# 最大并发数
max_concurrent = 200

# 连接超时
connect_timeout = 10

# 读取超时
read_timeout = 30
```

### 代理配置

```ini
[settings]
# 预设代理列表（用于访问外部API）
preset_proxy = socks5://user:pass@proxy1:port
preset_proxy = socks5://user:pass@proxy2:port
preset_proxy = http://user:pass@proxy3:port
```

### 测速配置

```ini
[settings]
# 自定义测速文件
speed_test_url = https://your-server.com/test_file.dat

# 测速文件大小（字节）
speed_test_size = 50000000
```

## 📈 检测报告示例

```
🎉 代理检测报告
⏰ 耗时: 125.67 秒
✅ 有效代理: 1,247 个

🌐 协议分布:
  - socks5_auth: 856 个
  - http: 234 个
  - https: 157 个

🌍 国家分布:
  - 🇺🇸 美国 (US): 423 个
  - 🇸🇬 新加坡 (SG): 198 个
  - 🇯🇵 日本 (JP): 167 个
  - 🇰🇷 韩国 (KR): 145 个
  - 🇩🇪 德国 (DE): 98 个
  - 🌐 未知 (UNKNOWN): 216 个

📈 延迟统计:
  - 均值: 245.67ms
  - 最低: 12.34ms
  - 最高: 1,234.56ms

📊 下载速度统计:
  - 均值: 15.67 MB/s
  - 最低: 0.12 MB/s
  - 最高: 89.45 MB/s
```

## 🛠️ 故障排除

### 常见问题

**Q: Telegram 通知发送失败**
- 检查 Bot Token 和 Chat ID 是否正确
- 确保机器人有发送消息权限
- 检查网络连接和防火墙设置

**Q: GeoIP 查询失败**
- 确保网络可访问 MaxMind 服务器
- 检查 `GeoLite2-Country.mmdb` 文件是否存在
- 尝试手动下载 GeoIP 数据库

**Q: 代理检测成功率低**
- 检查代理文件格式是否正确
- 调整超时时间设置
- 尝试使用预设代理访问外部服务

**Q: 程序运行缓慢**
- 适当增加 `max_concurrent` 值
- 检查网络带宽
- 考虑使用更快的测速服务器

### 调试模式

启用详细日志输出：

```bash
# Windows
set DEBUG=1
./ip-checker.exe

# Linux/macOS
export DEBUG=1
./ip-checker
```

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

### 开发环境设置

```bash
# 克隆仓库
git clone https://github.com/dazzlejc/ip-checker.git
cd ip-proxy-checker

# 安装依赖
go mod tidy

# 运行测试
go test ./...

# 编译
go build -o ip-checker ip-checker.go
```

### 代码规范

- 遵循 Go 官方代码规范
- 使用 `gofmt` 格式化代码
- 添加必要的注释和文档
- 确保所有测试通过

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE)。

## 🙏 致谢

- [Anji-318/Socks5-validity-testing](https://github.com/Anji-318/Socks5-validity-testing) - 本项目基于此优秀的 SOCKS5 代理检测工具进行开发
- [MaxMind](https://www.maxmind.com/) - GeoIP 数据库
- [Telegram Bot API](https://core.telegram.org/bots/api) - 通知服务
- Go 社区 - 优秀的编程语言和工具

## 📞 联系方式

- 项目主页: [GitHub Repository](https://github.com/dazzlejc/ip-checker)
- 问题反馈: [Issues](https://github.com/dazzlejc/ip-checker/issues)
- 邮箱: your-email@example.com

---

⭐ 如果这个项目对你有帮助，请给个 Star！