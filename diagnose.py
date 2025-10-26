#!/usr/bin/env python3
"""
诊断IP检测工具的问题
"""

import os
import re
from pathlib import Path

def diagnose_issues():
    """诊断问题"""
    print("=== IP检测工具问题诊断 ===\n")

    # 检查程序文件
    files_to_check = [
        "ip-checker-debug.exe",
        "GeoLite2-Country.mmdb",
        "config.ini"
    ]

    for file in files_to_check:
        path = Path(f"D:/IP/IP检测/IP-validity/{file}")
        if path.exists():
            size = path.stat().st_size
            print(f"✅ {file}: 存在 ({size:,} 字节)")
        else:
            print(f"❌ {file}: 不存在")

    # 检查输出目录
    output_dir = Path("D:/IP/IP检测/IP-validity/OUTPUT")
    if output_dir.exists():
        files = list(output_dir.glob("*"))
        print(f"✅ OUTPUT目录: 存在 ({len(files)} 个文件)")
        for file in files:
            print(f"   - {file.name}")
    else:
        print("❌ OUTPUT目录: 不存在")

    # 检查最新日志
    log_file = Path("D:/IP/IP检测/IP-validity/check_log.txt")
    if log_file.exists():
        with open(log_file, 'r', encoding='utf-8') as f:
            content = f.read()

        print(f"\n=== 日志分析 ===")

        # 查找关键信息
        patterns = [
            ("住宅IP发现", r"🏠 发现.*?个住宅IP"),
            ("文件写入", r"💾 已写入.*?个住宅IP"),
            ("文件删除", r"🗑️ 已删除.*?住宅"),
            ("GeoIP错误", r"GeoIP.*未加载|数据库.*未加载"),
            ("国家代码", r"国家代码.*查询"),
        ]

        for name, pattern in patterns:
            matches = re.findall(pattern, content)
            if matches:
                print(f"✅ {name}: 找到 {len(matches)} 条记录")
                for match in matches[:3]:  # 只显示前3条
                    print(f"   - {match}")
            else:
                print(f"❌ {name}: 未找到相关记录")

        # 查找最新住宅IP信息
        residential_matches = re.findall(r"🏠 住宅IP.*?(\d+\.\d+\.\d+\.\d+)", content)
        if residential_matches:
            print(f"\n🏠 发现的住宅IP示例:")
            for ip in residential_matches[:5]:
                print(f"   - {ip}")
    else:
        print("❌ 日志文件不存在")

    print(f"\n=== 可能的问题和解决方案 ===")
    print("1. 如果GeoIP数据库未加载:")
    print("   - 检查 GeoLite2-Country.mmdb 文件是否损坏")
    print("   - 尝试重新下载GeoIP数据库")
    print("2. 如果住宅IP文件被删除:")
    print("   - 检查写入权限")
    print("   - 检查磁盘空间")
    print("3. 如果国家信息为空:")
    print("   - 检查IPDetails字段是否正确赋值")
    print("   - 验证GeoIP数据库查询结果")

if __name__ == "__main__":
    diagnose_issues()