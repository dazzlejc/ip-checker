package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/oschwald/geoip2-golang"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/proxy"
	"gopkg.in/ini.v1"
)

// Config 结构体用于映射 config.ini 文件的内容
type Config struct {
	Telegram struct {
		BotToken string `ini:"bot_token"`
		ChatID   string `ini:"chat_id"`
	} `ini:"telegram"`
	Settings struct {
		PresetProxy   []string `ini:"preset_proxy"`
		FdipDir       string   `ini:"fdip_dir"`
		OutputDir     string   `ini:"output_dir"`
		CheckTimeout  int      `ini:"check_timeout"`
		MaxConcurrent int      `ini:"max_concurrent"`
	} `ini:"settings"`
	IPDetection struct {
		Enabled         bool     `ini:"enabled"`
		Services        []string `ini:"services"`
		IPInfoToken     string   `ini:"ipinfo_token"`
		IPRegistryKey   string   `ini:"ipregistry_key"`
		MaxConcurrent   int      `ini:"max_concurrent"`
		Timeout         int      `ini:"timeout"`
	} `ini:"ip_detection"`
	AutoProxyUpdate struct {
		Enabled          bool   `ini:"enabled"`
		MaxProxies       int    `ini:"max_proxies"`
		PreferResidential bool `ini:"prefer_residential"`
		MaxLatency       float64 `ini:"max_latency"`
		BackupConfig     bool   `ini:"backup_config"`
	} `ini:"auto_proxy_update"`
}

var (
	config    Config
	logFile   *os.File
	logMutex  sync.Mutex
)

// LogWriter 是一个实现了 io.Writer 接口的结构体，用于将日志同时写入文件和控制台，并移除时间戳
type LogWriter struct{}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	// 屏蔽 Telegram Bot Token
	logStr := string(p)
	if config.Telegram.BotToken != "" {
		logStr = strings.ReplaceAll(logStr, config.Telegram.BotToken, "[REDACTED]")
	}

	// 将处理后的字符串转换回字节
	cleanP := []byte(logStr)

	// 写入控制台
	os.Stdout.Write(cleanP)

	// 写入文件时移除颜色代码
	cleanP = removeColorCodes(cleanP)
	if logFile != nil {
		return logFile.Write(cleanP)
	}

	return len(cleanP), nil
}

// removeColorCodes 移除ANSI颜色代码
func removeColorCodes(p []byte) []byte {
	// ANSI 颜色代码通常以 `\033[` 开头，以 `m` 结尾
	re := regexp.MustCompile("\033\\[[0-9;]*m")
	return re.ReplaceAll(p, []byte(""))
}

// 定义颜色常量
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

// 定义颜色列表，用于随机选择
var colors = []string{ColorRed, ColorGreen, ColorYellow, ColorBlue, ColorCyan}

// ========= 1. 全局常量和配置 =========

// TEST_URLS 是用于测试代理的 URL 列表
var TEST_URLS = []string{
	"http://httpbin.org/ip",
	"https://httpbin.org/ip",
	"https://api.ipify.org?format=json",
}

// GEOIP_DB_URL 是 GeoIP 数据库的下载地址
const GEOIP_DB_URL = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-Country.mmdb"

// GEOIP_DB_PATH 是 GeoIP 数据库的本地路径
const GEOIP_DB_PATH = "GeoLite2-Country.mmdb"

// TEST_URL 是用于测试代理的 URL
const TEST_URL = "http://api.ipify.org"

var (
	// OUTPUT_FILES 定义了输出文件的名称
	OUTPUT_FILES = map[string]string{
		"socks5_auth":      "socks5_auth.txt",
		"socks5_noauth":    "socks5_noauth.txt",
		"socks4_auth":      "socks4_auth.txt",
		"socks4_noauth":    "socks4_noauth.txt",
		"http":             "http.txt",
		"https":            "https.txt",
		"socks5_auth_tg":   "socks5_auth_tg.txt",
		"socks5_noauth_tg": "socks5_noauth_tg.txt",
		"residential":      "residential.txt",
		"residential_tg":   "residential_tg.txt",
	}

	// COUNTRY_CODE_TO_NAME 存储国家代码到中文名的映射
	COUNTRY_CODE_TO_NAME = map[string]string{
		"AF": "阿富汗", "AL": "阿尔巴尼亚", "DZ": "阿尔及利亚", "AS": "美属萨摩亚", "AD": "安道尔",
		"AO": "安哥拉", "AI": "安圭拉", "AQ": "南极洲", "AG": "安提瓜和巴布达", "AR": "阿根廷",
		"AM": "亚美尼亚", "AW": "阿鲁巴", "AU": "澳大利亚", "AT": "奥地利", "AZ": "阿塞拜疆",
		"BS": "巴哈马", "BH": "巴林", "BD": "孟加拉国", "BB": "巴巴多斯", "BY": "白俄罗斯",
		"BE": "比利时", "BZ": "伯利兹", "BM": "百慕大", "BT": "不丹", "BO": "玻利维亚",
		"BA": "波斯尼亚和黑塞哥维那", "BW": "博茨瓦纳", "BR": "巴西", "IO": "英属印度洋领地",
		"VG": "英属维尔京群岛", "BN": "文莱", "BG": "保加利亚", "BF": "布基纳法索", "BI": "布隆迪",
		"KH": "柬埔寨", "CM": "喀麦隆", "CA": "加拿大", "CV": "佛得角", "KY": "开曼群岛",
		"CF": "中非共和国", "TD": "乍得", "CL": "智利", "CN": "中国", "CX": "圣诞岛",
		"CC": "科科斯群岛", "CO": "哥伦比亚", "KM": "科摩罗", "CK": "库克群岛", "CR": "哥斯达黎加",
		"CI": "科特迪瓦", "HR": "克罗地亚", "CU": "古巴", "CY": "塞浦路斯", "CZ": "捷克共和国",
		"CD": "刚果民主共和国", "DK": "丹麦", "DJ": "吉布提", "DM": "多米尼克", "DO": "多米尼加共和国",
		"TL": "东帝汶", "EC": "厄瓜多尔", "EG": "埃及", "SV": "萨尔瓦多", "GQ": "赤道几内亚",
		"ER": "厄立特里亚", "EE": "爱沙尼亚", "ET": "埃塞俄比亚", "FK": "福克兰群岛", "FO": "法罗群岛",
		"FJ": "斐济", "FI": "芬兰", "FR": "法国", "GF": "法属圭亚那", "PF": "法属波利尼西亚",
		"TF": "法属南部领地", "GA": "加蓬", "GM": "冈比亚", "GE": "格鲁吉亚", "DE": "德国",
		"GH": "加纳", "GI": "直布罗陀", "GR": "希腊", "GL": "格陵兰", "GD": "格林纳达",
		"GP": "瓜德罗普", "GU": "关岛", "GT": "危地马拉", "GG": "根西岛", "GN": "几内亚",
		"GW": "几内亚比绍", "GY": "圭亚那", "HT": "海地", "VA": "梵蒂冈", "HN": "洪都拉斯",
		"HK": "香港", "HU": "匈牙利", "IS": "冰岛", "IN": "印度", "ID": "印度尼西亚",
		"IR": "伊朗", "IQ": "伊拉克", "IE": "爱尔兰", "IM": "马恩岛", "IL": "以色列",
		"IT": "意大利", "JM": "牙买加", "JP": "日本", "JE": "泽西岛", "JO": "约旦",
		"KZ": "哈萨克斯坦", "KE": "肯尼亚", "KI": "基里巴斯", "XK": "科索沃", "KW": "科威特",
		"KG": "吉尔吉斯斯坦", "LA": "老挝", "LV": "拉脱维亚", "LB": "黎巴嫩", "LS": "莱索托",
		"LR": "利比里亚", "LY": "利比亚", "LI": "列支敦士登", "LT": "立陶宛", "LU": "卢森堡",
		"MO": "澳门", "MK": "北马其顿", "MG": "马达加斯加", "MW": "马拉维", "MY": "马来西亚",
		"MV": "马尔代夫", "ML": "马里", "MT": "马耳他", "MH": "马绍尔群岛", "MQ": "马提尼克",
		"MR": "毛里塔尼亚", "MU": "毛里求斯", "YT": "马约特", "MX": "墨西哥", "FM": "密克罗尼西亚",
		"MD": "摩尔多瓦", "MC": "摩纳哥", "MN": "蒙古", "ME": "黑山", "MS": "蒙特塞拉特",
		"MA": "摩洛哥", "MZ": "莫桑比克", "MM": "缅甸", "NA": "纳米比亚", "NR": "瑙鲁",
		"NP": "尼泊尔", "NL": "荷兰", "NC": "新喀里多尼亚", "NZ": "新西兰", "NI": "尼加拉瓜",
		"NE": "尼日尔", "NG": "尼日利亚", "NU": "纽埃", "NF": "诺福克岛", "KP": "朝鲜",
		"MP": "北马里亚纳群岛", "NO": "挪威", "OM": "阿曼", "PK": "巴基斯坦", "PW": "帕劳",
		"PS": "巴勒斯坦", "PA": "巴拿马", "PG": "巴布亚新几内亚", "PY": "巴拉圭", "PE": "秘鲁",
		"PH": "菲律宾", "PN": "皮特凯恩群岛", "PL": "波兰", "PT": "葡萄牙", "PR": "波多黎各",
		"QA": "卡塔尔", "CG": "刚果共和国", "RE": "留尼汪", "RO": "罗马尼亚", "RU": "俄罗斯",
		"RW": "卢旺达", "BL": "圣巴泰勒米", "SH": "圣赫勒拿", "KN": "圣基茨和内维斯", "LC": "圣卢西亚",
		"MF": "法属圣马丁", "PM": "圣皮埃尔和密克隆", "VC": "圣文森特和格林纳丁斯", "WS": "萨摩亚",
		"SM": "圣马力诺", "ST": "圣多美和普林西比", "SA": "沙特阿拉伯", "SN": "塞内加尔",
		"RS": "塞尔维亚", "SC": "塞舌尔", "SL": "塞拉利昂", "SG": "新加坡", "SX": "荷属圣马丁",
		"SK": "斯洛伐克", "SI": "斯洛文尼亚", "SB": "所罗门群岛", "SO": "索马里", "ZA": "南非",
		"GS": "南乔治亚岛和南桑威奇群岛", "KR": "韩国", "SS": "南苏丹", "ES": "西班牙",
		"LK": "斯里兰卡", "SD": "苏丹", "SR": "苏里南", "SJ": "斯瓦尔巴群岛和扬马延",
		"SZ": "斯威士兰", "SE": "瑞典", "CH": "瑞士", "SY": "叙利亚", "TW": "台湾",
		"TJ": "塔吉克斯坦", "TZ": "坦桑尼亚", "TH": "泰国", "TG": "多哥", "TK": "托克劳",
		"TO": "汤加", "TT": "特立尼达和多巴哥", "TN": "突尼斯", "TR": "土耳其", "TM": "土库曼斯坦",
		"TC": "特克斯和凯科斯群岛", "TV": "图瓦卢", "UG": "乌干达", "UA": "乌克兰",
		"AE": "阿拉伯联合酋长国", "GB": "英国", "US": "美国", "UY": "乌拉圭",
		"UZ": "乌兹别克斯坦", "VU": "瓦努阿图", "VE": "委内瑞拉", "VN": "越南",
		"WF": "瓦利斯和富图纳", "EH": "西撒哈拉", "YE": "也门", "ZM": "赞比亚", "ZW": "津巴布韦",
		"UNKNOWN": "未知",
	}

	// COUNTRY_FLAG_MAP 存储国家代码到国旗表情的映射
	COUNTRY_FLAG_MAP = map[string]string{
		"AD": "🇦🇩", "AE": "🇦🇪", "AF": "🇦🇫", "AG": "🇦🇬", "AI": "🇦🇮", "AL": "🇦🇱", "AM": "🇦🇲", "AO": "🇦🇴",
		"AQ": "🇦🇶", "AR": "🇦🇷", "AS": "🇦🇸", "AT": "🇦🇹", "AU": "🇦🇺", "AW": "🇦🇼", "AX": "🇦🇽", "AZ": "🇦🇿",
		"BA": "🇧🇦", "BB": "🇧🇧", "BD": "🇧🇩", "BE": "🇧🇪", "BF": "🇧🇫", "BG": "🇧🇬", "BH": "🇧🇭", "BI": "🇧🇮",
		"BJ": "🇧🇯", "BL": "🇧🇱", "BM": "🇧🇲", "BN": "🇧🇳", "BO": "🇧🇴", "BQ": "🇧🇶", "BR": "🇧🇷", "BS": "🇧🇸",
		"BT": "🇧🇹", "BV": "🇧🇻", "BW": "🇧🇼", "BY": "🇧🇾", "BZ": "🇧🇿", "CA": "🇨🇦", "CC": "🇨🇨", "CD": "🇨🇩",
		"CF": "🇨🇫", "CG": "🇨🇬", "CH": "🇨🇭", "CI": "🇨🇮", "CK": "🇨🇰", "CL": "🇨🇱", "CM": "🇨🇲", "CN": "🇨🇳",
		"CO": "🇨🇴", "CR": "🇨🇷", "CU": "🇨🇺", "CV": "🇨🇻", "CW": "🇨🇼", "CX": "🇨🇽", "CY": "🇨🇾", "CZ": "🇨🇿",
		"DE": "🇩🇪", "DJ": "🇩🇯", "DK": "🇩🇰", "DM": "🇩🇲", "DO": "🇩🇴", "DZ": "🇩🇿", "EC": "🇪🇨", "EE": "🇪🇪",
		"EG": "🇪🇬", "EH": "🇪🇭", "ER": "🇪🇷", "ES": "🇪🇸", "ET": "🇪🇹", "FI": "🇫🇮", "FJ": "🇫🇯", "FK": "🇫🇰",
		"FM": "🇫🇲", "FO": "🇫🇴", "FR": "🇫🇷", "GA": "🇬🇦", "GB": "🇬🇧", "GD": "🇬🇩", "GE": "🇬🇪", "GF": "🇬🇫",
		"GG": "🇬🇬", "GH": "🇬🇭", "GI": "🇬🇮", "GL": "🇬🇱", "GM": "🇬🇲", "GN": "🇬🇳", "GP": "🇬🇵", "GQ": "🇬🇶",
		"GR": "🇬🇷", "GS": "🇬🇸", "GT": "🇬🇹", "GU": "🇬🇺", "GW": "🇬🇼", "GY": "🇬🇾", "HK": "🇭🇰", "HM": "🇭🇲",
		"HN": "🇭🇳", "HR": "🇭🇷", "HT": "🇭🇹", "HU": "🇭🇺", "ID": "🇮🇩", "IE": "🇮🇪", "IL": "🇮🇱", "IM": "🇮🇲",
		"IN": "🇮🇳", "IO": "🇮🇴", "IQ": "🇮🇶", "IR": "🇮🇷", "IS": "🇮🇸", "IT": "🇮🇹", "JE": "🇯🇪", "JM": "🇯🇲",
		"JO": "🇯🇴", "JP": "🇯🇵", "KE": "🇰🇪", "KG": "🇰🇬", "KH": "🇰🇭", "KI": "🇰🇮", "KM": "🇰🇲", "KN": "🇰🇳",
		"KP": "🇰🇵", "KR": "🇰🇷", "KW": "🇰🇼", "KY": "🇰🇾", "KZ": "🇰🇿", "LA": "🇱🇦", "LB": "🇱🇧", "LC": "🇱🇨",
		"LI": "🇱🇮", "LK": "🇱🇰", "LR": "🇱🇷", "LS": "🇱🇸", "LT": "🇱🇹", "LU": "🇱🇺", "LV": "🇱🇻", "LY": "🇱🇾",
		"MA": "🇲🇦", "MC": "🇲🇨", "MD": "🇲🇩", "ME": "🇲🇪", "MF": "🇲🇫", "MG": "🇲🇬", "MH": "🇲🇭", "MK": "🇲🇰",
		"ML": "🇲🇱", "MM": "🇲🇲", "MN": "🇲🇳", "MO": "🇲🇴", "MP": "🇲🇵", "MQ": "🇲🇶", "MR": "🇲🇷", "MS": "🇲🇸",
		"MT": "🇲🇹", "MU": "🇲🇺", "MV": "🇲🇻", "MW": "🇲🇼", "MX": "🇲🇽", "MY": "🇲🇾", "MZ": "🇲🇿", "NA": "🇳🇦",
		"NC": "🇳🇨", "NE": "🇳🇪", "NF": "🇳🇫", "NG": "🇳🇬", "NI": "🇳🇮", "NL": "🇳🇱", "NO": "🇳🇴", "NP": "🇳🇵",
		"NR": "🇳🇷", "NU": "🇳🇺", "NZ": "🇳🇿", "OM": "🇴🇲", "PA": "🇵🇦", "PE": "🇵🇪", "PF": "🇵🇫", "PG": "🇵🇬",
		"PH": "🇵🇭", "PK": "🇵🇰", "PL": "🇵🇱", "PM": "🇵🇲", "PN": "🇵🇳", "PR": "🇵🇷", "PS": "🇵🇸", "PT": "🇵🇹",
		"PW": "🇵🇼", "PY": "🇵🇾", "QA": "🇶🇦", "RE": "🇷🇪", "RO": "🇷🇴", "RU": "🇷🇺", "RW": "🇷🇼",
		"SA": "🇸🇦", "SB": "🇸🇬", "SC": "🇸🇨", "SD": "🇸🇩", "SE": "🇸🇪", "SG": "🇸🇬", "SH": "🇸🇭", "SI": "🇸🇮",
		"SJ": "🇸🇯", "SK": "🇸🇰", "SL": "🇸🇱", "SM": "🇸🇲", "SN": "🇸🇳", "SO": "🇸🇴", "SR": "🇸🇷", "SS": "🇸🇸",
		"ST": "🇸🇹", "SV": "🇸🇻", "SX": "🇸🇽", "SY": "🇸🇾", "SZ": "🇸🇿", "TC": "🇹🇨", "TD": "🇹🇩", "TF": "🇹🇫",
		"TG": "🇹🇬", "TH": "🇹🇭", "TJ": "🇹🇯", "TK": "🇹🇰", "TL": "🇹🇱", "TM": "🇹🇲", "TN": "🇹🇳", "TO": "🇹🇴",
		"TR": "🇹🇷", "TT": "🇹🇹", "TV": "🇹🇻", "UG": "🇺🇬", "UM": "🇺🇲", "US": "🇺🇸", "UY": "🇺🇾", "UZ": "🇺🇿",
		"VA": "🇻🇦", "VC": "🇻🇨", "VE": "🇻🇪", "VG": "🇻🇬", "VI": "🇻🇮", "VN": "🇻🇳", "VU": "🇻🇺", "WF": "🇼🇫",
		"WS": "🇼🇸", "XK": "🇽🇰", "YE": "🇾🇹", "YT": "🇾🇹", "ZA": "🇿🇦", "ZM": "🇿🇲", "ZW": "🇿🇼", "UNKNOWN": "🌐",
	}

	// IP_TYPE_MAP 存储IP类型到图标的映射
	IP_TYPE_MAP = map[string]string{
		"datacenter": "🖥️",
		"business":   "🏢",
		"residential": "🏠",
		"mobile":     "📱",
		"education":  "🎓",
		"isp":        "🌐",
		"hosting":    "🖥️",
		"vpn":        "🔒",
		"proxy":      "🔗",
		"unknown":    "❓",
	}

	// IP_TYPE_DESCRIPTION 存储IP类型描述
	IP_TYPE_DESCRIPTION = map[string]string{
		"datacenter":   "数据中心IP",
		"business":     "商业IP",
		"residential":  "住宅IP",
		"mobile":       "移动IP",
		"education":    "教育IP",
		"isp":          "ISP网络",
		"hosting":      "主机IP",
		"vpn":          "VPN网络",
		"proxy":        "代理网络",
		"unknown":      "未知类型",
	}

	// FAILURE_REASON_MAP 定义失败原因的规范化映射
	FAILURE_REASON_MAP = map[string]string{
		"EOF":                            "连接中断",
		"read: connection reset by peer": "连接被重置",
		"context deadline exceeded":      "操作超时",
		"connect: connection refused":    "连接被拒",
		"dial tcp":                      "连接失败 (TCP)",
		"lookup":                        "DNS解析失败",
		"no route to host":              "主机不可达",
		"connection was reset":           "连接重置",
		"i/o timeout":                   "I/O超时",
		"tls: handshake failure":         "TLS握手失败",
		"tls: internal error":            "TLS内部错误",
		"connection abort":              "连接异常中断",
		"proxy connect tcp":             "代理连接失败",
		"Bad Request":                   "请求错误 (Bad Request)",
	}
)

// ProxyInfo 结构体用于存储解析出的代理信息
type ProxyInfo struct {
	URL      string
	Protocol string
	Reason   string // 仅用于初始解析阶段
}

// ProxyResult 结构体用于存储检测结果
type ProxyResult struct {
	URL       string
	Protocol  string
	Latency   float64
	Success   bool
	IP        string
	IPType    string
	IPDetails string
	Reason    string
}

// Telegram API 响应结构体
type telegramAPIResponse struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
}

// IPTypeDetectionResponse IP类型检测API响应结构体
type IPTypeDetectionResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	Region      string `json:"region"`
	RegionName  string `json:"regionName"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string `json:"timezone"`
	ISP         string `json:"isp"`
	ORG         string `json:"org"`
	AS          string `json:"as"`
	Query       string `json:"query"`
}

// IPInfoResponse IPInfo API响应结构体
type IPInfoResponse struct {
	IP       string            `json:"ip"`
	Hostname string            `json:"hostname"`
	City     string            `json:"city"`
	Region   string            `json:"region"`
	Country  string            `json:"country"`
	Loc      string            `json:"loc"`
	Org      string            `json:"org"`
	Postal   string            `json:"postal"`
	Timezone string            `json:"timezone"`
	Readme   string            `json:"readme"`
	Type     string            `json:"type"`
}

// GeoIPManager 结构体用于封装 GeoIP Reader 和缓存
type GeoIPManager struct {
	reader *geoip2.Reader
	mu     sync.RWMutex
	cache  map[string]string
}

// geoIPManager 是 GeoIPManager 的全局实例
var geoIPManager = &GeoIPManager{
	cache: make(map[string]string),
}

// telegramClientCache 缓存一个已验证的 Telegram 客户端，避免重复验证
var (
	telegramClientCache *http.Client
	clientCacheMutex    sync.Mutex
)

// failedProxiesCache 记录已知的失效代理，避免重复尝试
var (
	failedProxiesCache = make(map[string]time.Time)
	failedProxiesMutex sync.RWMutex
)

// 计算字符串在终端中的显示宽度，中文字符占2个宽度（🚫固化）
func getStringDisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		if utf8.RuneLen(r) > 1 {
			width += 2 // 中文字符等双宽字符
		} else {
			width += 1 // 英文、数字等单宽字符
		}
	}
	return width
}

// DrawCenteredTitleBox 绘制居中标题框
func DrawCenteredTitleBox(title string, width int) {
	// 1. 移除 ANSI 颜色代码，以获取纯文本
	cleanTitle := removeColorCodes([]byte(title))

	// 2. 使用新函数，准确计算标题的显示宽度
	titleDisplayWidth := getStringDisplayWidth(string(cleanTitle))

	// 3. 定义标题框内部的总显示宽度（标题 + 左右各2个空格）
	innerBoxWidth := titleDisplayWidth + 4

	// 4. 标题框总宽度 = 内部宽度 + 左右边框
	boxTotalWidth := innerBoxWidth + 2

	// 如果标题框总宽度超出终端宽度，则不居中
	if boxTotalWidth >= width {
		fmt.Println()
		fmt.Println("╔" + strings.Repeat("═", innerBoxWidth) + "╗")
		fmt.Println("║  " + title + "  ║")
		fmt.Println("╚" + strings.Repeat("═", innerBoxWidth) + "╝")
		fmt.Println()
		return
	}

	// 5. 计算左右两边的填充空格数，以实现居中
	padding := (width - boxTotalWidth) / 2
	paddingStr := strings.Repeat(" ", padding)

	// 6. 构建标题框的每一行，确保长度完全一致
	topBorder := paddingStr + "╔" + strings.Repeat("═", innerBoxWidth) + "╗"
	titleLine := paddingStr + "║  " + title + "  ║"
	bottomBorder := paddingStr + "╚" + strings.Repeat("═", innerBoxWidth) + "╝"

	fmt.Println()
	fmt.Println(topBorder)
	fmt.Println(titleLine)
	fmt.Println(bottomBorder)
	fmt.Println()
}

// loadConfig 读取配置文件并打印美化后的日志
func loadConfig(configPath string) error {
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("❌ 无法加载配置文件: %w", err)
	}

	err = cfg.MapTo(&config)
	if err != nil {
		return fmt.Errorf("❌ 无法映射配置到结构体: %w", err)
	}

	proxyStr := cfg.Section("settings").Key("preset_proxy").String()
	if proxyStr != "" {
		config.Settings.PresetProxy = strings.Split(proxyStr, ",")
	}

	// 获取终端宽度
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // 默认宽度
	}

	// 使用新的函数来绘制标题框，并将标题文本设置为黄色
	DrawCenteredTitleBox(ColorYellow+"   代 理 检 测 工 具 v1.0   "+ColorReset, width)

	// 打印美化后的配置加载成功提示
	log.Println(ColorGreen + "✅ 配置加载成功！" + ColorReset)
	if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
		log.Println(ColorCyan + "- Telegram 机器人已就绪。" + ColorReset)
	} else {
		log.Println(ColorYellow + "- Telegram 配置不完整，将跳过通知。" + ColorReset)
	}

	if len(config.Settings.PresetProxy) > 0 {
		log.Printf(ColorCyan+"- 已加载 %d 个预设代理。\n", len(config.Settings.PresetProxy))
	} else {
		log.Println(ColorYellow + "- 没有预设代理，将使用直连方式下载GeoIP数据库。" + ColorReset)
	}

	log.Printf(ColorCyan+"- 检测超时设置为 %d 秒，最大并发数 %d。\n", config.Settings.CheckTimeout, config.Settings.MaxConcurrent)
	log.Println(ColorCyan + "------------------------------------------" + ColorReset)

	return nil
}

// ========= 2. GeoIP 数据库处理函数 =========

// downloadGeoIPDatabase 尝试下载 GeoIP 数据库文件
func downloadGeoIPDatabase(dbPath string) bool {
	log.Printf("ℹ️ 正在下载 GeoIP 数据库到: %s\n", dbPath)

	// 清理过期的失效代理缓存
	cleanExpiredFailedProxies()

	// 首先尝试通过预设代理下载，跳过已知失效的代理
	for _, proxyURL := range config.Settings.PresetProxy {
		// 检查是否在失效代理缓存中
		failedProxiesMutex.RLock()
		if failTime, exists := failedProxiesCache[proxyURL]; exists {
			// 如果在30分钟内失败过，跳过这个代理
			if time.Since(failTime) < 30*time.Minute {
				failedProxiesMutex.RUnlock()
				log.Printf("⏭️ 跳过最近失效的代理 %s (剩余冷却时间: %.1f分钟)\n",
					proxyURL, (30*time.Minute-time.Since(failTime)).Minutes())
				continue
			}
		}
		failedProxiesMutex.RUnlock()

		log.Printf("⏳ 尝试通过预设代理 %s 下载 GeoIP 数据库...\n", proxyURL)

		if downloadGeoIPWithProxy(dbPath, proxyURL) {
			// 下载成功，从失效代理缓存中移除（如果之前存在）
			failedProxiesMutex.Lock()
			delete(failedProxiesCache, proxyURL)
			failedProxiesMutex.Unlock()
			return true
		}

		log.Printf("❌ 代理 %s 下载失败，尝试下一个代理\n", proxyURL)

		// 将失效代理添加到缓存
		failedProxiesMutex.Lock()
		failedProxiesCache[proxyURL] = time.Now()
		failedProxiesMutex.Unlock()
	}

	// 如果所有代理都失败或被跳过，尝试直连
	log.Printf("❌ 所有预设代理均失败或已被跳过，将尝试直连下载...\n")
	return downloadGeoIPWithProxy(dbPath, "")
}

// downloadGeoIPWithProxy 使用指定代理下载 GeoIP 数据库
func downloadGeoIPWithProxy(dbPath, proxyURL string) bool {
	// 首先清理可能存在的临时文件
	tempPath := dbPath + ".tmp"
	if _, err := os.Stat(tempPath); err == nil {
		log.Printf("🧹 清理旧的临时文件: %s\n", tempPath)
		os.Remove(tempPath)
	}

	var transport *http.Transport
	var err error

	if proxyURL == "" {
		log.Printf("🔗 使用直连方式下载\n")
		transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
		}
	} else {
		log.Printf("🔗 使用代理: %s\n", proxyURL)
		transport, err = createTransportWithProxy(proxyURL)
		if err != nil {
			log.Printf("❌ 创建代理 transport 失败: %v\n", err)
			return false
		}
	}

	// 使用更长的超时时间来下载大文件
	client := &http.Client{
		Transport: transport,
		Timeout:   300 * time.Second, // 5分钟超时
	}

	log.Printf("📥 开始下载 GeoIP 数据库...\n")
	startTime := time.Now()

	resp, err := client.Get(GEOIP_DB_URL)
	if err != nil {
		log.Printf("❌ 下载失败: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ HTTP 状态码错误: %d\n", resp.StatusCode)
		return false
	}

	// 获取文件大小用于进度显示
	contentLength := resp.ContentLength
	if contentLength > 0 {
		log.Printf("📊 文件大小: %.2f MB\n", float64(contentLength)/1024/1024)
	}

	// 创建临时文件
	outFile, err := os.Create(tempPath)
	if err != nil {
		log.Printf("❌ 创建临时文件失败: %v\n", err)
		return false
	}

	// 确保在函数退出时处理文件关闭和清理
	defer func() {
		outFile.Close()
		// 如果最终文件不存在，说明下载失败，清理临时文件
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			os.Remove(tempPath)
		}
	}()

	// 创建进度报告器
	writer := &progressWriter{
		writer:    outFile,
		total:     contentLength,
		startTime: startTime,
	}

	// 复制数据并显示进度
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		log.Printf("❌ 写入文件失败: %v\n", err)
		return false
	}

	// 确保数据写入磁盘
	if err := outFile.Sync(); err != nil {
		log.Printf("⚠️ 刷新文件到磁盘失败: %v\n", err)
	}

	duration := time.Since(startTime)
	log.Printf("✅ 下载完成！耗时: %.2f 秒，平均速度: %.2f KB/s\n",
		duration.Seconds(), float64(written)/duration.Seconds()/1024)

	// 关闭文件以便重命名
	outFile.Close()

	// 重命名为最终文件名
	if err := os.Rename(tempPath, dbPath); err != nil {
		log.Printf("❌ 重命名文件失败: %v\n", err)
		return false
	}

	// 验证下载的文件
	if isGeoIPFileValid(dbPath) {
		connectionType := "直连"
		if proxyURL != "" {
			connectionType = "代理 " + proxyURL
		}
		log.Printf("🟢 成功通过 %s 下载并验证 GeoIP 数据库\n", connectionType)
		return true
	} else {
		log.Printf("❌ 下载的文件验证失败，删除文件\n")
		os.Remove(dbPath)
		return false
	}
}

// progressWriter 用于显示下载进度的写入器
type progressWriter struct {
	writer    io.Writer
	total     int64
	written   int64
	startTime time.Time
	lastLog   time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}

	pw.written += int64(n)

	// 每5秒更新一次进度
	now := time.Now()
	if now.Sub(pw.lastLog) >= 5*time.Second {
		if pw.total > 0 {
			percent := float64(pw.written) / float64(pw.total) * 100
			speed := float64(pw.written) / now.Sub(pw.startTime).Seconds() / 1024
			log.Printf("📈 下载进度: %.1f%% (%.2f MB/%.2f MB), 速度: %.2f KB/s\n",
				percent, float64(pw.written)/1024/1024, float64(pw.total)/1024/1024, speed)
		} else {
			speed := float64(pw.written) / now.Sub(pw.startTime).Seconds() / 1024
			log.Printf("📈 已下载: %.2f MB, 速度: %.2f KB/s\n",
				float64(pw.written)/1024/1024, speed)
		}
		pw.lastLog = now
	}

	return n, nil
}

// isGeoIPFileValid 验证 GeoIP 数据库文件是否有效且未过期
func isGeoIPFileValid(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	fileInfo, _ := os.Stat(filePath)
	if fileInfo.Size() < 1024*1024 { // 1MB
		log.Printf("⚠️ GeoIP 数据库文件 %s 过小，可能无效。\n", filePath)
		return false
	}
	mtime := fileInfo.ModTime()
	ageDays := time.Since(mtime).Hours() / 24
	if ageDays > 30 {
		log.Printf("⚠️ GeoIP 数据库文件 %s 已超过 30 天 (%.1f 天)，建议更新。\n", filePath, ageDays)
	}

	reader, err := geoip2.Open(filePath)
	if err != nil {
		log.Printf("❌ GeoIP 数据库文件 %s 验证失败: %v\n", filePath, err)
		return false
	}
	defer reader.Close()

	ip := net.ParseIP("8.8.8.8")
	if ip == nil {
		return false
	}
	country, err := reader.Country(ip)
	if err != nil {
		log.Printf("❌ GeoIP 数据库测试失败: %v\n", err)
		return false
	}
	if country.Country.IsoCode != "" {
		log.Printf("✅ GeoIP 数据库测试成功，IP %s -> %s\n", ip, country.Country.IsoCode)
		return true
	}
	log.Printf("❌ GeoIP 数据库测试失败，IP %s 无国家代码。\n", ip)
	return false
}

// initGeoIPReader 初始化 GeoIP 数据库读取器
func initGeoIPReader() {
	log.Println("----------- GeoIP 数据库初始化 -----------")

	// 检查本地数据库文件是否存在且有效
	if fileInfo, err := os.Stat(GEOIP_DB_PATH); err == nil {
		if isGeoIPFileValid(GEOIP_DB_PATH) {
			// 文件存在且有效，检查是否需要更新（基于文件年龄）
			mtime := fileInfo.ModTime()
			ageDays := time.Since(mtime).Hours() / 24

			if ageDays <= 7 {
				// 7天内的文件被认为是新鲜的，直接使用
				log.Printf("✅ 本地 GeoIP 数据库已存在且新鲜 (%.1f天): %s\n", ageDays, GEOIP_DB_PATH)
			} else if ageDays <= 30 {
				// 7-30天的文件仍然可用，但提示更新
				log.Printf("✅ 本地 GeoIP 数据库可用但较旧 (%.1f天): %s\n", ageDays, GEOIP_DB_PATH)
				log.Printf("ℹ️ 建议更新数据库以获得更准确的地理位置信息\n")
			} else {
				// 超过30天的文件，建议重新下载
				log.Printf("⚠️ 本地 GeoIP 数据库已过期 (%.1f天): %s，将重新下载。\n", ageDays, GEOIP_DB_PATH)
				os.Remove(GEOIP_DB_PATH)
				if !downloadGeoIPDatabase(GEOIP_DB_PATH) {
					log.Printf("❌ 下载 GeoIP 数据库失败，地理位置查询将不可用。\n")
					log.Println("------------------------------------------")
					return
				}
			}
		} else {
			// 文件存在但无效，删除并重新下载
			log.Printf("⚠️ 本地 GeoIP 数据库损坏或无效: %s，将重新下载。\n", GEOIP_DB_PATH)
			os.Remove(GEOIP_DB_PATH)
			if !downloadGeoIPDatabase(GEOIP_DB_PATH) {
				log.Printf("❌ 下载 GeoIP 数据库失败，地理位置查询将不可用。\n")
				log.Println("------------------------------------------")
				return
			}
		}
	} else {
		// 文件不存在，需要下载
		log.Printf("ℹ️ 本地 GeoIP 数据库不存在: %s，开始下载最新文件。\n", GEOIP_DB_PATH)
		if !downloadGeoIPDatabase(GEOIP_DB_PATH) {
			log.Printf("❌ 下载 GeoIP 数据库失败，地理位置查询将不可用。\n")
			log.Println("------------------------------------------")
			return
		}
	}

	// 加载数据库到内存
	reader, err := geoip2.Open(GEOIP_DB_PATH)
	if err != nil {
		log.Printf("❌ GeoIP 数据库加载失败: %v。地理位置查询将不可用。\n", err)
		log.Println("------------------------------------------")
		return
	}
	geoIPManager.reader = reader
	log.Println("✅ GeoIP 数据库加载成功。")
	log.Println("------------------------------------------")
}

// closeGeoIPReader 关闭 GeoIP 数据库读取器
func closeGeoIPReader() {
	if geoIPManager.reader != nil {
		if err := geoIPManager.reader.Close(); err != nil {
			log.Printf("⚠️ 关闭 GeoIP 数据库失败: %v\n", err)
		} else {
			log.Println("ℹ️ GeoIP 数据库已关闭。")
		}
		geoIPManager.reader = nil
	}
}

// getCountryFromIPBatch 批量查询 IP 的国家代码
func getCountryFromIPBatch(ips []string) map[string]string {
	results := make(map[string]string)
	if geoIPManager.reader == nil {
		log.Printf("⚠️ GeoIP 数据库未加载，无法查询国家信息。\n")
		for _, ip := range ips {
			results[ip] = "UNKNOWN"
		}
		return results
	}

	for _, ipStr := range ips {
		geoIPManager.mu.RLock()
		if code, ok := geoIPManager.cache[ipStr]; ok {
			results[ipStr] = code
			geoIPManager.mu.RUnlock()
			continue
		}
		geoIPManager.mu.RUnlock()

		ip := net.ParseIP(ipStr)
		if ip == nil {
			results[ipStr] = "UNKNOWN"
			continue
		}
		country, err := geoIPManager.reader.Country(ip)
		if err != nil {
			results[ipStr] = "UNKNOWN"
			continue
		}
		countryCode := country.Country.IsoCode
		if _, ok := COUNTRY_FLAG_MAP[countryCode]; !ok {
			countryCode = "UNKNOWN"
		}
		results[ipStr] = countryCode

		geoIPManager.mu.Lock()
		geoIPManager.cache[ipStr] = countryCode
		geoIPManager.mu.Unlock()
	}
	return results
}

// getCountryFromIP 查询单个IP的国家代码
func getCountryFromIP(ip string) string {
	if geoIPManager.reader == nil {
		return "UNKNOWN"
	}

	// 检查缓存
	geoIPManager.mu.RLock()
	if code, ok := geoIPManager.cache[ip]; ok {
		geoIPManager.mu.RUnlock()
		return code
	}
	geoIPManager.mu.RUnlock()

	// 解析IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "UNKNOWN"
	}

	// 查询国家
	country, err := geoIPManager.reader.Country(parsedIP)
	if err != nil {
		return "UNKNOWN"
	}

	countryCode := country.Country.IsoCode
	if _, ok := COUNTRY_FLAG_MAP[countryCode]; !ok {
		countryCode = "UNKNOWN"
	}

	// 缓存结果
	geoIPManager.mu.Lock()
	geoIPManager.cache[ip] = countryCode
	geoIPManager.mu.Unlock()

	return countryCode
}

// ========= 3. IP类型检测函数 =========

// IPTypeInfo IP类型信息结构体
type IPTypeInfo struct {
	Type    string
	Details string
	Org     string
	ISP     string
}

// detectIPType 检测IP类型
func detectIPType(ip string) IPTypeInfo {
	if !config.IPDetection.Enabled {
		return IPTypeInfo{Type: "unknown", Details: "未启用IP类型检测"}
	}

	// 尝试不同的IP类型检测服务
	for _, service := range config.IPDetection.Services {
		var info IPTypeInfo
		var err error

		switch strings.ToLower(service) {
		case "ipinfo":
			info, err = detectIPTypeWithIPInfo(ip)
		case "ipapi":
			info, err = detectIPTypeWithIPAPI(ip)
		case "ipapis":
			info, err = detectIPTypeWithIPApis(ip)
		case "ipregistry":
			info, err = detectIPTypeWithIPRegistry(ip)
		default:
			log.Printf("⚠️ 未知的IP检测服务: %s\n", service)
			continue
		}

		if err == nil && info.Type != "unknown" {
			return info
		}
		// 如果该服务失败，记录日志并尝试下一个服务
		if err != nil {
			log.Printf("⚠️ IP类型检测服务 %s 失败: %v\n", service, err)
		}
	}

	return IPTypeInfo{Type: "unknown", Details: "无法检测IP类型"}
}

// detectIPTypeWithIPInfo 使用IPInfo.io检测IP类型
func detectIPTypeWithIPInfo(ip string) (IPTypeInfo, error) {
	if config.IPDetection.IPInfoToken == "" {
		return IPTypeInfo{}, fmt.Errorf("IPInfo Token未配置")
	}

	client := &http.Client{Timeout: time.Duration(config.IPDetection.Timeout) * time.Second}
	url := fmt.Sprintf("https://ipinfo.io/%s/json?token=%s", ip, config.IPDetection.IPInfoToken)

	resp, err := client.Get(url)
	if err != nil {
		return IPTypeInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPTypeInfo{}, fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	var ipInfoResp IPInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&ipInfoResp); err != nil {
		return IPTypeInfo{}, err
	}

	// 解析IP类型
	ipType := analyzeIPType(ipInfoResp.Org, ipInfoResp.Type)
	details := ipInfoResp.Org
	if details == "" {
		details = "未知组织"
	}

	return IPTypeInfo{
		Type:    ipType,
		Details: details,
		Org:     ipInfoResp.Org,
		ISP:     ipInfoResp.Org,
	}, nil
}

// detectIPTypeWithIPAPI 使用IPAPI.com检测IP类型
func detectIPTypeWithIPAPI(ip string) (IPTypeInfo, error) {
	client := &http.Client{Timeout: time.Duration(config.IPDetection.Timeout) * time.Second}
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,query", ip)

	resp, err := client.Get(url)
	if err != nil {
		return IPTypeInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPTypeInfo{}, fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	var ipapiResp IPTypeDetectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&ipapiResp); err != nil {
		return IPTypeInfo{}, err
	}

	if ipapiResp.Status != "success" {
		return IPTypeInfo{}, fmt.Errorf("API响应失败: %s", ipapiResp.Message)
	}

	// 解析IP类型
	ipType := analyzeIPType(ipapiResp.ORG, "")
	details := ipapiResp.ORG
	if details == "" {
		details = "未知组织"
	}

	return IPTypeInfo{
		Type:    ipType,
		Details: details,
		Org:     ipapiResp.ORG,
		ISP:     ipapiResp.ISP,
	}, nil
}

// detectIPTypeWithIPApis 使用IPApis.com检测IP类型
func detectIPTypeWithIPApis(ip string) (IPTypeInfo, error) {
	client := &http.Client{Timeout: time.Duration(config.IPDetection.Timeout) * time.Second}
	url := fmt.Sprintf("http://ipapis.com/%s", ip)

	resp, err := client.Get(url)
	if err != nil {
		return IPTypeInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPTypeInfo{}, fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return IPTypeInfo{}, err
	}

	// 提取相关信息
	org, _ := result["organization"].(string)
	isp, _ := result["isp"].(string)

	// 解析IP类型
	ipType := analyzeIPType(org, "")
	details := org
	if details == "" {
		details = "未知组织"
	}

	return IPTypeInfo{
		Type:    ipType,
		Details: details,
		Org:     org,
		ISP:     isp,
	}, nil
}

// detectIPTypeWithIPRegistry 使用IPRegistry.co检测IP类型
func detectIPTypeWithIPRegistry(ip string) (IPTypeInfo, error) {
	if config.IPDetection.IPRegistryKey == "" {
		return IPTypeInfo{}, fmt.Errorf("IPRegistry Key未配置")
	}

	client := &http.Client{Timeout: time.Duration(config.IPDetection.Timeout) * time.Second}
	url := fmt.Sprintf("https://api.ipregistry.co/%s?key=%s", ip, config.IPDetection.IPRegistryKey)

	resp, err := client.Get(url)
	if err != nil {
		return IPTypeInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPTypeInfo{}, fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return IPTypeInfo{}, err
	}

	// 提取相关信息
	if connection, ok := result["connection"].(map[string]interface{}); ok {
		if organization, ok := connection["organization"].(string); ok {
			// 解析IP类型
			ipType := analyzeIPType(organization, "")
			details := organization
			if details == "" {
				details = "未知组织"
			}

			return IPTypeInfo{
				Type:    ipType,
				Details: details,
				Org:     organization,
				ISP:     organization,
			}, nil
		}
	}

	return IPTypeInfo{}, fmt.Errorf("无法解析IPRegistry响应")
}

// analyzeIPType 根据API返回的类型确定IP类型
func analyzeIPType(org, apiType string) string {
	// 首先检查API明确提供的类型
	if apiType != "" {
		apiTypeLower := strings.ToLower(apiType)
		switch apiTypeLower {
		case "datacenter", "hosting":
			return "datacenter"
		case "business":
			return "business"
		case "residential":
			return "residential"
		case "mobile":
			return "mobile"
		case "education":
			return "education"
		case "isp":
			return "business"
		}
	}

	// 如果API没有明确类型，通过组织信息推断
	if org != "" {
		orgLower := strings.ToLower(org)

		// 数据中心关键词
		datacenterKeywords := []string{
			"datacenter", "hosting", "server", "cloud", "dedicated",
			"vps", "vpn", "proxy", "colo", "colocation", "idc",
			"internet data center", "web hosting", "virtual server",
			"amazon", "google", "microsoft", "oracle", "alibaba",
			"aws", "gcp", "azure", "digitalocean", "vultr", "linode",
			"hetzner", "ovh", "scaleway", "rackspace", "ibm",
		}

		// 移动网络关键词
		mobileKeywords := []string{
			"mobile", "wireless", "cellular", "gsm", "3g", "4g", "5g",
			"lte", "wcdma", "umts", "cell", "phone", "smartphone",
		}

		// 教育机构关键词
		educationKeywords := []string{
			"university", "college", "education", "academic", "school",
			"educational", "research", "institute", "campus",
		}

		// 检查数据中心
		for _, keyword := range datacenterKeywords {
			if strings.Contains(orgLower, keyword) {
				return "datacenter"
			}
		}

		// 检查移动网络
		for _, keyword := range mobileKeywords {
			if strings.Contains(orgLower, keyword) {
				return "mobile"
			}
		}

		// 检查教育机构
		for _, keyword := range educationKeywords {
			if strings.Contains(orgLower, keyword) {
				return "education"
			}
		}

		// 检查是否包含ISP相关的商业关键词
		businessKeywords := []string{
			"corporation", "corp", "company", "ltd", "limited", "inc",
			"enterprise", "business", "commercial", "isp", "telecom",
		}

		for _, keyword := range businessKeywords {
			if strings.Contains(orgLower, keyword) {
				return "business"
			}
		}
	}

	// 默认返回residential，因为大多数IP都是住宅类型的
	return "residential"
}

// detectIPTypeBatch 批量检测IP类型
func detectIPTypeBatch(ips []string) map[string]IPTypeInfo {
	if !config.IPDetection.Enabled {
		result := make(map[string]IPTypeInfo)
		for _, ip := range ips {
			result[ip] = IPTypeInfo{Type: "unknown", Details: "未启用IP类型检测"}
		}
		return result
	}

	results := make(map[string]IPTypeInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 控制并发数
	semaphore := make(chan struct{}, config.IPDetection.MaxConcurrent)

	for _, ip := range ips {
		wg.Add(1)
		go func(ipAddr string) {
			defer wg.Done()
			semaphore <- struct{}{} // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			info := detectIPType(ipAddr)

			mu.Lock()
			results[ipAddr] = info
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	return results
}

// ========= 5. 最优代理选择和配置更新函数 =========

// ProxyScore 代理评分结构体
type ProxyScore struct {
	Proxy   ProxyResult
	Score   float64
	Reason  string
}

// calculateProxyScore 计算代理综合评分
func calculateProxyScore(proxy ProxyResult) ProxyScore {
	score := 1000.0 // 基础分数
	reason := "基础评分"

	// 延迟评分（延迟越低分数越高）
	latencyScore := 1000.0 / (proxy.Latency + 1) // 避免除零
	score += latencyScore
	reason += fmt.Sprintf(", 延迟加分: %.1f", latencyScore)

	// IP类型评分
	switch proxy.IPType {
	case "residential":
		score += 500
		reason += ", 住宅IP +500"
	case "mobile":
		score += 400
		reason += ", 移动IP +400"
	case "business":
		score += 300
		reason += ", 商业IP +300"
	case "datacenter":
		score += 200
		reason += ", 数据中心IP +200"
	default:
		reason += ", 未知类型 +0"
	}

	// 地理位置加分（中国IP额外加分）
	if proxy.IPDetails == "CN" || strings.Contains(proxy.IPDetails, "中国") {
		score += 100
		reason += ", 中国IP +100"
	}

	// 协议类型加分
	switch proxy.Protocol {
	case "socks5_noauth", "socks5_auth":
		score += 50
		reason += ", SOCKS5 +50"
	case "https":
		score += 30
		reason += ", HTTPS +30"
	case "http":
		score += 20
		reason += ", HTTP +20"
	}

	return ProxyScore{
		Proxy:  proxy,
		Score:  score,
		Reason: reason,
	}
}

// selectBestProxies 选择最优代理列表
func selectBestProxies(validProxies []ProxyResult, maxCount int, preferResidential bool, maxLatency float64) []ProxyScore {
	var scoredProxies []ProxyScore

	// 计算每个代理的评分
	for _, proxy := range validProxies {
		// 检查延迟限制
		if maxLatency > 0 && proxy.Latency > maxLatency {
			continue
		}

		score := calculateProxyScore(proxy)

		// 如果偏好住宅IP，给住宅IP额外加分
		if preferResidential && proxy.IPType == "residential" {
			score.Score += 200
			score.Reason += ", 住宅IP偏好 +200"
		}

		scoredProxies = append(scoredProxies, score)
	}

	// 按评分从高到低排序
	sort.Slice(scoredProxies, func(i, j int) bool {
		return scoredProxies[i].Score > scoredProxies[j].Score
	})

	// 返回前N个最优代理
	if len(scoredProxies) > maxCount {
		scoredProxies = scoredProxies[:maxCount]
	}

	return scoredProxies
}

// updateConfigPresetProxies 更新配置文件中的预设代理列表
func updateConfigPresetProxies(bestProxies []ProxyScore) error {
	if len(bestProxies) == 0 {
		log.Println("⚠️ 没有可用的代理来更新配置")
		return nil
	}

	// 验证代理列表质量
	validProxies := validateProxiesForUpdate(bestProxies)
	if len(validProxies) == 0 {
		return fmt.Errorf("没有通过验证的代理可以更新配置")
	}

	// 读取现有配置文件
	cfg, err := ini.Load("config.ini")
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 备份现有配置
	if config.AutoProxyUpdate.BackupConfig {
		backupPath := fmt.Sprintf("config_backup_%s.ini", time.Now().Format("20060102_150405"))
		if err := cfg.SaveTo(backupPath); err != nil {
			log.Printf("⚠️ 配置文件备份失败: %v\n", err)
		} else {
			log.Printf("💾 配置文件已备份到: %s\n", backupPath)
		}
	}

	// 构建新的代理列表
	var newProxies []string
	for _, scoredProxy := range validProxies {
		newProxies = append(newProxies, scoredProxy.Proxy.URL)
		log.Printf("🎯 选择代理: %s (评分: %.1f, 延迟: %.2fms, 类型: %s)\n",
			scoredProxy.Proxy.URL, scoredProxy.Score, scoredProxy.Proxy.Latency, scoredProxy.Proxy.IPType)
	}

	// 更新预设代理列表
	cfg.Section("settings").Key("preset_proxy").SetValue(strings.Join(newProxies, ","))

	// 保存配置文件
	if err := cfg.SaveTo("config.ini"); err != nil {
		return fmt.Errorf("保存配置文件失败: %v", err)
	}

	// 重新加载配置到内存
	if err := loadSecureConfig("config.ini"); err != nil {
		log.Printf("⚠️ 配置文件保存成功但重新加载失败: %v\n", err)
		// 不返回错误，因为文件已经保存成功
	}

	log.Printf("✅ 已更新预设代理列表，共 %d 个代理\n", len(newProxies))
	return nil
}

// validateProxiesForUpdate 验证代理列表是否适合更新到配置
func validateProxiesForUpdate(bestProxies []ProxyScore) []ProxyScore {
	var validProxies []ProxyScore

	// 统计协议类型分布
	protocolCount := make(map[string]int)

	for _, scoredProxy := range bestProxies {
		proxy := scoredProxy.Proxy

		// 基本有效性检查
		if proxy.Success != true {
			log.Printf("❌ 跳过无效代理: %s (测试未通过)\n", proxy.URL)
			continue
		}

		// 延迟检查（如果设置了最大延迟限制）
		if config.AutoProxyUpdate.MaxLatency > 0 && proxy.Latency > config.AutoProxyUpdate.MaxLatency {
			log.Printf("❌ 跳过高延迟代理: %s (%.2fms > %.2fms)\n",
				proxy.URL, proxy.Latency, config.AutoProxyUpdate.MaxLatency)
			continue
		}

		// IP类型偏好检查
		if config.AutoProxyUpdate.PreferResidential && proxy.IPType != "residential" {
			log.Printf("⚠️ 代理不是住宅类型: %s (类型: %s)，但仍然包含\n", proxy.URL, proxy.IPType)
		}

		// 协议检查
		if !isSupportedProtocol(proxy.Protocol) {
			log.Printf("❌ 跳过不支持的协议: %s (协议: %s)\n", proxy.URL, proxy.Protocol)
			continue
		}

		validProxies = append(validProxies, scoredProxy)
		protocolCount[proxy.Protocol]++
	}

	// 打印协议统计
	log.Printf("📊 新代理列表协议分布:\n")
	for protocol, count := range protocolCount {
		log.Printf("  - %s: %d 个\n", protocol, count)
	}

	return validProxies
}

// isSupportedProtocol 检查协议是否被支持
func isSupportedProtocol(protocol string) bool {
	supportedProtocols := []string{
		"socks5_auth", "socks5_noauth",
		"socks4_auth", "socks4_noauth",
		"http", "https",
	}

	for _, supported := range supportedProtocols {
		if protocol == supported {
			return true
		}
	}
	return false
}

// testNewPresets 测试新的预设代理列表
func testNewPresets(newProxies []string) bool {
	log.Println("🧪 正在测试新的预设代理列表...")

	successCount := 0
	for i, proxyURL := range newProxies {
		log.Printf("测试代理 %d/%d: %s\n", i+1, len(newProxies), proxyURL)

		// 创建代理测试客户端
		client, err := createTelegramClientWithProxy(proxyURL)
		if err != nil {
			log.Printf("❌ 代理 %s 测试失败: %v\n", proxyURL, err)
			continue
		}

		// 简单的API测试
		testURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", config.Telegram.BotToken)
		resp, err := client.Get(testURL)
		if err != nil {
			log.Printf("❌ 代理 %s API测试失败: %v\n", proxyURL, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Printf("✅ 代理 %s 测试成功\n", proxyURL)
			successCount++
		} else {
			log.Printf("❌ 代理 %s API返回错误: %d\n", proxyURL, resp.StatusCode)
		}
	}

	successRate := float64(successCount) / float64(len(newProxies)) * 100
	log.Printf("📊 预设代理测试完成: %d/%d 成功 (%.1f%%)\n",
		successCount, len(newProxies), successRate)

	return successRate >= 50.0 // 至少50%成功率
}

// ========= 6. 代理解析和测试函数 =========

// extractProxiesFromFile 从指定目录的txt文件中提取代理
func extractProxiesFromFile(dir string, maxGoRoutines int) chan *ProxyInfo {
	proxiesChan := make(chan *ProxyInfo, maxGoRoutines*2)

	// 定义各种正则表达式来匹配不同的代理格式
	reAuthSocks5 := regexp.MustCompile(`^([\d.]+):(\d+)\s*\|\s*([^|]*?):([^|]*?)\s*\|.*$`)

	// 新增正则表达式
	reIPPort := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}):(\d+)$`)                                    // 192.168.1.1:8080
	reIPPortAuth := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}):(\d+)\s*:\s*([^:]+):([^:]+)$`)      // 192.168.1.1:8080:user:pass
	reIPPortProtocol := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}):(\d+)\s*:\s*([a-zA-Z]+)$`)        // 192.168.1.1:8080:socks5
	reIPPortAuthProtocol := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}):(\d+)\s*:\s*([^:]+):([^:]+)\s*:\s*([a-zA-Z]+)$`) // 192.168.1.1:8080:user:pass:socks5

	// 支持空格分隔的格式
	reSpaceSeparated := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3})\s+(\d+)(?:\s+([^:]+)(?::([^:]+))?(?:\s+([a-zA-Z]+))?)?$`)

	// 支持域名格式
	reDomainPort := regexp.MustCompile(`^([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}):(\d+)$`)
	reDomainPortAuth := regexp.MustCompile(`^([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}):(\d+)\s*:\s*([^:]+):([^:]+)$`)

	// 支持特殊格式 (如：proxy.txt 中的格式)
	reSpecialFormat1 := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3})\s*-\s*(\d+)\s*-\s*([^:]+)\s*-\s*([a-zA-Z]+)$`)
	reSpecialFormat2 := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3})\s*@\s*(\d+)$`)
	reSpecialFormat3 := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3})\s*;\s*(\d+)(?:\s*;\s*([^;]+)\s*;\s*([^;]+)\s*;\s*([a-zA-Z]+))?$`)

	// 支持逗号分隔格式和其他特殊格式
	reCommaSeparated := regexp.MustCompile(`^([^,]+),\s*(\d+)(?:,\s*([^,]*),\s*([^,]*),?\s*([a-zA-Z]*))?$`)

	// 支持域名:端口:协议格式
	reDomainPortProtocol := regexp.MustCompile(`^([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}):(\d+)\s*:\s*([a-zA-Z]+)$`)

	// 混合格式支持
	reMixedFormat1 := regexp.MustCompile(`^([a-zA-Z]+)://([^\s:]+):(\d+)$`)
	reMixedFormat2 := regexp.MustCompile(`^([a-zA-Z]+)://([^:]+):([^@]+)@([^\s:]+):(\d+)$`)

	// 简单主机:端口格式
	reSimpleHostPort := regexp.MustCompile(`^([^\s:]+):(\d+)$`)

	// 支持JSON格式 (单行)
	reJSONFormat := regexp.MustCompile(`^\{[^}]*"host"\s*:\s*"([^"]+)"[^}]*"port"\s*:\s*"?(\d+)"?[^}]*\}$`)

	// IPv6支持
	reIPv6Port := regexp.MustCompile(`^\[([0-9a-fA-F:]+)\]:(\d+)$`)
	reIPv6PortAuth := regexp.MustCompile(`^\[([0-9a-fA-F:]+)\]:(\d+)\s*:\s*([^:]+):([^:]+)$`)

	// 通用格式：host:port[:user:pass[:protocol]]
	reGenericFormat := regexp.MustCompile(`^([^\s:]+(?:\[[0-9a-fA-F:]+\])?):(\d+)(?::([^:]*)(?::([^:]*))?(?::([^:]+))?)?$`)

	go func() {
		defer close(proxiesChan)
		files, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("[错误] 读取目录 %s 失败: %v\n", dir, err)
			return
		}

		var wg sync.WaitGroup
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".txt") {
				wg.Add(1)
				go func(fileName string) {
					defer wg.Done()
					filePath := filepath.Join(dir, fileName)
					f, err := os.Open(filePath)
					if err != nil {
						log.Printf("[错误] 打开文件 %s 失败: %v\n", filePath, err)
						return
					}
					defer f.Close()

					scanner := bufio.NewScanner(f)
					for scanner.Scan() {
						line := strings.TrimSpace(scanner.Text())
						if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
							continue
						}

						// 0. 预处理：移除常见的格式符号和空白
						cleanLine := strings.Map(func(r rune) rune {
							if r == '\t' || r == '\r' || r == '\n' {
								return -1
							}
							return r
						}, line)
						cleanLine = strings.TrimSpace(cleanLine)

						// 0.1. 智能过滤：跳过已处理的输出行
						if isOutputLine(cleanLine) {
							continue
						}

						// 1. 尝试以 `#` 分割并解析为 URL 格式 (socks5://user:pass@host:port#...)
						proxyURLStr := strings.SplitN(cleanLine, "#", 2)[0]
						parsedURL, err := url.Parse(proxyURLStr)
						if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
							protocol := parsedURL.Scheme
							if handleProxyURL(proxiesChan, parsedURL, protocol) {
								continue
							}
						}

						// 2. 尝试各种正则表达式格式
						if tryParseWithRegex(proxiesChan, cleanLine, reAuthSocks5, reIPPort, reIPPortAuth, reIPPortProtocol, reIPPortAuthProtocol,
							reSpaceSeparated, reDomainPort, reDomainPortAuth, reDomainPortProtocol,
							reSpecialFormat1, reSpecialFormat2, reSpecialFormat3, reCommaSeparated, reMixedFormat1, reMixedFormat2, reSimpleHostPort, reJSONFormat, reIPv6Port, reIPv6PortAuth, reGenericFormat) {
							continue
						}

						// 3. 尝试解析旧的分隔格式（兼容性）
						if tryParseOldFormat(proxiesChan, cleanLine) {
							continue
						}

						// 如果所有尝试都失败，记录警告
						log.Printf("[警告] 无法解析代理行: %s\n", line)
					}
				}(file.Name())
			}
		}
		wg.Wait()
	}()
	return proxiesChan
}

// isOutputLine 检查是否为已处理的输出行
func isOutputLine(line string) bool {
	// 检查是否包含延迟信息格式（仅检测程序生成的格式）
	if strings.Contains(line, "延迟:") && strings.Contains(line, "ms") {
		return true
	}

	// 检查是否包含国家信息格式（仅检测程序生成的格式）
	if strings.Contains(line, "国家:") && (strings.Contains(line, "🇨🇳") ||
		strings.Contains(line, "🇺🇸") || strings.Contains(line, "🇯🇵") ||
		strings.Contains(line, "🇰🇷") || strings.Contains(line, "🇩🇪") ||
		strings.Contains(line, "🇫🇷") || strings.Contains(line, "🇬🇧") ||
		strings.Contains(line, "🇮🇹") || strings.Contains(line, "🇨🇦") ||
		strings.Contains(line, "🇦🇺") || strings.Contains(line, "🇸🇬") ||
		strings.Contains(line, "🇮🇳") || strings.Contains(line, "🇷🇺") ||
		strings.Contains(line, "🇧🇷") || strings.Contains(line, "🇳🇱") ||
		strings.Contains(line, "🇹🇷") || strings.Contains(line, "🇮🇹")) {
		return true
	}

	// 检查是否为程序警告信息
	if strings.HasPrefix(line, "[警告] 无法解析代理行:") {
		return true
	}

	// 检查是否包含代理检测结果的特征（程序生成格式）
	if strings.Contains(line, ", 延迟:") && strings.Contains(line, ", 国家:") {
		return true
	}

	// 检查是否为程序状态输出（但排除包含IP和端口的行）
	if (strings.Contains(line, "✅") || strings.Contains(line, "❌") ||
		strings.Contains(line, "⚠️") || strings.Contains(line, "📊")) &&
		!strings.Contains(line, ":") && !strings.Contains(line, "|") {
		return true
	}

	// 检查是否为统计信息
	if strings.Contains(line, "有效代理:") || strings.Contains(line, "协议分布:") ||
		strings.Contains(line, "国家分布:") || strings.Contains(line, "延迟统计:") {
		return true
	}

	// 检查是否为菜单选项
	if strings.Contains(line, "开始代理检测") || strings.Contains(line, "更新 GeoIP 数据库") ||
		strings.Contains(line, "退出") || strings.Contains(line, "请输入您的选择") {
		return true
	}

	return false
}

// handleProxyURL 处理标准的代理URL格式
func handleProxyURL(proxiesChan chan *ProxyInfo, parsedURL *url.URL, protocol string) bool {
	// 对HTTPS代理进行正确处理
	if protocol == "https" {
		proxiesChan <- &ProxyInfo{
			URL:      parsedURL.String(),
			Protocol: "https",
		}
		return true
	}

	if strings.HasPrefix(protocol, "socks5") && parsedURL.User != nil {
		protocol = "socks5_auth"
	} else if strings.HasPrefix(protocol, "socks5") && parsedURL.User == nil {
		protocol = "socks5_noauth"
	}

	proxiesChan <- &ProxyInfo{
		URL:      parsedURL.String(),
		Protocol: protocol,
	}
	return true
}

// tryParseWithRegex 尝试用各种正则表达式解析代理格式
func tryParseWithRegex(proxiesChan chan *ProxyInfo, line string,
	reAuthSocks5 *regexp.Regexp, reIPPort *regexp.Regexp, reIPPortAuth *regexp.Regexp,
	reIPPortProtocol *regexp.Regexp, reIPPortAuthProtocol *regexp.Regexp,
	reSpaceSeparated *regexp.Regexp, reDomainPort *regexp.Regexp, reDomainPortAuth *regexp.Regexp,
	reDomainPortProtocol *regexp.Regexp, reSpecialFormat1 *regexp.Regexp, reSpecialFormat2 *regexp.Regexp,
	reSpecialFormat3 *regexp.Regexp, reCommaSeparated *regexp.Regexp, reMixedFormat1 *regexp.Regexp,
	reMixedFormat2 *regexp.Regexp, reSimpleHostPort *regexp.Regexp, reJSONFormat *regexp.Regexp,
	reIPv6Port *regexp.Regexp, reIPv6PortAuth *regexp.Regexp, reGenericFormat *regexp.Regexp) bool {

	// 1. 旧格式：ip:port | user:pass |...
	if matches := reAuthSocks5.FindStringSubmatch(line); len(matches) == 5 {
		ip, port, username, password := matches[1], matches[2], matches[3], matches[4]
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s@%s:%s", url.QueryEscape(username), url.QueryEscape(password), ip, port),
			Protocol: "socks5_auth",
		}
		return true
	}

	// 2. 简单IP:端口格式
	if matches := reIPPort.FindStringSubmatch(line); len(matches) == 3 {
		ip, port := matches[1], matches[2]
		// 默认为socks5无认证
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s", ip, port),
			Protocol: "socks5_noauth",
		}
		return true
	}

	// 3. IP:端口:用户名:密码
	if matches := reIPPortAuth.FindStringSubmatch(line); len(matches) == 5 {
		ip, port, username, password := matches[1], matches[2], matches[3], matches[4]
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s@%s:%s", url.QueryEscape(username), url.QueryEscape(password), ip, port),
			Protocol: "socks5_auth",
		}
		return true
	}

	// 4. IP:端口:协议
	if matches := reIPPortProtocol.FindStringSubmatch(line); len(matches) == 4 {
		ip, port, protocol := matches[1], matches[2], strings.ToLower(matches[3])
		if createProxyFromParts(proxiesChan, ip, port, "", "", protocol) {
			return true
		}
	}

	// 5. IP:端口:用户名:密码:协议
	if matches := reIPPortAuthProtocol.FindStringSubmatch(line); len(matches) == 6 {
		ip, port, username, password, protocol := matches[1], matches[2], matches[3], matches[4], strings.ToLower(matches[5])
		if createProxyFromParts(proxiesChan, ip, port, username, password, protocol) {
			return true
		}
	}

	// 6. 空格分隔格式
	if matches := reSpaceSeparated.FindStringSubmatch(line); len(matches) >= 3 {
		ip, port := matches[1], matches[2]
		var username, password, protocol string
		if len(matches) > 3 && matches[3] != "" {
			// 检查是否为用户名:密码格式
			userParts := strings.SplitN(matches[3], ":", 2)
			if len(userParts) == 2 {
				username, password = userParts[0], userParts[1]
			} else {
				protocol = strings.ToLower(matches[3])
			}
		}
		if len(matches) > 4 && matches[4] != "" {
			password = matches[4]
		}
		if len(matches) > 5 && matches[5] != "" {
			protocol = strings.ToLower(matches[5])
		}
		if createProxyFromParts(proxiesChan, ip, port, username, password, protocol) {
			return true
		}
	}

	// 7. 域名格式
	if matches := reDomainPort.FindStringSubmatch(line); len(matches) == 3 {
		host, port := matches[1], matches[2]
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s", host, port),
			Protocol: "socks5_noauth",
		}
		return true
	}

	// 8. 域名:端口:用户名:密码
	if matches := reDomainPortAuth.FindStringSubmatch(line); len(matches) == 5 {
		host, port, username, password := matches[1], matches[2], matches[3], matches[4]
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s@%s:%s", url.QueryEscape(username), url.QueryEscape(password), host, port),
			Protocol: "socks5_auth",
		}
		return true
	}

	// 9. 域名:端口:协议
	if matches := reDomainPortProtocol.FindStringSubmatch(line); len(matches) == 4 {
		host, port, protocol := matches[1], matches[2], strings.ToLower(matches[3])
		if createProxyFromParts(proxiesChan, host, port, "", "", protocol) {
			return true
		}
	}

	// 10. 特殊格式1: IP - port - user - pass - protocol
	if matches := reSpecialFormat1.FindStringSubmatch(line); len(matches) == 6 {
		ip, port, username, password, protocol := matches[1], matches[2], matches[3], matches[4], strings.ToLower(matches[5])
		if createProxyFromParts(proxiesChan, ip, port, username, password, protocol) {
			return true
		}
	}

	// 11. 特殊格式2: IP @ port
	if matches := reSpecialFormat2.FindStringSubmatch(line); len(matches) == 3 {
		ip, port := matches[1], matches[2]
		proxiesChan <- &ProxyInfo{
			URL:      fmt.Sprintf("socks5://%s:%s", ip, port),
			Protocol: "socks5_noauth",
		}
		return true
	}

	// 12. 特殊格式3: IP ; port ; user ; pass ; protocol
	if matches := reSpecialFormat3.FindStringSubmatch(line); len(matches) >= 3 {
		ip, port := matches[1], matches[2]
		var username, password, protocol string
		if len(matches) > 3 && matches[3] != "" {
			username = matches[3]
		}
		if len(matches) > 4 && matches[4] != "" {
			password = matches[4]
		}
		if len(matches) > 5 && matches[5] != "" {
			protocol = strings.ToLower(matches[5])
		}
		if createProxyFromParts(proxiesChan, ip, port, username, password, protocol) {
			return true
		}
	}

	// 13. 逗号分隔格式
	if matches := reCommaSeparated.FindStringSubmatch(line); len(matches) >= 3 {
		host, port := matches[1], matches[2]
		var username, password, protocol string
		if len(matches) > 3 && matches[3] != "" {
			username = matches[3]
		}
		if len(matches) > 4 && matches[4] != "" {
			password = matches[4]
		}
		if len(matches) > 5 && matches[5] != "" {
			protocol = strings.ToLower(matches[5])
		}
		if createProxyFromParts(proxiesChan, host, port, username, password, protocol) {
			return true
		}
	}

	// 14. 混合格式1: protocol://host:port (从输出日志中看到)
	if matches := reMixedFormat1.FindStringSubmatch(line); len(matches) == 4 {
		protocol, host, port := strings.ToLower(matches[1]), matches[2], matches[3]
		if createProxyFromParts(proxiesChan, host, port, "", "", protocol) {
			return true
		}
	}

	// 15. 混合格式2: socks://user:pass@host:port
	if matches := reMixedFormat2.FindStringSubmatch(line); len(matches) == 6 {
		protocol, username, password, host, port := strings.ToLower(matches[1]), matches[2], matches[3], matches[4], matches[5]
		if createProxyFromParts(proxiesChan, host, port, username, password, protocol) {
			return true
		}
	}

	// 16. 简单主机:端口格式（包括域名、IP、IPv6）
	if matches := reSimpleHostPort.FindStringSubmatch(line); len(matches) == 3 {
		host, port := matches[1], matches[2]
		// 智能推断协议
		if createProxyFromParts(proxiesChan, host, port, "", "", "") {
			return true
		}
	}

	// 17. JSON格式
	if matches := reJSONFormat.FindStringSubmatch(line); len(matches) == 3 {
		host, port := matches[1], matches[2]
		if createProxyFromParts(proxiesChan, host, port, "", "", "") {
			return true
		}
	}

	// 18. IPv6格式
	if matches := reIPv6Port.FindStringSubmatch(line); len(matches) == 3 {
		ipv6, port := matches[1], matches[2]
		if createProxyFromParts(proxiesChan, "["+ipv6+"]", port, "", "", "") {
			return true
		}
	}

	// 19. IPv6:端口:用户名:密码
	if matches := reIPv6PortAuth.FindStringSubmatch(line); len(matches) == 5 {
		ipv6, port, username, password := matches[1], matches[2], matches[3], matches[4]
		if createProxyFromParts(proxiesChan, "["+ipv6+"]", port, username, password, "socks5") {
			return true
		}
	}

	// 20. 通用格式：host:port[:user:pass[:protocol]]
	if matches := reGenericFormat.FindStringSubmatch(line); len(matches) >= 3 {
		host, port := matches[1], matches[2]
		var username, password, protocol string
		if len(matches) > 3 && matches[3] != "" {
			username = matches[3]
		}
		if len(matches) > 4 && matches[4] != "" {
			password = matches[4]
		}
		if len(matches) > 5 && matches[5] != "" {
			protocol = strings.ToLower(matches[5])
		}
		if createProxyFromParts(proxiesChan, host, port, username, password, protocol) {
			return true
		}
	}

	// 21. 最后的兜底策略：尝试解析任何包含冒号和数字的格式
	if strings.Contains(line, ":") {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 {
			// 尝试将最后部分作为端口
			portStr := parts[len(parts)-1]
			if portNum, err := strconv.Atoi(portStr); err == nil && portNum > 0 && portNum <= 65535 {
				// 将前面的部分作为主机
				host := strings.Join(parts[:len(parts)-1], ":")
				host = strings.TrimSpace(host)
				if host != "" {
					if createProxyFromParts(proxiesChan, host, portStr, "", "", "") {
						return true
					}
				}
			}
		}
	}

	return false
}

// createProxyFromParts 根据组件创建代理
func createProxyFromParts(proxiesChan chan *ProxyInfo, host, port, username, password, protocol string) bool {
	// 验证端口
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return false
	}

	// 智能协议推断
	if protocol == "" {
		protocol = inferProtocolByPortAndContext(host, port, username, password)
	}

	// 协议映射和规范化
	protocol = normalizeProtocol(protocol)

	// 构建代理URL
	var proxyURL string
	if username != "" && password != "" {
		proxyURL = fmt.Sprintf("%s://%s:%s@%s:%s", protocol, url.QueryEscape(username), url.QueryEscape(password), host, port)
	} else {
		proxyURL = fmt.Sprintf("%s://%s:%s", protocol, host, port)
	}

	// 确定协议标识
	protocolID := determineProtocolID(protocol, username, password)

	proxiesChan <- &ProxyInfo{
		URL:      proxyURL,
		Protocol: protocolID,
	}
	return true
}

// inferProtocolByPortAndContext 根据端口和上下文智能推断协议
func inferProtocolByPortAndContext(host, port, username, password string) string {
	portNum, _ := strconv.Atoi(port)

	// 根据端口推断常见协议
	switch portNum {
	case 80, 8080, 3128, 8000, 8888:
		// HTTP常见端口
		if username != "" && password != "" {
			return "http" // HTTP代理通常需要认证
		}
		return "http"
	case 443, 8443:
		// HTTPS常见端口
		return "https"
	case 1080, 1081, 1082:
		// SOCKS常见端口
		return "socks5"
	case 5555:
		// 特殊SOCKS5端口（从您的示例中看到）
		return "socks5"
	case 20000:
		// 特殊管理端口
		return "socks5"
	case 343:
		// 特殊端口
		return "http"
	default:
		// 默认根据是否有认证信息推断
		if username != "" && password != "" {
			return "socks5" // 有认证信息更可能是SOCKS5
		}
		return "socks5" // 默认SOCKS5
	}
}

// normalizeProtocol 规范化协议名称
func normalizeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))

	switch protocol {
	case "socks5", "socks", "socks5h", "sock5", "socks5proxy":
		return "socks5"
	case "socks4", "socks4a", "sock4":
		return "socks4"
	case "http", "http-proxy", "proxy":
		return "http"
	case "https", "https-proxy", "ssl", "tls":
		return "https"
	case "tcp", "connect":
		return "http" // TCP CONNECT通常指HTTP代理
	default:
		// 对于未知协议，尝试启发式判断
		if strings.Contains(protocol, "socks") {
			return "socks5"
		}
		if strings.Contains(protocol, "http") {
			return "http"
		}
		if strings.Contains(protocol, "ssl") || strings.Contains(protocol, "tls") {
			return "https"
		}
		return "socks5" // 默认SOCKS5
	}
}

// determineProtocolID 确定最终的协议标识
func determineProtocolID(protocol string, username, password string) string {
	switch protocol {
	case "socks5":
		if username != "" && password != "" {
			return "socks5_auth"
		}
		return "socks5_noauth"
	case "socks4":
		if username != "" {
			return "socks4_auth"
		}
		return "socks4_noauth"
	case "http":
		return "http"
	case "https":
		return "https"
	default:
		return protocol
	}
}

// tryParseOldFormat 尝试解析旧的分隔格式（兼容性）
func tryParseOldFormat(proxiesChan chan *ProxyInfo, line string) bool {
	parts := strings.SplitN(line, "|", 2)
	proxyStr := strings.TrimSpace(parts[0])

	proxyParts := strings.Split(proxyStr, ":")
	if len(proxyParts) >= 3 {
		protocol := strings.ToLower(proxyParts[len(proxyParts)-1])

		var ip, port, username, password string
		if len(proxyParts) >= 5 {
			// 格式：ip:port:username:password:protocol
			ip = strings.Join(proxyParts[:len(proxyParts)-4], ":")
			port = proxyParts[len(proxyParts)-4]
			username = proxyParts[len(proxyParts)-3]
			password = proxyParts[len(proxyParts)-2]
		} else {
			// 格式：ip:port:protocol
			ip = strings.Join(proxyParts[:len(proxyParts)-2], ":")
			port = proxyParts[len(proxyParts)-2]
		}

		return createProxyFromParts(proxiesChan, ip, port, username, password, protocol)
	}

	return false
}

// NetworkClient 增强的网络客户端结构体
type NetworkClient struct {
	client    *http.Client
	timeout   time.Duration
	retries   int
	retryDelay time.Duration
}

// NewNetworkClient 创建新的网络客户端
func NewNetworkClient(timeout time.Duration, retries int) *NetworkClient {
	return &NetworkClient{
		timeout:    timeout,
		retries:    retries,
		retryDelay: 500 * time.Millisecond, // 进一步减少重试间隔
	}
}

// DoWithRetry 带重试机制的HTTP请求
func (nc *NetworkClient) DoWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < nc.retries; attempt++ {
		if attempt > 0 {
			delay := nc.retryDelay * time.Duration(attempt)
			if delay > 2*time.Second {
				delay = 2 * time.Second // 最多等待2秒
			}
			log.Printf("🔄 网络请求重试 %d/%d (等待%.1fs)", attempt, nc.retries, delay.Seconds())
			time.Sleep(delay)
		}

		if nc.client == nil {
			nc.client = &http.Client{
				Timeout: nc.timeout,
			}
		}

		startTime := time.Now()
		resp, err := nc.client.Do(req)
		requestDuration := time.Since(startTime)

		if err == nil {
			log.Printf("✅ HTTP请求成功，耗时: %v，状态码: %d", requestDuration, resp.StatusCode)
			return resp, nil
		}

		lastErr = err
		log.Printf("❌ 网络请求失败 (尝试 %d/%d)，耗时 %v: %v", attempt+1, nc.retries, requestDuration, err)

		// 如果是网络相关错误，重新创建客户端
		if isNetworkError(err) {
			nc.client = nil
		}
	}

	log.Printf("❌ 所有重试尝试均失败，返回错误: %v", lastErr)
	return nil, lastErr
}

// isNetworkError 判断是否为网络错误
func isNetworkError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection aborted")
}

// createTelegramClientWithProxy 创建一个带代理的 HTTP 客户端用于 Telegram 通信（使用 aigo.go 的方式）
func createTelegramClientWithProxy(proxyURL string) (*http.Client, error) {
	// 检查环境变量中的配置
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	// 如果环境变量存在，使用环境变量
	if botToken != "" {
		config.Telegram.BotToken = botToken
	}
	if chatID != "" {
		config.Telegram.ChatID = chatID
	}

	if config.Telegram.BotToken == "" {
		return nil, fmt.Errorf("Telegram Bot Token 未配置")
	}

	var transport *http.Transport
	var err error

	if proxyURL == "" {
		log.Printf("🔍 使用直连方式创建Telegram客户端")
		transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second, // 使用合理的连接超时
			}).DialContext,
		}
	} else {
		log.Printf("🔍 使用代理创建Telegram客户端: %s", proxyURL)
		transport, err = createTransportWithProxy(proxyURL) // 使用标准的传输创建
		if err != nil {
			log.Printf("❌ 创建代理传输失败: %v", err)
			return nil, fmt.Errorf("创建代理传输失败: %v", err)
		}
	}

	// 创建客户端，使用合理的超时
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // 使用合理的超时
	}

	// 验证连接
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", config.Telegram.BotToken)
	log.Printf("🔍 开始验证Telegram API连接")

	resp, err := client.Get(apiURL)
	if err != nil {
		log.Printf("❌ Telegram API验证失败: %v", err)
		return nil, fmt.Errorf("代理验证失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Telegram API返回错误，状态码: %d, 响应: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("代理验证失败，HTTP 状态码: %d", resp.StatusCode)
	}

	log.Printf("✅ Telegram API验证成功")

	return client, nil
}

// getTelegramClient 获取一个可用的 Telegram 客户端，并进行缓存
func getTelegramClient() *http.Client {
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()

	// 如果缓存中已有有效的客户端，直接返回
	if telegramClientCache != nil {
		return telegramClientCache
	}

	// 清理过期的失效代理缓存（清理超过1小时的记录）
	cleanExpiredFailedProxies()

	var client *http.Client
	var err error

	// 遍历预设代理列表，跳过已知失效的代理
	for _, proxyURL := range config.Settings.PresetProxy {
		// 检查是否在失效代理缓存中
		failedProxiesMutex.RLock()
		if failTime, exists := failedProxiesCache[proxyURL]; exists {
			// 如果在30分钟内失败过，跳过这个代理
			if time.Since(failTime) < 30*time.Minute {
				failedProxiesMutex.RUnlock()
				log.Printf("⏭️ 跳过最近失效的代理 %s (剩余冷却时间: %.1f分钟)\n",
					proxyURL, (30*time.Minute-time.Since(failTime)).Minutes())
				continue
			}
		}
		failedProxiesMutex.RUnlock()

		log.Printf("⏳ 尝试通过预设代理 %s 连接 Telegram API...\n", proxyURL)
		client, err = createTelegramClientWithProxy(proxyURL)
		if err == nil {
			log.Printf("🟢 成功通过代理 %s 建立 Telegram 会话。\n", proxyURL)
			telegramClientCache = client // 缓存成功的客户端

			// 从失效代理缓存中移除（如果之前存在）
			failedProxiesMutex.Lock()
			delete(failedProxiesCache, proxyURL)
			failedProxiesMutex.Unlock()

			return client
		}

		log.Printf("❌ 预设代理 %s 连接 Telegram 失败: %v\n", proxyURL, err)

		// 将失效代理添加到缓存
		failedProxiesMutex.Lock()
		failedProxiesCache[proxyURL] = time.Now()
		failedProxiesMutex.Unlock()
	}

	log.Println("⏳ 所有预设代理均失败或已被跳过，尝试直连...")
	client, err = createTelegramClientWithProxy("")
	if err == nil {
		log.Println("✅ 直连 Telegram API 成功。")
		telegramClientCache = client // 缓存直连客户端
		return client
	}
	log.Println("❌ 直连 Telegram API 失败，所有连接方式均失败。")
	return nil
}

// sendSecureTelegramMessage 安全发送Telegram消息（使用 aigo.go 的方式）
func sendSecureTelegramMessage(message string) bool {
	if config.Telegram.BotToken == "" || config.Telegram.ChatID == "" {
		log.Println("❌ 未配置 TELEGRAM_BOT_TOKEN 或 TELEGRAM_CHAT_ID，跳过 Telegram 通知")
		return false
	}

	client := getTelegramClient()
	if client == nil {
		log.Println("❌ 无法建立网络连接，跳过 Telegram 消息发送。")
		return false
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.Telegram.BotToken)
	payload := map[string]string{
		"chat_id":    config.Telegram.ChatID,
		"text":       message,
		"parse_mode": "MarkdownV2",
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("❌ Telegram 消息发送失败: %v\n", err)
		// 如果发送失败，清除缓存客户端，以便下次重新验证
		clientCacheMutex.Lock()
		telegramClientCache = nil
		clientCacheMutex.Unlock()
		log.Println("⚠️ Telegram 客户端已失效，已清除缓存，下次将重新验证。")
		return false
	}
	defer resp.Body.Close()

	var apiResp telegramAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil || !apiResp.Ok {
		log.Printf("❌ Telegram API 错误: %s\n", apiResp.Description)
		// 如果API返回错误，清除缓存客户端
		clientCacheMutex.Lock()
		telegramClientCache = nil
		clientCacheMutex.Unlock()
		return false
	}

	log.Println("✅ Telegram 消息发送成功！")
	return true
}

// getProxyDescription 获取代理描述
func getProxyDescription(proxyURL string) string {
	if proxyURL == "" {
		return "直连"
	}
	return proxyURL
}

// cleanExpiredFailedProxies 清理过期的失效代理缓存
func cleanExpiredFailedProxies() {
	failedProxiesMutex.Lock()
	defer failedProxiesMutex.Unlock()

	now := time.Now()
	for proxyURL, failTime := range failedProxiesCache {
		// 清理超过1小时的记录
		if now.Sub(failTime) > time.Hour {
			delete(failedProxiesCache, proxyURL)
		}
	}

	// 可选：如果缓存过大，清理最旧的一些记录
	maxCacheSize := 100
	if len(failedProxiesCache) > maxCacheSize {
		// 按时间排序，保留最新的记录
		type proxyFail struct {
			url  string
			time time.Time
		}
		var fails []proxyFail
		for url, t := range failedProxiesCache {
			fails = append(fails, proxyFail{url: url, time: t})
		}

		// 按时间降序排序（最新的在前）
		sort.Slice(fails, func(i, j int) bool {
			return fails[i].time.After(fails[j].time)
		})

		// 清空缓存并重新添加最新的记录
		failedProxiesCache = make(map[string]time.Time)
		for i := 0; i < maxCacheSize && i < len(fails); i++ {
			failedProxiesCache[fails[i].url] = fails[i].time
		}

		log.Printf("🧹 失效代理缓存过大，已清理保留最新的 %d 条记录\n", maxCacheSize)
	}
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// createUltraOptimizedTransportWithProxy 创建超优化的代理传输（彻底解决超时问题）
func createUltraOptimizedTransportWithProxy(proxyURL string) (*http.Transport, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	// 极短的拨号器超时，彻底解决卡死问题
	dialer := &net.Dialer{
		Timeout: 800 * time.Millisecond, // 0.8秒超时
	}

	switch parsedURL.Scheme {
	case "http":
		// HTTP代理处理
		proxyFunc := http.ProxyURL(parsedURL)
		return &http.Transport{
			Proxy:       proxyFunc,
			DialContext: dialer.DialContext,
		}, nil
	case "https":
		// HTTPS代理 - 使用CONNECT隧道方式
		proxyFunc := http.ProxyURL(parsedURL)
		return &http.Transport{
			Proxy:             proxyFunc,
			DialContext:       dialer.DialContext,
			ForceAttemptHTTP2: false,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		}, nil
	case "socks5", "socks5h":
		// SOCKS5代理 - 超激进优化配置
		var auth *proxy.Auth
		if parsedURL.User != nil {
			password, _ := parsedURL.User.Password()
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: password,
			}
		}

		socks5Dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, dialer)
		if err != nil {
			return nil, err
		}

		return &http.Transport{
			DialContext: socks5Dialer.(proxy.ContextDialer).DialContext,
			// 最激进的TLS配置
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
				MaxVersion:         tls.VersionTLS12, // 强制TLS 1.2
				ServerName:         "", // 跳过SNI检查
			},
			// 彻底禁用HTTP/2
			ForceAttemptHTTP2: false,
			// 添加更多优化参数
			DisableKeepAlives:    false, // 保持连接
			DisableCompression:    false, // 允许压缩
		}, nil
	case "socks4":
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{User: parsedURL.User.Username()}
		}

		socks4Dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, dialer)
		if err != nil {
			return nil, err
		}

		return &http.Transport{
			DialContext: socks4Dialer.(proxy.ContextDialer).DialContext,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的协议: %s", parsedURL.Scheme)
	}
}

// loadSecureConfig 安全加载配置（支持环境变量）
func loadSecureConfig(configPath string) error {
	// 首先加载配置文件
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("❌ 无法加载配置文件: %w", err)
	}

	err = cfg.MapTo(&config)
	if err != nil {
		return fmt.Errorf("❌ 无法映射配置到结构体: %w", err)
	}

	// 环境变量覆盖配置文件
	if botToken := os.Getenv("TELEGRAM_BOT_TOKEN"); botToken != "" {
		config.Telegram.BotToken = botToken
		log.Println("🔐 使用环境变量中的 Telegram Bot Token")
	}

	if chatID := os.Getenv("TELEGRAM_CHAT_ID"); chatID != "" {
		config.Telegram.ChatID = chatID
		log.Println("🔐 使用环境变量中的 Telegram Chat ID")
	}

	if presetProxies := os.Getenv("PRESET_PROXIES"); presetProxies != "" {
		config.Settings.PresetProxy = strings.Split(presetProxies, ",")
		log.Println("🔐 使用环境变量中的预设代理")
	}

	proxyStr := cfg.Section("settings").Key("preset_proxy").String()
	if proxyStr != "" && len(config.Settings.PresetProxy) == 0 {
		config.Settings.PresetProxy = strings.Split(proxyStr, ",")
	}

	return nil
}

// 主函数
func main() {
	// 设置日志格式
	log.SetFlags(0)
	var err error
	logFile, err = os.OpenFile("check_log.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("❌ 无法打开日志文件: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(&LogWriter{})

	// 安全加载配置
	if err := loadSecureConfig("config.ini"); err != nil {
		log.Fatalf("❌ 配置加载失败: %v", err)
	}

	// 设置默认值
	if config.Settings.CheckTimeout <= 0 {
		config.Settings.CheckTimeout = 15
		log.Printf("⚠️ 未设置检测超时，使用默认值: %d 秒\n", config.Settings.CheckTimeout)
	}
	if config.Settings.MaxConcurrent <= 0 {
		config.Settings.MaxConcurrent = 50
		log.Printf("⚠️ 未设置最大并发数，使用默认值: %d\n", config.Settings.MaxConcurrent)
	}
	if config.Settings.FdipDir == "" {
		config.Settings.FdipDir = "FDIP"
		log.Printf("⚠️ 未设置代理目录，使用默认值: %s\n", config.Settings.FdipDir)
	}
	if config.Settings.OutputDir == "" {
		config.Settings.OutputDir = "OUTPUT"
		log.Printf("⚠️ 未设置输出目录，使用默认值: %s\n", config.Settings.OutputDir)
	}

	// 获取终端宽度
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}

	DrawCenteredTitleBox(ColorYellow+"   代 理 检 测 工 具 v2.0 (增强版)   "+ColorReset, width)

	log.Println(ColorGreen + "✅ 配置加载成功！" + ColorReset)
	if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
		log.Println(ColorCyan + "- Telegram 机器人已就绪。" + ColorReset)
	} else {
		log.Println(ColorYellow + "- Telegram 配置不完整，将跳过通知。" + ColorReset)
	}

	if len(config.Settings.PresetProxy) > 0 {
		log.Printf(ColorCyan+"- 已加载 %d 个预设代理。\n", len(config.Settings.PresetProxy))
	} else {
		log.Println(ColorYellow + "- 没有预设代理，将使用直连方式。" + ColorReset)
	}

	log.Printf(ColorCyan+"- 检测超时设置为 %d 秒，最大并发数 %d。\n", config.Settings.CheckTimeout, config.Settings.MaxConcurrent)
	log.Println(ColorCyan + "- 已启用网络重试机制和错误处理优化。" + ColorReset)
	log.Println(ColorCyan + "------------------------------------------" + ColorReset)

	showMenu()
}

// showMenu 显示主菜单并处理用户输入
func showMenu() {
	for {
		fmt.Println(ColorYellow + "\n--- 请选择一个操作 ---" + ColorReset)
		fmt.Println("1. 🚀 " + ColorGreen + "开始代理检测" + ColorReset)
		fmt.Println("2. 🌐 " + ColorBlue + "更新 GeoIP 数据库" + ColorReset)
		fmt.Println("3. ❌ " + ColorRed + "退出" + ColorReset)
		fmt.Print("请输入您的选择 (1/2/3): ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			runEnhancedCheck()
		case "2":
			log.Println("----------- GeoIP 数据库更新 -----------")
			if _, err := os.Stat(GEOIP_DB_PATH); err == nil && isGeoIPFileValid(GEOIP_DB_PATH) {
				log.Printf("✅ 本地 GeoIP 数据库已存在且有效: %s\n", GEOIP_DB_PATH)
				fileInfo, _ := os.Stat(GEOIP_DB_PATH)
				mtime := fileInfo.ModTime()
				ageDays := time.Since(mtime).Hours() / 24
				if ageDays < 7 {
					log.Printf("ℹ️ 数据库较新 (%.1f 天)，无需更新。\n", ageDays)
					log.Println("------------------------------------------")
				} else {
					log.Printf("⚠️ 数据库较旧 (%.1f 天)，将强制更新。\n", ageDays)
					log.Println("------------------------------------------")
					downloadGeoIPDatabase(GEOIP_DB_PATH)
				}
			} else {
				if err == nil {
					log.Printf("⚠️ 本地 GeoIP 数据库无效，将重新下载。\n")
					os.Remove(GEOIP_DB_PATH)
				} else {
					log.Printf("ℹ️ 本地 GeoIP 数据库不存在，将下载最新文件。\n")
				}
				log.Println("------------------------------------------")
				downloadGeoIPDatabase(GEOIP_DB_PATH)
			}
		case "3":
			fmt.Println("👋 退出程序。")
			return
		default:
			fmt.Println(ColorRed + "⚠️ 无效的选择，请重新输入。" + ColorReset)
		}
	}
}

// runEnhancedCheck 增强版代理检测核心逻辑
func runEnhancedCheck() {
	log.Println(ColorGreen + "**🚀 代理检测工具启动 (增强版)**" + ColorReset)
	log.Println(ColorCyan + "------------------------------------------" + ColorReset)

	start := time.Now()

	// 发送启动通知
	if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
		message := "*🚀 代理检测工具启动 \\(增强版\\)*"
		if sendSecureTelegramMessage(message) {
			log.Println("✅ 启动通知发送成功")
		} else {
			log.Println("❌ 启动通知发送失败，但程序将继续运行")
		}
	} else {
		log.Println(ColorYellow + "❌ 未配置 Telegram Bot Token 或 Chat ID，跳过 Telegram 通知。" + ColorReset)
	}

	// 初始化GeoIP数据库
	initGeoIPReader()
	defer closeGeoIPReader()

	// 检查代理目录
	fdipPath := filepath.Join(".", config.Settings.FdipDir)
	if _, err := os.Stat(fdipPath); os.IsNotExist(err) {
		log.Printf(ColorRed+"❌ 目录不存在: %s\n"+ColorReset, fdipPath)
		sendSecureTelegramMessage(escapeMarkdownV2("❌ 错误: 目录 `"+config.Settings.FdipDir+"` 不存在"))
		return
	}

	// 提取代理
	log.Println(ColorCyan + "📂 正在读取代理文件..." + ColorReset)
	proxiesChan := extractProxiesFromFile(fdipPath, config.Settings.MaxConcurrent)

	// 收集所有代理
	var allProxies []*ProxyInfo
	for p := range proxiesChan {
		allProxies = append(allProxies, p)
	}

	// 去重处理
	uniqueProxies := removeDuplicateProxies(allProxies)
	log.Printf("📊 原始代理数量: %d, 去重后: %d (去除了 %d 个重复代理)\n",
		len(allProxies), len(uniqueProxies), len(allProxies)-len(uniqueProxies))

	if len(uniqueProxies) == 0 {
		log.Println(ColorYellow + "⚠️ 未提取到任何代理，退出" + ColorReset)
		sendSecureTelegramMessage(escapeMarkdownV2("⚠️ *代理检测完成*\n没有提取到任何代理"))
		return
	}

	log.Println(ColorCyan + "⏳ 正在异步检测代理有效性，请稍候..." + ColorReset)

	// 分发代理到测试通道
	testProxiesChan := make(chan *ProxyInfo, config.Settings.MaxConcurrent)
	go func() {
		defer close(testProxiesChan)
		for _, p := range uniqueProxies {
			testProxiesChan <- p
		}
	}()

	// 运行测试
	resultsChan := runProxyTests(testProxiesChan)

	// 处理结果
	var validProxies []ProxyResult
	failedProxiesStats := make(map[string]int)
	ipsToQuery := make(map[string]struct{})

	// 实时处理结果
	for result := range resultsChan {
		if result.Success {
			// 获取IP类型图标和描述
			ipTypeIcon := IP_TYPE_MAP[result.IPType]
			if ipTypeIcon == "" {
				ipTypeIcon = IP_TYPE_MAP["unknown"]
			}
			ipTypeDesc := IP_TYPE_DESCRIPTION[result.IPType]
			if ipTypeDesc == "" {
				ipTypeDesc = IP_TYPE_DESCRIPTION["unknown"]
			}

			// 打印可用代理的实时信息
			log.Printf(ColorGreen+"| 延迟: %.2fms | IP: %-15s | %s %s"+ColorReset+" ✅ 可用: %s\n",
				result.Latency, result.IP, ipTypeIcon, ipTypeDesc, result.URL)

			validProxies = append(validProxies, result)
			if result.IP != "" {
				ipsToQuery[result.IP] = struct{}{}
			}
		} else {
			// 打印失败代理的实时信息
			reason := result.Reason
			normalizedReason := "其他错误"
			for key, val := range FAILURE_REASON_MAP {
				if strings.Contains(reason, key) {
					normalizedReason = val
					break
				}
			}
			reHTTPStatus := regexp.MustCompile(`HTTP Status: (\d+)`)
			if matches := reHTTPStatus.FindStringSubmatch(reason); len(matches) == 2 {
				statusCode, _ := strconv.Atoi(matches[1])
				if statusCode >= 400 && statusCode < 500 {
					normalizedReason = fmt.Sprintf("客户端错误 (%d)", statusCode)
				} else if statusCode >= 500 && statusCode < 600 {
					normalizedReason = fmt.Sprintf("服务器错误 (%d)", statusCode)
				} else {
					normalizedReason = fmt.Sprintf("HTTP 状态 (%d)", statusCode)
				}
			}
			log.Printf(ColorRed+"❌ 失败: %s | 原因: %s\n"+ColorReset, result.URL, normalizedReason)
			failedProxiesStats[normalizedReason]++
		}
	}

	log.Println(ColorCyan + "\n🎉 代理检测完成，正在生成报告..." + ColorReset)

	if len(validProxies) == 0 {
		log.Println(ColorYellow + "⚠️ 没有检测到可用代理" + ColorReset)
		sendSecureTelegramMessage(escapeMarkdownV2("⚠️ *代理检测完成*\n没有检测到任何可用代理"))
		return
	}

	// 批量查询IP地理位置
	ips := make([]string, 0, len(ipsToQuery))
	for ip := range ipsToQuery {
		ips = append(ips, ip)
	}
	log.Printf("🔍 开始查询 %d 个IP的地理位置信息\n", len(ips))
	countryCodesMap := getCountryFromIPBatch(ips)
	log.Printf("🌍 地理位置查询完成，获得 %d 个国家代码\n", len(countryCodesMap))

	// 更新代理的国家信息（保持IP地址不变，添加国家代码到新的字段）
	for i := range validProxies {
		if countryCode, ok := countryCodesMap[validProxies[i].IP]; ok {
			// 保持IP地址不变，将国家代码存储在IPDetails字段中
			if validProxies[i].IPDetails == "" {
				validProxies[i].IPDetails = countryCode
			}
		} else {
			// 如果没有找到国家代码，设置为UNKNOWN
			if validProxies[i].IPDetails == "" {
				validProxies[i].IPDetails = "UNKNOWN"
			}
		}
	}

	// 写入结果文件
	log.Println(ColorCyan + "\n💾 正在写入结果文件..." + ColorReset)
	writeValidProxies(validProxies)

	// 生成统计报告
	generateEnhancedReport(validProxies, failedProxiesStats, start)

	// 自动更新Telegram预设代理列表
	if config.AutoProxyUpdate.Enabled && len(validProxies) > 0 {
		log.Println(ColorCyan + "\n🔄 正在自动更新Telegram预设代理列表..." + ColorReset)

		// 记录更新前的状态
		originalProxyCount := len(config.Settings.PresetProxy)
		log.Printf("📋 更新前预设代理数量: %d\n", originalProxyCount)

		// 选择最优代理
		bestProxies := selectBestProxies(
			validProxies,
			config.AutoProxyUpdate.MaxProxies,
			config.AutoProxyUpdate.PreferResidential,
			config.AutoProxyUpdate.MaxLatency,
		)

		if len(bestProxies) > 0 {
			log.Printf("🎯 选出 %d 个最优代理用于更新\n", len(bestProxies))

			// 更新配置文件
			updateStart := time.Now()
			if err := updateConfigPresetProxies(bestProxies); err != nil {
				log.Printf(ColorRed+"❌ 自动更新预设代理失败: %v\n"+ColorReset, err)

				// 发送失败通知
				if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
					failureMsg := fmt.Sprintf("❌ *代理自动更新失败*\n错误: `%s`\n耗时: `%.2f`秒",
						escapeMarkdownV2(err.Error()), time.Since(updateStart).Seconds())
					sendSecureTelegramMessage(failureMsg)
				}
			} else {
				updateDuration := time.Since(updateStart)
				log.Printf(ColorGreen+"✅ Telegram预设代理列表自动更新完成！耗时: %.2f秒\n"+ColorReset, updateDuration.Seconds())

				// 发送成功通知
				if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
					successMsg := fmt.Sprintf("✅ *代理自动更新成功*\n更新代理数: `%d`\n耗时: `%.2f`秒",
						len(bestProxies), updateDuration.Seconds())
					sendSecureTelegramMessage(successMsg)
				}
			}
		} else {
			log.Println(ColorYellow + "⚠️ 没有找到符合条件的代理来更新预设列表" + ColorReset)

			// 发送警告通知
			if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
				warningMsg := "⚠️ *代理自动更新警告*\n没有找到符合条件的代理\n可能原因:\n• 延迟超限\n• 协议不支持\n• 代理测试失败"
				sendSecureTelegramMessage(warningMsg)
			}
		}
	} else {
		if !config.AutoProxyUpdate.Enabled {
			log.Println(ColorCyan + "ℹ️ 自动更新功能已禁用，跳过预设代理更新" + ColorReset)
		} else {
			log.Println(ColorYellow + "⚠️ 没有有效代理，跳过预设代理更新" + ColorReset)
		}
	}

	// 发送文件到Telegram
	log.Println(ColorCyan + "\n📤 正在推送所有输出文件..." + ColorReset)
	log.Printf("📁 输出目录: %s\n", config.Settings.OutputDir)

	sentCount := 0
	skipCount := 0
	for _, filePath := range OUTPUT_FILES {
		fullPath := filepath.Join(config.Settings.OutputDir, filePath)
		log.Printf("🔍 检查文件: %s\n", fullPath)

		if sendTelegramFile(fullPath) {
			sentCount++
		} else {
			skipCount++
		}
	}

	log.Printf("📊 文件推送完成: 成功 %d 个，跳过 %d 个\n", sentCount, skipCount)

	// 发送结束通知
	if config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
		sendSecureTelegramMessage("*🎉 程序运行结束*")
	}

	log.Println(ColorGreen + "\033[1m🎉 程序运行结束！\033[0m" + ColorReset)
}

// generateEnhancedReport 生成增强版检测报告
func generateEnhancedReport(validProxies []ProxyResult, failedProxiesStats map[string]int, start time.Time) {
	totalValidCount := len(validProxies)
	protocolDistribution := make(map[string]int)
	countryDistribution := make(map[string]int)
	ipTypeDistribution := make(map[string]int)
	var latencies []float64

	for _, p := range validProxies {
		protoKey := p.Protocol
		if strings.HasPrefix(protoKey, "socks5") {
			protoKey += "_tg"
		}
		protocolDistribution[protoKey]++
		countryDistribution[p.IPDetails]++
		ipTypeDistribution[p.IPType]++
		latencies = append(latencies, p.Latency)
	}

	// 计算延迟统计
	minLatency, maxLatency, avgLatency := 0.0, 0.0, 0.0
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		minLatency = latencies[0]
		maxLatency = latencies[len(latencies)-1]
		var sum float64
		for _, l := range latencies {
			sum += l
		}
		avgLatency = sum / float64(len(latencies))
	}

	// 打印报告
	log.Println(ColorGreen + "\n🎉 代理检测报告 (增强版)" + ColorReset)
	log.Printf("⏰ 耗时: %.2f 秒\n", time.Since(start).Seconds())
	log.Printf("✅ 有效代理: %d 个\n", totalValidCount)

	// 协议分布
	if len(protocolDistribution) > 0 {
		log.Println(ColorBlue + "\n🌐 协议分布:" + ColorReset)
		var sortedProtocols []string
		for proto := range protocolDistribution {
			sortedProtocols = append(sortedProtocols, proto)
		}
		sort.Strings(sortedProtocols)
		for _, proto := range sortedProtocols {
			log.Printf("  - %s: %d 个\n", proto, protocolDistribution[proto])
		}
	}

	// 国家分布
	if len(countryDistribution) > 0 {
		log.Println(ColorBlue + "\n🌍 国家分布:" + ColorReset)
		var sortedCountries []string
		for country := range countryDistribution {
			sortedCountries = append(sortedCountries, country)
		}
		sort.Strings(sortedCountries)
		for _, countryCode := range sortedCountries {
			flag := COUNTRY_FLAG_MAP[countryCode]
			countryName := COUNTRY_CODE_TO_NAME[countryCode]
			if countryCode == "" || countryCode == "UNKNOWN" {
				log.Printf("  - 调试: 发现空国家代码，数量: %d\n", countryDistribution[countryCode])
			}
			log.Printf("  - %s %s (%s): %d 个\n", flag, countryName, countryCode, countryDistribution[countryCode])
		}
	}

	// IP类型分布
	if len(ipTypeDistribution) > 0 {
		log.Println(ColorBlue + "\n🏷️ IP类型分布:" + ColorReset)
		var sortedTypes []string
		for ipType := range ipTypeDistribution {
			sortedTypes = append(sortedTypes, ipType)
		}
		sort.Strings(sortedTypes)
		for _, ipType := range sortedTypes {
			icon := IP_TYPE_MAP[ipType]
			if icon == "" {
				icon = IP_TYPE_MAP["unknown"]
			}
			desc := IP_TYPE_DESCRIPTION[ipType]
			if desc == "" {
				desc = IP_TYPE_DESCRIPTION["unknown"]
			}
			log.Printf("  - %s %s: %d 个\n", icon, desc, ipTypeDistribution[ipType])
		}
	}

	// 延迟统计
	if len(latencies) > 0 {
		log.Println(ColorBlue + "\n📈 延迟统计:" + ColorReset)
		log.Printf("  - 均值: %.2fms\n", avgLatency)
		log.Printf("  - 最低: %.2fms\n", minLatency)
		log.Printf("  - 最高: %.2fms\n", maxLatency)
	}

	// 失败原因统计
	if len(failedProxiesStats) > 0 {
		log.Println(ColorRed + "\n⚠️ 检测失败原因:" + ColorReset)
		var reasons []string
		for reason := range failedProxiesStats {
			reasons = append(reasons, reason)
		}
		sort.Slice(reasons, func(i, j int) bool {
			return failedProxiesStats[reasons[i]] > failedProxiesStats[reasons[j]]
		})
		for _, reason := range reasons {
			log.Printf("  - %s: %d 个\n", reason, failedProxiesStats[reason])
		}
	}

	// 发送Telegram报告
	sendTelegramReport(validProxies, failedProxiesStats, start, protocolDistribution, countryDistribution, ipTypeDistribution, latencies)
}

// sendTelegramReport 发送Telegram报告
func sendTelegramReport(validProxies []ProxyResult, failedProxiesStats map[string]int, start time.Time,
	protocolDistribution map[string]int, countryDistribution map[string]int,
	ipTypeDistribution map[string]int, latencies []float64) {

	totalValidCount := len(validProxies)
	var messageParts []string

	messageParts = append(messageParts, "*🎉 代理检测报告 (增强版)*")
	messageParts = append(messageParts, fmt.Sprintf("⏰ 耗时: `%.2f` 秒", time.Since(start).Seconds()))
	messageParts = append(messageParts, fmt.Sprintf("✅ 有效代理: `%d` 个", totalValidCount))

	// 协议分布
	if len(protocolDistribution) > 0 {
		messageParts = append(messageParts, "\n*🌐 协议分布*:")
		var sortedProtocols []string
		for proto := range protocolDistribution {
			sortedProtocols = append(sortedProtocols, proto)
		}
		sort.Strings(sortedProtocols)
		for _, proto := range sortedProtocols {
			messageParts = append(messageParts, fmt.Sprintf("  - `%s`: `%d` 个", proto, protocolDistribution[proto]))
		}
	}

	// 国家分布
	if len(countryDistribution) > 0 {
		messageParts = append(messageParts, "\n*🌍 国家分布*:")
		var sortedCountries []string
		for country := range countryDistribution {
			sortedCountries = append(sortedCountries, country)
		}
		sort.Strings(sortedCountries)
		for _, countryCode := range sortedCountries {
			flag := COUNTRY_FLAG_MAP[countryCode]
			countryName := COUNTRY_CODE_TO_NAME[countryCode]
			messageParts = append(messageParts, fmt.Sprintf("  - %s %s: `%d` 个", flag, countryName, countryDistribution[countryCode]))
		}
	}

	// IP类型分布
	if len(ipTypeDistribution) > 0 {
		messageParts = append(messageParts, "\n*🏷️ IP类型分布*:")
		var sortedTypes []string
		for ipType := range ipTypeDistribution {
			sortedTypes = append(sortedTypes, ipType)
		}
		sort.Strings(sortedTypes)
		for _, ipType := range sortedTypes {
			icon := IP_TYPE_MAP[ipType]
			if icon == "" {
				icon = IP_TYPE_MAP["unknown"]
			}
			desc := IP_TYPE_DESCRIPTION[ipType]
			if desc == "" {
				desc = IP_TYPE_DESCRIPTION["unknown"]
			}
			messageParts = append(messageParts, fmt.Sprintf("  - %s %s: `%d` 个", icon, desc, ipTypeDistribution[ipType]))
		}
	}

	// 延迟统计
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		minLatency := latencies[0]
		maxLatency := latencies[len(latencies)-1]
		var sum float64
		for _, l := range latencies {
			sum += l
		}
		avgLatency := sum / float64(len(latencies))

		messageParts = append(messageParts, "\n*📈 延迟统计*:")
		messageParts = append(messageParts, fmt.Sprintf("  - 均值: `%.2f`ms", avgLatency))
		messageParts = append(messageParts, fmt.Sprintf("  - 最低: `%.2f`ms", minLatency))
		messageParts = append(messageParts, fmt.Sprintf("  - 最高: `%.2f`ms", maxLatency))
	}

	// 失败原因
	if len(failedProxiesStats) > 0 {
		messageParts = append(messageParts, "\n*⚠️ 检测失败原因*:")
		var reasons []string
		for reason := range failedProxiesStats {
			reasons = append(reasons, reason)
		}
		sort.Slice(reasons, func(i, j int) bool {
			return failedProxiesStats[reasons[i]] > failedProxiesStats[reasons[j]]
		})
		for _, reason := range reasons {
			messageParts = append(messageParts, fmt.Sprintf("  - `%s`: `%d` 个", reason, failedProxiesStats[reason]))
		}
	}

	finalTelegramMessage := strings.Join(messageParts, "\n")
	finalTelegramMessage = escapeMarkdownV2(finalTelegramMessage)

	sendSecureTelegramMessage(finalTelegramMessage)
}

// removeDuplicateProxies 移除重复的代理
func removeDuplicateProxies(proxies []*ProxyInfo) []*ProxyInfo {
	seen := make(map[string]bool)
	var unique []*ProxyInfo

	for _, proxy := range proxies {
		// 使用URL作为唯一标识符（包含协议、认证、主机、端口）
		key := proxy.URL
		if !seen[key] {
			seen[key] = true
			unique = append(unique, proxy)
		}
	}

	return unique
}

// createTransportWithProxy 创建一个带代理的 http.Transport (从原始代码复制)
func createTransportWithProxy(proxyURL string) (*http.Transport, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	switch parsedURL.Scheme {
	case "http":
		// HTTP代理处理
		proxyFunc := http.ProxyURL(parsedURL)
		return &http.Transport{
			Proxy:       proxyFunc,
			DialContext: dialer.DialContext,
		}, nil
	case "https":
		// HTTPS代理 - 使用CONNECT隧道方式
		proxyFunc := http.ProxyURL(parsedURL)
		return &http.Transport{
			Proxy:             proxyFunc,
			DialContext:       dialer.DialContext,
			ForceAttemptHTTP2: false, // 避免HTTP/2干扰代理连接
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // 跳过证书验证以避免证书问题
		}, nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if parsedURL.User != nil {
			password, _ := parsedURL.User.Password()
			auth = &proxy.Auth{User: parsedURL.User.Username(), Password: password}
		}

		socks5Dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, dialer)
		if err != nil {
			return nil, err
		}

		return &http.Transport{
			DialContext: socks5Dialer.(proxy.ContextDialer).DialContext,
		}, nil
	case "socks4":
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{User: parsedURL.User.Username()}
		}

		socks4Dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, dialer)
		if err != nil {
			return nil, err
		}

		return &http.Transport{
			DialContext: socks4Dialer.(proxy.ContextDialer).DialContext,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的协议: %s", parsedURL.Scheme)
	}
}


// escapeMarkdownV2 对字符串进行转义以符合MarkdownV2规范 (从原始代码复制)
func escapeMarkdownV2(text string) string {
	var escaped bytes.Buffer
	for _, r := range text {
		switch r {
		case '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!':
			escaped.WriteRune('\\')
			escaped.WriteRune(r)
		default:
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}

// runProxyTests 并发测试代理 (从原始代码复制)
func runProxyTests(proxiesChan chan *ProxyInfo) chan ProxyResult {
	resultsChan := make(chan ProxyResult)
	var wg sync.WaitGroup

	// 启动 worker goroutine
	for i := 0; i < config.Settings.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range proxiesChan {
				result := testProxy(context.Background(), p)
				resultsChan <- result
			}
		}()
	}

	// 启动一个 goroutine 来关闭结果通道
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	return resultsChan
}

// testProxy 测试单个代理的有效性 (从原始代码复制)
func testProxy(ctx context.Context, proxyInfo *ProxyInfo) ProxyResult {
	start := time.Now()
	_, err := url.Parse(proxyInfo.URL)
	if err != nil {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("URL解析错误: %v", err)}
	}

	var transport *http.Transport
	transport, err = createTransportWithProxy(proxyInfo.URL)
	if err != nil {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("创建代理客户端失败: %v", err)}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(config.Settings.CheckTimeout) * time.Second,
	}

	// 根据代理协议选择合适的测试URL
	testURL := selectTestURL(proxyInfo.Protocol)

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("创建请求失败: %v", err)}
	}

	resp, err := client.Do(req)
	if err != nil {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("网络错误: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("HTTP Status: %d", resp.StatusCode)}
	}

	latency := time.Since(start).Seconds() * 1000 // 转换为毫秒
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProxyResult{URL: proxyInfo.URL, Success: false, Reason: fmt.Sprintf("读取响应失败: %v", err)}
	}

	// 解析JSON响应获取IP地址
	var ipResponse struct {
		Origin string `json:"origin"`
		IP     string `json:"ip"`
	}
	ipAddr := ""
	if err := json.Unmarshal(body, &ipResponse); err != nil {
		// 如果JSON解析失败，尝试直接使用响应内容
		ipAddr = strings.TrimSpace(string(body))
	} else {
		// 优先使用 origin 字段，其次使用 ip 字段
		if ipResponse.Origin != "" {
			ipAddr = ipResponse.Origin
		} else if ipResponse.IP != "" {
			ipAddr = ipResponse.IP
		}
	}

	// 检测IP类型
	var ipType, ipDetails string
	if ipAddr != "" && config.IPDetection.Enabled {
		typeInfo := detectIPType(ipAddr)
		ipType = typeInfo.Type
		ipDetails = typeInfo.Details
	} else {
		ipType = "unknown"
		ipDetails = "未检测"
	}

	// 获取国家代码并存储在IPDetails中（如果GeoIP可用）
	if ipAddr != "" && geoIPManager.reader != nil {
		countryCode := getCountryFromIP(ipAddr)
		if countryCode != "" {
			// 保存国家代码到IPDetails字段，覆盖IP类型检测的详细信息
			ipDetails = countryCode
		}
	}

	return ProxyResult{
		URL:       proxyInfo.URL,
		Protocol:  proxyInfo.Protocol,
		Latency:   latency,
		Success:   true,
		IP:        ipAddr,
		IPType:    ipType,
		IPDetails: ipDetails,
		Reason:    "",
	}
}

// selectTestURL 根据代理协议选择最合适的测试URL (从原始代码复制)
func selectTestURL(protocol string) string {
	switch protocol {
	case "https":
		// HTTPS代理优先使用HTTPS测试URL
		return "https://httpbin.org/ip"
	case "http":
		// HTTP代理使用HTTP测试URL
		return "http://httpbin.org/ip"
	default:
		// SOCKS等代理可以使用HTTP或HTTPS，优先HTTP
		return "http://httpbin.org/ip"
	}
}

// writeValidProxies 将有效的代理列表写入相应的输出文件 (从原始代码复制)
func writeValidProxies(validProxies []ProxyResult) {
	if _, err := os.Stat(config.Settings.OutputDir); os.IsNotExist(err) {
		os.Mkdir(config.Settings.OutputDir, 0755)
	}

	groupedProxies := make(map[string][]ProxyResult)
	var residentialProxies []ProxyResult // 收集所有住宅IP

	for _, proxy := range validProxies {
		key := strings.Replace(proxy.Protocol, "socks5h", "socks5", 1)
		groupedProxies[key] = append(groupedProxies[key], proxy)

		// 为socks5代理单独处理Telegram格式
		if key == "socks5_auth" || key == "socks5_noauth" {
			groupedProxies[key+"_tg"] = append(groupedProxies[key+"_tg"], proxy)
		}

		// 收集住宅IP到专用列表
		if proxy.IPType == "residential" {
			residentialProxies = append(residentialProxies, proxy)
			log.Printf("🏠 发现住宅IP: %s (国家: %s)\n", proxy.URL, proxy.IPDetails)
		}
	}

	log.Printf("📊 总共发现 %d 个住宅IP\n", len(residentialProxies))

	// 处理住宅IP专用文件
	if len(residentialProxies) > 0 {
		// 按延迟排序住宅IP
		sort.Slice(residentialProxies, func(i, j int) bool {
			return residentialProxies[i].Latency < residentialProxies[j].Latency
		})

		// 写入标准住宅IP文件
		log.Printf("💾 开始写入标准住宅IP文件...\n")
		writeResidentialFile("residential.txt", residentialProxies, false)
		// 写入Telegram格式住宅IP文件
		log.Printf("💾 开始写入Telegram格式住宅IP文件...\n")
		writeResidentialFile("residential_tg.txt", residentialProxies, true)

		log.Printf("🏠 发现 %d 个住宅IP，已保存到专用文件: residential.txt, residential_tg.txt\n", len(residentialProxies))
	}

	for key, file := range OUTPUT_FILES {
		proxies := groupedProxies[key]
		fullPath := filepath.Join(config.Settings.OutputDir, file)

		// 跳过住宅文件，因为它们已经被单独处理
		if key == "residential" || key == "residential_tg" {
			log.Printf("ℹ️ 跳过住宅文件 %s (已单独处理)\n", file)
			continue
		}

		if len(proxies) > 0 {
			sort.Slice(proxies, func(i, j int) bool {
				return proxies[i].Latency < proxies[j].Latency
			})

			outFile, err := os.Create(fullPath)
			if err != nil {
				log.Printf("❌ 写入文件 %s 失败: %v\n", fullPath, err)
				continue
			}
			defer outFile.Close()

			for _, p := range proxies {
				countryCode := p.IPDetails
				if countryCode == "" {
					countryCode = "UNKNOWN"
				}
				flag := COUNTRY_FLAG_MAP[countryCode]
				if flag == "" {
					flag = COUNTRY_FLAG_MAP["UNKNOWN"]
				}
				countryName := COUNTRY_CODE_TO_NAME[countryCode]

				// 获取IP类型信息
				ipTypeIcon := IP_TYPE_MAP[p.IPType]
				if ipTypeIcon == "" {
					ipTypeIcon = IP_TYPE_MAP["unknown"]
				}
				ipTypeDesc := IP_TYPE_DESCRIPTION[p.IPType]
				if ipTypeDesc == "" {
					ipTypeDesc = IP_TYPE_DESCRIPTION["unknown"]
				}

				var line string
				if strings.HasSuffix(key, "_tg") {
					parsedURL, _ := url.Parse(p.URL)
					query := url.Values{}
					query.Set("server", parsedURL.Hostname())
					query.Set("port", parsedURL.Port())
					if parsedURL.User != nil {
						query.Set("user", parsedURL.User.Username())
						password, _ := parsedURL.User.Password()
						query.Set("pass", password)
					}
					deepLink := fmt.Sprintf("https://t.me/socks?%s", query.Encode())
					line = fmt.Sprintf("%s, 延迟: %.2fms, 国家: %s %s, %s %s\n",
						deepLink, p.Latency, flag, countryName, ipTypeIcon, ipTypeDesc)
				} else {
					line = fmt.Sprintf("%s, 延迟: %.2fms, 国家: %s %s, %s %s\n",
						p.URL, p.Latency, flag, countryName, ipTypeIcon, ipTypeDesc)
				}
				outFile.WriteString(line)
			}
			log.Printf("💾 已写入 %d 条代理到文件: %s\n", len(proxies), fullPath)
		} else {
			if _, err := os.Stat(fullPath); err == nil {
				os.Remove(fullPath)
				log.Printf("🗑️ 已删除空文件: %s\n", fullPath)
			} else {
				log.Printf("ℹ️ 文件 %s 不存在或为空，跳过写入。\n", fullPath)
			}
		}
	}
}

// writeResidentialFile 写入住宅IP专用文件
func writeResidentialFile(fileName string, residentialProxies []ProxyResult, isTGFormat bool) {
	fullPath := filepath.Join(config.Settings.OutputDir, fileName)

	outFile, err := os.Create(fullPath)
	if err != nil {
		log.Printf("❌ 写入住宅IP文件 %s 失败: %v\n", fullPath, err)
		return
	}
	defer outFile.Close()

	for _, p := range residentialProxies {
		countryCode := p.IPDetails
		if countryCode == "" {
			countryCode = "UNKNOWN"
		}
		flag := COUNTRY_FLAG_MAP[countryCode]
		if flag == "" {
			flag = COUNTRY_FLAG_MAP["UNKNOWN"]
		}
		countryName := COUNTRY_CODE_TO_NAME[countryCode]
		ipTypeDesc := "住宅IP"
		ipTypeIcon := "🏠"

		var line string
		if isTGFormat {
			// Telegram格式：创建t.me/socks链接
			parsedURL, _ := url.Parse(p.URL)
			query := url.Values{}
			query.Set("server", parsedURL.Hostname())
			query.Set("port", parsedURL.Port())
			if parsedURL.User != nil {
				query.Set("user", parsedURL.User.Username())
				password, _ := parsedURL.User.Password()
				query.Set("pass", password)
			}
			deepLink := fmt.Sprintf("https://t.me/socks?%s", query.Encode())
			line = fmt.Sprintf("%s, 延迟: %.2fms, 国家: %s %s, %s %s\n",
				deepLink, p.Latency, flag, countryName, ipTypeIcon, ipTypeDesc)
		} else {
			// 标准格式
			line = fmt.Sprintf("%s, 延迟: %.2fms, 国家: %s %s, %s %s\n",
				p.URL, p.Latency, flag, countryName, ipTypeIcon, ipTypeDesc)
		}
		outFile.WriteString(line)
	}

	log.Printf("💾 已写入 %d 个住宅IP到文件: %s\n", len(residentialProxies), fullPath)
}

// sendTelegramFile 发送 Telegram 文件（使用 aigo.go 的方式）
func sendTelegramFile(filePath string) bool {
	// 检查Telegram配置
	if config.Telegram.BotToken == "" || config.Telegram.ChatID == "" {
		log.Println("❌ 未配置 TELEGRAM_BOT_TOKEN 或 TELEGRAM_CHAT_ID，跳过 Telegram 文件通知")
		return false
	}

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log.Printf("ℹ️ 文件 %s 不存在，跳过推送。\n", filepath.Base(filePath))
		return false
	}
	if err != nil {
		log.Printf("❌ 检查文件 %s 失败: %v\n", filePath, err)
		return false
	}
	if fileInfo.Size() == 0 {
		log.Printf("ℹ️ 文件 %s 为空 (%d 字节)，跳过推送。\n", filepath.Base(filePath), fileInfo.Size())
		os.Remove(filePath)
		return false
	}

	log.Printf("📄 准备发送文件: %s (%.2f MB)\n", filepath.Base(filePath), float64(fileInfo.Size())/1024/1024)

	// 获取Telegram客户端
	client := getTelegramClient()
	if client == nil {
		log.Println("❌ 无法建立网络连接，跳过 Telegram 文件发送。")
		return false
	}

	// 构建请求
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", config.Telegram.BotToken)

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("❌ 无法打开文件 %s: %v\n", filePath, err)
		return false
	}
	defer file.Close()

	// 创建multipart表单
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件字段
	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		log.Printf("❌ 创建 multipart 表单文件失败: %v\n", err)
		return false
	}

	// 复制文件内容
	copied, err := io.Copy(part, file)
	if err != nil {
		log.Printf("❌ 复制文件到表单失败: %v\n", err)
		return false
	}
	log.Printf("📋 文件内容已复制到表单 (%d 字节)\n", copied)

	// 添加chat_id字段
	if err := writer.WriteField("chat_id", config.Telegram.ChatID); err != nil {
		log.Printf("❌ 添加 chat_id 字段失败: %v\n", err)
		return false
	}

	// 关闭writer
	if err := writer.Close(); err != nil {
		log.Printf("❌ 关闭 multipart writer 失败: %v\n", err)
		return false
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		log.Printf("❌ 创建 HTTP 请求失败: %v\n", err)
		return false
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	log.Printf("📤 正在发送文件到 Telegram...")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ 文件 %s 发送失败: %v\n", filepath.Base(filePath), err)
		// 如果发送失败，清除缓存客户端
		clientCacheMutex.Lock()
		telegramClientCache = nil
		clientCacheMutex.Unlock()
		log.Println("⚠️ Telegram 客户端已失效，已清除缓存，下次将重新验证。")
		return false
	}
	defer resp.Body.Close()

	// 检查响应
	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("📨 Telegram API 响应状态: %d\n", resp.StatusCode)

	var apiResp telegramAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		log.Printf("❌ 解析 Telegram API 响应失败: %v\n", err)
		log.Printf("📨 原始响应: %s\n", string(bodyBytes))
		return false
	}

	if !apiResp.Ok {
		log.Printf("❌ Telegram API 错误: %s\n", apiResp.Description)
		// 如果API返回错误，清除缓存客户端
		clientCacheMutex.Lock()
		telegramClientCache = nil
		clientCacheMutex.Unlock()
		return false
	}

	log.Printf("✅ 文件 %s 已成功推送到 Telegram。\n", filepath.Base(filePath))
	return true
}

// quickNetworkTest 快速网络连接测试
func quickNetworkTest(proxyURL string) bool {
	var transport *http.Transport
	var err error

	if proxyURL == "" {
		transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
		}
	} else {
		transport, err = createTransportWithProxy(proxyURL)
		if err != nil {
			return false
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	// 使用一个简单的URL测试连接
	testURL := "https://httpbin.org/ip"
	req, err := http.NewRequestWithContext(context.Background(), "GET", testURL, nil)
	if err != nil {
		return false
	}

	// 只尝试一次，不重试
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
