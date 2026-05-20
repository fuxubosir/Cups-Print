#!/bin/bash -ex

if [ $(grep -ci $CUPSADMIN /etc/shadow) -eq 0 ]; then
    useradd -r -G lpadmin -M $CUPSADMIN

    # add password
    echo $CUPSADMIN:$CUPSPASSWORD | chpasswd

    # add tzdata
    ln -fs /usr/share/zoneinfo/$TZ /etc/localtime
    dpkg-reconfigure --frontend noninteractive tzdata
fi

# restore default cups config in case user does not have any
if [ ! -f /etc/cups/cupsd.conf ]; then
    cp -rpn /etc/cups-bak/* /etc/cups/
fi

# ── 已添加 HP 1020 打印机的默认纸张 Letter → A4 一次性修补 ──
# issue #48：foo2zjs 上游 HP-LaserJet_1020 PPD 的 *DefaultPageSize 是 Letter。
# 苹果设备走 AirPrint（IPP）时按 media-default 渲染首屏纸张，Letter 默认会让
# 国内常用的 A4 在 iPhone 打印面板里被折叠/隐藏，用户反映"无 A4 选项"。
#
# 已通过 install-hp-laserjet1020.sh 在 /usr/share/cups/model/HP/ 安装了 A4-default
# 变体 PPD，新加的打印机可以直接选这版。但已经按"(recommended)"加好的存量打印机
# 不会自动迁移——它们的 PPD 副本在 /etc/cups/ppd/<printer>.ppd，仍是 Letter 默认。
#
# 这里在 cupsd 启动前对存量副本做一次性原地修补：
#   - 仅处理 foo2zjs HP 1020 PPD（用 *Product 和 *FoomaticIDs 双重指纹）
#   - 仅当当前 *DefaultPageSize 是 Letter（用户没在 CUPS UI 里改过默认纸张）
#   - 仅当 *PageSize 列表里确实声明了 A4
#   - 三个条件都满足才把四组 *Default*: Letter 同步改成 A4
# 任何一条不满足就跳过，不会覆盖用户的显式选择，也不会处理非 HP 1020 的 PPD。
# 修改前先备份成 .bak-cupsweb-issue48，方便用户回退。
if [ -d /etc/cups/ppd ]; then
    for ppd in /etc/cups/ppd/*.ppd; do
        [ -f "$ppd" ] || continue
        grep -q '^\*Product:[[:space:]]*"(HP LaserJet 1020)"' "$ppd" || continue
        grep -q '^\*FoomaticIDs:[[:space:]]\+HP-LaserJet_1020[[:space:]]\+foo2zjs-z1' "$ppd" || continue
        grep -q '^\*DefaultPageSize:[[:space:]]\+Letter[[:space:]]*$' "$ppd" || continue
        grep -q '^\*PageSize A4' "$ppd" || continue

        cp -p "$ppd" "${ppd}.bak-cupsweb-issue48"
        sed -i -E '
            s/^\*DefaultPageSize:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageSize: A4/;
            s/^\*DefaultPageRegion:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageRegion: A4/;
            s/^\*DefaultImageableArea:[[:space:]]+Letter[[:space:]]*$/\*DefaultImageableArea: A4/;
            s/^\*DefaultPaperDimension:[[:space:]]+Letter[[:space:]]*$/\*DefaultPaperDimension: A4/
        ' "$ppd"
        echo "[entrypoint] patched $ppd: HP 1020 default paper Letter → A4 (issue #48; backup at ${ppd}.bak-cupsweb-issue48)"
    done
fi

# ── HP host-based 打印机固件上传（issue #48 真正的修复点） ────────────
# HP LaserJet 1020 / 1018 / 1005 / P100x / P1505 等"GDI / host-based"机型
# 每次上电都要先由主机把固件写入 /dev/usb/lpN 才能进入工作状态。物理机上
# 由 foo2zjs 安装的 udev 规则（/usr/lib/udev/rules.d/85-hplj10xx.rules）
# 根据 USB 产品字串匹配后 RUN+="hpljNNNN"，触发同包提供的
# /usr/lib/udev/hpljNNNN 脚本——脚本内部走 CUPS USB backend 把
# /lib/firmware/hp/sihpNNNN.dl 上传到打印机。
#
# 容器内没有 udev daemon、kernel uevent 也不会自动传进来，原生 udev 规则
# 不会触发，issue #48 用户反馈的"换 A4 PPD 之后还是打不出来 + 手动
# `cat sihp1020.dl > /dev/usb/lp0` 后才能打印"就是这一步缺失。
#
# 这里复用 foo2zjs 上游同一个脚本——不重新发明扫 sysfs 的轮子——只是手动
# 给它喂上 udev 本来会注入的 SUBSYSTEM 环境变量，让脚本里 `[ "$SUBSYSTEM"
# != "usb" ] && exit` 的防御性检查通过，进入 CUPS USB backend 加载分支。
# CUPS backend 自己会通过 libusb 枚举设备 ID 里带 "HP...LaserJet...1020"
# 字串的打印机，把 /lib/firmware/hp/sihp1020.dl 写进去；上传完成后打印机
# 切到 ready 态、产品字串不再含我们关心的型号，无副作用可重复调用。
#
# 后台跑：上游脚本里有两个 `sleep 3`（避开 udev 探测竞态），共 6s；同步
# 调用会拖慢 cupsd 启动。后台跑既不阻塞 cupsd，也不影响 USB backend——
# 上传期间 cupsd 即使被请求打印也会排队，固件就绪后第一份打印就能成功。
#
# 不做定时轮询：CUPS USB backend 不区分"固件已加载/未加载"，重复上传可能
# 干扰正在打印的设备。打印机断电重连后通常 Docker 设备映射也需要重做
# （--device=/dev/usb/lp0 是构建期固定的），届时用户重启容器即可重新触发。
HPLJ_LOADER=/usr/lib/udev/hplj1020
HPLJ_LOG=/var/log/cups/hplj1020-firmware.log
if [ -x "$HPLJ_LOADER" ]; then
    mkdir -p /var/log/cups
    echo "[entrypoint] dispatching $HPLJ_LOADER in background (issue #48); detailed log: $HPLJ_LOG"
    # 子 shell 隔离环境变量与 set -x 跟踪输出，避免污染主流程。
    # 脚本里 log() 函数在无 tty 时默认走 logger → syslog，容器内无 syslog
    # 守护会丢日志；这里把 stderr/stdout 一起重定向到固定文件，配合上一行
    # echo 提示用户去哪里看，方便 issue #48 类问题的复诊。
    (
        set +x
        SUBSYSTEM=usb "$HPLJ_LOADER" >>"$HPLJ_LOG" 2>&1 || true
    ) &
fi

# ── 后台拉起 avahi-daemon 与 ipp-usb：用于 driverless / IPP Everywhere 发现 ──
# 其中 ipp-usb 负责把 USB 直连的 IPP Everywhere 打印机（如 Brother DCP-T425W）
# 暴露成本地 http://localhost 的 IPP 端点，让 CUPS 能把它识别为
# "IPP Everywhere (color)" 机型。两者均允许缺失（某些架构 ipp-usb 可能未安装，
# 或容器未拿到 USB 设备），失败不影响 cupsd 启动。
if command -v avahi-daemon >/dev/null 2>&1; then
    # 不存在 dbus 时 avahi-daemon 会失败，用 --no-rlimits --no-drop-root 简化容器内启动；
    # 如宿主 dbus 不可用则静默跳过。
    mkdir -p /var/run/dbus
    (dbus-daemon --system --fork 2>/dev/null || true)
    (avahi-daemon --daemonize --no-chroot 2>/dev/null || true)
fi
if command -v ipp-usb >/dev/null 2>&1; then
    # ipp-usb 默认走 systemd，容器里直接前台 --no-fork 失败，用后台模式；
    # 拿不到 USB（未挂 /dev/bus/usb）时会自动退出，不影响 cupsd。
    mkdir -p /var/log/ipp-usb /var/lock/ipp-usb
    (ipp-usb >/var/log/ipp-usb/ipp-usb.log 2>&1 &) || true
fi

exec /usr/sbin/cupsd -f
