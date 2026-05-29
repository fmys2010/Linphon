# Linphon 使用说明

当前正式版本：`1.3.1`

Linphon 是一个 Linux 上使用 Psiphon 和 VPNGate 的小工具。你可以把它理解成：先安装一个 `linph` 命令，然后用这个命令安装、启动、停止和管理代理或 VPN。

如果你只是想用，不需要关心技术细节。照着下面命令走就行。

## 一键开始

在服务器上运行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh)
```

这一步只安装 `linph` 命令。用 `curl` 直接运行时，脚本会先拉取仓库源码再构建。如果系统里还没有 Go，脚本会先自动安装发行版里的 Go 包；Debian/Ubuntu 上会使用 `apt-get install golang-go`，不是 `apt install go`。

然后运行：

```bash
linph install
linph start
```

到这里就启动好了。

默认代理地址：

```text
HTTP/HTTPS: 127.0.0.1:8081
SOCKS:      127.0.0.1:1081
```

如果你想安装完马上启动，也可以把第二步合成这样：

```bash
linph install --start
```

## 最常用命令

查看当前端口：

```bash
linph port
```

查看当前地区：

```bash
linph ctry
```

查看运行日志：

```bash
linph log
```

停止：

```bash
linph stop
```

重新启动：

```bash
linph restart
```

再次启动：

```bash
linph start
```

## 切换地区

例如切换到美国：

```bash
linph switch-ctry US
```

切换到日本：

```bash
linph switch-ctry JP
```

也可以准备多个地区：

```bash
linph switch-ctry US,JP,CA
```

常见地区代码示例：

```text
US 美国
JP 日本
CA 加拿大
SG 新加坡
GB 英国
DE 德国
FR 法国
NL 荷兰
```

## 修改端口

默认端口是：

```text
HTTP/HTTPS: 8081
SOCKS:      1081
```

如果你想改成别的端口，例如 HTTP 用 `18080`，SOCKS 用 `18081`：

```bash
linph switch-port 18080 18081
```

改完后查看：

```bash
linph port
```

## 多开几个代理

如果你想同时开多个本地代理，例如 2 个：

```bash
linph psi set psiphon --slot-count 2 --regions US,JP
linph restart
```

然后查看端口：

```bash
linph port
```

它会显示每个代理对应的端口。

如果要指定起始端口：

```bash
linph psi set psiphon --slot-count 2 --regions US,JP --http-port 18080 --socks-port 18081
linph restart
```

## 浏览器怎么用

在浏览器或系统代理设置里填：

```text
HTTP 代理:  127.0.0.1  8081
HTTPS 代理: 127.0.0.1  8081
SOCKS 代理: 127.0.0.1  1081
```

如果你改过端口，就填你自己改后的端口。

设置好后，打开 IP 查询网站，看看出口 IP 是否变化。

## 使用 VPNGate

默认安装后用的是 Psiphon，也就是本地 HTTP/SOCKS 代理。如果你想改用 VPNGate，可以先准备系统里的 OpenVPN：

```bash
sudo apt install openvpn
```

VPNGate/OpenVPN 通常需要 root 权限或 `CAP_NET_ADMIN`，机器上还要有 `/dev/net/tun`。

然后启用 VPNGate：

```bash
linph vg set vpngate --regions US --activate
linph start
```

VPNGate 走的是系统 VPN 路由，不是本地代理端口，所以启用后一般不需要再给浏览器填 `127.0.0.1:8081`。

常用命令：

```bash
# 查看当前 provider
linph provider get

# 切换 VPNGate 地区
linph switch-ctry JP

# 刷新 VPNGate 服务器列表
linph vg refresh

# 切回 Psiphon 代理
linph provider set psiphon
linph restart
```

VPNGate 是志愿者服务器，稳定性会波动。如果某个地区连不上，可以换一个地区再试。

`linph provider set vg` 只会切换到已经通过 `linph vg set ...` 配好的 VPNGate 配置；它不会自动创建新的 live 配置。

如果你要调试自建服务器列表或离线 fixture，才需要考虑 `--allow-insecure-api-url`、`--allow-local-api-url` 或 `--allow-unsafe-cache-path` 这类高级覆盖项。

## 查看当前 provider

普通用户一般不用管 provider。你只需要知道：

```text
psi = Psiphon，本地 HTTP/SOCKS 代理
vg  = VPNGate，系统 VPN 路由，需要 OpenVPN
```

如果你想看当前 provider：

```bash
linph provider get
```

如果需要重新设置为 Psiphon：

```bash
linph provider set psiphon
```

如果需要切换到 VPNGate：

```bash
linph provider set vg
```

注意：这条命令要求你已经运行过 `linph vg set vpngate --regions US --activate` 配好 VPNGate。第一次切换 VPNGate 时，优先用 `linph vg set ... --activate`。

## 卸载

普通卸载：

```bash
linph uninstall
```

或者：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/uninstall.sh)
```

普通卸载会尽量保留配置文件，方便以后再装。

如果你想连配置一起删掉：

```bash
linph uninstall --purge
```

或者：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/uninstall.sh) --purge
```

## 老用户的一步安装方式

如果你想用旧版那种“运行脚本后完整安装并自动启动”的方式，可以用：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh) --legacy-full-install
```

这个模式主要给老用户和旧脚本过渡使用。新用户建议用前面的新流程：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh)
linph install
linph start
```

## 常见问题

### 运行 `linph` 提示找不到命令怎么办？

先重新打开一个终端，或者运行：

```bash
export PATH="/usr/local/bin:$PATH"
```

再试：

```bash
linph --help
```

查看版本：

```bash
linph --version
```

### 启动失败怎么办？

先看日志：

```bash
linph log
```

然后可以尝试重启：

```bash
linph restart
```

### 端口被占用了怎么办？

换一组端口：

```bash
linph switch-port 18080 18081
```

然后查看：

```bash
linph port
```

### 想改地区怎么办？

例如改成日本：

```bash
linph switch-ctry JP
```

### 想恢复默认端口怎么办？

```bash
linph switch-port 8081 1081
```

### `curl` 安装安全吗？

这个命令是为了方便使用：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh)
```

它适合你信任这个 GitHub 仓库时使用。更严格的安全场景，应使用经过签名验证的 release 包，或者先人工检查脚本内容再运行。

## 命令速查

```bash
# 第一次安装
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh)
linph install
linph start

# 安装后立刻启动
linph install --start

# 查看状态信息
linph port
linph ctry
linph log

# 控制运行
linph start
linph stop
linph restart

# 切换地区和端口
linph switch-ctry JP
linph switch-port 18080 18081

# 多开两个代理
linph psi set psiphon --slot-count 2 --regions US,JP
linph restart

# 卸载
linph uninstall

# 连配置一起删除
linph uninstall --purge
```
