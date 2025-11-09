# 更新日志

所有重要更改都会记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [1.0.0] - 2024-11-09

### 新增
- 🎉 首次发布 IP 代理检测工具
- ✨ 支持多种代理协议检测 (SOCKS5, HTTP, HTTPS)
- 🌍 集成 GeoIP 地理位置查询
- 📱 Telegram 通知推送功能
- 📊 详细的检测报告和统计
- 📁 多格式输出文件 (TXT, CSV, Telegram 格式)
- ⚡ 高并发检测支持
- 🛡️ 严格的代理可用性验证
- 🎯 智能重试机制

### 技术特性
- Go 语言开发，跨平台支持
- 内存优化的并发处理
- 自动 GeoIP 数据库更新
- 简化的 Telegram API 集成
- 灵活的配置文件系统
- 完整的命令行参数支持

### 支持的平台
- Windows 10/11
- Linux (Ubuntu, CentOS, Debian)
- macOS

### 输出格式
- 纯文本格式代理列表
- CSV 格式详细报告
- Telegram 点击即用链接
- 住宅 IP 专用格式

---

## 版本说明

- **主版本号**: 不兼容的 API 修改
- **次版本号**: 向下兼容的功能性新增
- **修订号**: 向下兼容的问题修正