# IP Checker Tool

一个高效的多线程IP地址验证和代理检测工具，支持HTTP/HTTPS和SOCKS5代理的批量检测。

## 功能特点

- 🚀 **多线程检测** - 支持并发检测，提高检测效率
- 🌍 **地理位置检测** - 集成GeoIP数据库，显示IP地址的地理位置信息
- 🔍 **多种代理类型** - 支持HTTP/HTTPS、SOCKS4、SOCKS5代理检测
- 📊 **详细报告** - 生成详细的检测结果报告
- ⚙️ **灵活配置** - 支持配置文件自定义参数
- 🛠️ **诊断工具** - 内置诊断功能，帮助排查网络问题

## 系统要求

- Go 1.19 或更高版本
- Windows/Linux/macOS

## 安装

### 从源码编译

```bash
git clone https://github.com/dazzlejc/ip-checker.git
cd ip-checker
go mod tidy
go build -o ip-checker ip-checker.go
```

### 直接下载可执行文件

从 [Releases](https://github.com/dazzlejc/ip-checker/releases) 页面下载适合您系统的预编译可执行文件。

## 使用方法

### 基本用法

```bash
# 检测单个IP地址
./ip-checker -ip 8.8.8.8

# 检测IP列表
./ip-checker -file ip_list.txt

# 检测代理
./ip-checker -proxy -proxy_type http -file proxy_list.txt
```

### 配置选项

创建 `config.ini` 文件来自定义配置：

```ini
[thread]
threads = 100
timeout = 10

[proxy]
check_proxy = true
proxy_types = http,https,socks5

[output]
save_results = true
output_format = txt,csv

[geoip]
database_path = GeoLite2-Country.mmdb
```

### 命令行参数

- `-ip` - 指定要检测的IP地址
- `-file` - 指定包含IP地址的文件
- `-proxy` - 启用代理检测模式
- `-proxy_type` - 指定代理类型 (http/https/socks4/socks5)
- `-threads` - 设置并发线程数
- `-timeout` - 设置超时时间（秒）
- `-output` - 指定输出文件
- `-config` - 指定配置文件路径

## 输出文件

工具会在以下目录生成结果：

- `OUTPUT/https.txt` - HTTP/HTTPS代理结果
- `OUTPUT/socks5_auth.txt` - SOCKS5代理结果
- `OUTPUT/residential.txt` - 住宅代理结果

## 示例

### 检测代理列表

```bash
# 检测HTTP代理
./ip-checker -proxy -proxy_type http -file http_proxies.txt

# 检测SOCKS5代理
./ip-checker -proxy -proxy_type socks5 -file socks5_proxies.txt

# 自定义线程数和超时
./ip-checker -proxy -threads 200 -timeout 15 -file proxies.txt
```

### 地理位置检测

```bash
# 检测IP并显示地理位置信息
./ip-checker -ip 1.1.1.1 -geoip
```

## 配置文件详解

### config.ini 完整配置示例

```ini
# 线程配置
[thread]
threads = 50              # 并发线程数
timeout = 30              # 网络超时时间（秒）
retry = 3                 # 重试次数

# 代理配置
[proxy]
check_proxy = true        # 是否启用代理检测
proxy_types = http,https,socks5  # 支持的代理类型
check_anonymity = true    # 检查匿名级别
check_speed = true        # 测试连接速度

# 输出配置
[output]
save_results = true       # 保存结果到文件
output_format = txt,csv   # 输出格式
create_backup = true      # 创建备份文件
log_level = info          # 日志级别

# GeoIP配置
[geoip]
enabled = true           # 启用GeoIP检测
database_path = ./GeoLite2-Country.mmdb  # 数据库路径
```

## 诊断工具

工具包含诊断功能，帮助排查网络问题：

```bash
# 运行诊断
./ip-checker -diagnose

# 诊断特定IP
./ip-checker -diagnose -ip 8.8.8.8
```

## 故障排除

### 常见问题

1. **编译错误：go mod not found**
   ```bash
   go mod init ip-checker
   go mod tidy
   ```

2. **GeoIP数据库缺失**
   - 下载GeoLite2-Country.mmdb数据库文件
   - 放置在程序同目录下或指定路径

3. **权限问题**
   - Linux/macOS: `chmod +x ip-checker`
   - Windows: 以管理员身份运行

4. **网络超时**
   - 增加超时时间：`-timeout 60`
   - 减少线程数：`-threads 10`

## 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 免责声明

本工具仅用于合法的网络测试和诊断目的。使用者需要遵守当地法律法规，不得用于非法活动。开发者不承担任何滥用责任。

## 更新日志

### v1.0.0
- 初始版本发布
- 支持HTTP/HTTPS/SOCKS5代理检测
- 集成GeoIP地理位置检测
- 多线程并发检测
- 详细的检测报告生成

## 联系方式

- GitHub: [@dazzlejc](https://github.com/dazzlejc)
- Issues: [GitHub Issues](https://github.com/dazzlejc/ip-checker/issues)

---

⭐ 如果这个项目对您有帮助，请给它一个星标！