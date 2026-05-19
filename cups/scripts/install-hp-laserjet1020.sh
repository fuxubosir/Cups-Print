#!/usr/bin/env bash
# HP LaserJet 1020 / 1020 Plus 固件安装 + A4-default PPD 派生脚本。
#
# 背景（issue #40）：
# HP LaserJet 1020 系列属于"host-based"打印机（也称 GDI 打印机），打印机内部
# 没有 ROM 存储固件，每次上电后必须由主机上传固件（sihp1020.dl）才能工作。
# Debian 的 printer-driver-foo2zjs 包已提供了驱动过滤器（foo2zjs / foo2zjs-wrapper），
# 但固件文件本身因版权限制不包含在 Debian 包内，需要额外下载。
#
# 本脚本负责：
#   ① 从本仓库 GitHub Releases 镜像下载 sihp1020.dl 固件文件；
#   ② 放置到 /usr/share/foo2zjs/firmware/ 目录（foo2zjs 标准固件路径）；
#   ③ 从 foo2zjs.ppd-compiled 派生一份默认 A4 纸张的 PPD，安装到
#      /usr/share/cups/model/HP/HP-LaserJet_1020-foo2zjs-A4.ppd（issue #48）。
#
# 固件上传到 USB 设备实际由 foo2xqx-wrapper 在每次打印时按需完成——
# 容器内没有 udev，但 foo2xqx-wrapper 调用 foo2zjs 自带的 hotplug 逻辑
# （检测 VID:PID = 03f0:2b17 的设备），把 sihp1020.dl 写入对应 USB 设备节点。
#
# ────────────────────────────────────────────────────────────────────
# 下载策略
# ────────────────────────────────────────────────────────────────────
# 与 install-escpr2.sh / install-konica-bizhub.sh 同模式：只从本仓库自维护的
# GitHub Releases 镜像（tag = cups-driver）下载，避免 HP 官方下载链路在 CI 里
# 的不稳定性。fail-fast：下载失败立即非零退出。
# 升级/替换固件：在本仓库 cups-driver release 上传新版文件，修改下方 URL。
#
# ────────────────────────────────────────────────────────────────────
# A4-default PPD 的来由（issue #48）
# ────────────────────────────────────────────────────────────────────
# foo2zjs 上游 HP-LaserJet_1020.ppd 的 *DefaultPageSize 是 Letter（美国默认），
# 但 *PageSize 列表里包含 A4。CUPS 把 PPD 的 PageSize 列表暴露给 IPP
# media-supported，把 *DefaultPageSize 暴露给 IPP media-default。
#
# 苹果设备走 AirPrint（IPP）连接 CUPS 共享出来的打印机时，会优先按
# media-default 渲染首屏纸张选项。当 default 是 Letter 时，国内常用的 A4
# 在 iOS 打印面板里会被折叠/隐藏，用户反映"无 A4 选项"。
#
# 修复手法：把 foo2zjs 的 HP 1020 PPD 抽出来，把四组默认值（PageSize /
# PageRegion / ImageableArea / PaperDimension）从 Letter 改成 A4，并改写
# NickName 让它在 CUPS UI / lpinfo -m 里清晰可辨。原 foo2zjs.ppd-compiled
# 里的 Letter-default PPD 保留不动——用户可在添加打印机时按需选择 A4 变体，
# 或对已存在的打印机用 lpadmin -p NAME -o media-default=iso_a4_210x297mm
# 切换。

set -eo pipefail

# ────────────────────────────────────────────────────────────────────
# 配置
# ────────────────────────────────────────────────────────────────────
FW_FILENAME="sihp1020.dl"
FW_MIRROR_URL="https://github.com/hanxi/cups-web/releases/download/cups-driver/${FW_FILENAME}"
FW_INSTALL_DIR="/usr/share/foo2zjs/firmware"

PYPPD_ARCHIVE="/usr/share/cups/model/foo2zjs.ppd-compiled"
PPD_INSTALL_DIR="/usr/share/cups/model/HP"
PPD_INSTALL_NAME="HP-LaserJet_1020-foo2zjs-A4.ppd"
# Trixie 起 printer-driver-foo2zjs-common 改成铺原始 PPD（不再 pyppd 打包），
# 候选路径按优先级列出，找到第一个就用。
PPD_SEARCH_DIRS=(
    /usr/share/cups/model/foo2zjs
    /usr/share/cups/model
    /usr/share/ppd/foo2zjs
    /usr/share/ppd
)

# ────────────────────────────────────────────────────────────────────
# 下载 & 安装固件
# ────────────────────────────────────────────────────────────────────
mkdir -p "${FW_INSTALL_DIR}"

echo "[hp-laserjet1020] downloading firmware from ${FW_MIRROR_URL}"
curl -fL --retry 3 --retry-delay 3 -o "${FW_INSTALL_DIR}/${FW_FILENAME}" "${FW_MIRROR_URL}"

# 校验文件非空
if [ ! -s "${FW_INSTALL_DIR}/${FW_FILENAME}" ]; then
    echo "[hp-laserjet1020] FATAL: downloaded firmware file is empty"
    exit 1
fi

echo "[hp-laserjet1020] installed firmware: ${FW_INSTALL_DIR}/${FW_FILENAME} ($(wc -c < "${FW_INSTALL_DIR}/${FW_FILENAME}") bytes)"

# ────────────────────────────────────────────────────────────────────
# 派生 A4-default PPD（issue #48）
# ────────────────────────────────────────────────────────────────────
# foo2zjs 的 PPD 在不同 Debian 版本里有两种打包方式：
#   ① 老版本（dh_pyppd）：/usr/share/cups/model/foo2zjs.ppd-compiled 单文件可执行
#      archive；调用 `archive cat HP-LaserJet_1020.ppd` 抽内容。见
#      https://github.com/OpenPrinting/pyppd 的 runner.cat 实现：
#      输入会被规范化成 "0/<filename>"，所以传 "HP-LaserJet_1020.ppd" 即可。
#   ② Trixie 起：直接铺到 /usr/share/cups/model/foo2zjs/HP-LaserJet_1020.ppd[.gz]
#      或类似路径（见 PPD_SEARCH_DIRS）。
#
# 注意：HP-LaserJet_1020.ppd 是 foo2zjs 源码的文件名（PPD/HP-LaserJet_1020.ppd），
# 不是 NickName。两种打包方式如果都拿不到非空内容会立即 fail-fast。

PPD_TMP="$(mktemp /tmp/hp1020-a4.ppd.XXXXXX)"
trap 'rm -f "${PPD_TMP}"' EXIT

if [ -x "${PYPPD_ARCHIVE}" ]; then
    echo "[hp-laserjet1020] extracting HP-LaserJet_1020.ppd from pyppd archive ${PYPPD_ARCHIVE}"
    "${PYPPD_ARCHIVE}" cat HP-LaserJet_1020.ppd > "${PPD_TMP}"
else
    PPD_SRC=""
    for dir in "${PPD_SEARCH_DIRS[@]}"; do
        for cand in "${dir}/HP-LaserJet_1020.ppd" "${dir}/HP-LaserJet_1020.ppd.gz"; do
            if [ -f "${cand}" ]; then
                PPD_SRC="${cand}"
                break 2
            fi
        done
    done
    if [ -z "${PPD_SRC}" ]; then
        echo "[hp-laserjet1020] FATAL: HP-LaserJet_1020.ppd not found in pyppd archive nor in ${PPD_SEARCH_DIRS[*]}"
        exit 1
    fi
    echo "[hp-laserjet1020] using PPD source ${PPD_SRC}"
    case "${PPD_SRC}" in
        *.gz) zcat "${PPD_SRC}" > "${PPD_TMP}" ;;
        *)    cat  "${PPD_SRC}" > "${PPD_TMP}" ;;
    esac
fi

if [ ! -s "${PPD_TMP}" ]; then
    echo "[hp-laserjet1020] FATAL: extracted PPD is empty (foo2zjs upstream may have renamed HP-LaserJet_1020.ppd)"
    exit 1
fi

# 校验抽出来的 PPD 确实是 HP 1020、且 PageSize 列表里有 A4。
# 任何一项不满足都说明 foo2zjs 上游结构变了，立即 fail-fast。
if ! grep -q '^\*Product:[[:space:]]*"(HP LaserJet 1020)"' "${PPD_TMP}"; then
    echo "[hp-laserjet1020] FATAL: extracted PPD doesn't look like HP LaserJet 1020"
    exit 1
fi
if ! grep -q '^\*PageSize A4' "${PPD_TMP}"; then
    echo "[hp-laserjet1020] FATAL: A4 not in extracted PPD's PageSize list"
    exit 1
fi
if ! grep -q '^\*DefaultPageSize:[[:space:]]\+Letter' "${PPD_TMP}"; then
    echo "[hp-laserjet1020] FATAL: extracted PPD's *DefaultPageSize is not Letter (upstream may have changed defaults)"
    exit 1
fi

# 把 4 组 *Default*: Letter 改成 A4，同时改 PCFileName / ShortNickName / NickName，
# 让 lpinfo -m 里能跟原 PPD 区分开。
# - PCFileName 是 8.3 风格的内部唯一标识，避免和原 "FOO2ZJS-.PPD" 撞名。
# - ShortNickName ≤ 31 字符（PPD 规范），"HP LaserJet 1020 foo2zjs-z1 A4" = 30 字符，刚好。
sed -i -E '
    s/^\*PCFileName:[[:space:]]+"[^"]*"/\*PCFileName:\t"FOO2A4Z1.PPD"/;
    s/^\*ShortNickName:[[:space:]]+"[^"]*"/\*ShortNickName: "HP LaserJet 1020 foo2zjs-z1 A4"/;
    s|^\*NickName:[[:space:]]+"[^"]*"|\*NickName:      "HP LaserJet 1020 Foomatic/foo2zjs-z1 A4 (cups-web)"|;
    s/^\*DefaultPageSize:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageSize: A4/;
    s/^\*DefaultPageRegion:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageRegion: A4/;
    s/^\*DefaultImageableArea:[[:space:]]+Letter[[:space:]]*$/\*DefaultImageableArea: A4/;
    s/^\*DefaultPaperDimension:[[:space:]]+Letter[[:space:]]*$/\*DefaultPaperDimension: A4/
' "${PPD_TMP}"

# 验证 4 组默认都改成 A4，否则说明 sed 没命中任何一行（PPD 格式变了）。
for key in DefaultPageSize DefaultPageRegion DefaultImageableArea DefaultPaperDimension; do
    if ! grep -q "^\*${key}:[[:space:]]\+A4" "${PPD_TMP}"; then
        echo "[hp-laserjet1020] FATAL: failed to patch *${key} to A4"
        exit 1
    fi
done

install -d "${PPD_INSTALL_DIR}"
install -m 644 "${PPD_TMP}" "${PPD_INSTALL_DIR}/${PPD_INSTALL_NAME}"

echo "[hp-laserjet1020] installed A4-default PPD: ${PPD_INSTALL_DIR}/${PPD_INSTALL_NAME}"
