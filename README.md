# Linphon 使用说明

Linphon 是一个 Linux 上使用 Psiphon 的小工具。你可以把它理解成：先安装一个 `linph` 命令，然后用这个命令安装、启动、停止和管理代理。

如果你只是想用，不需要关心技术细节。照着下面命令走就行。

## 一键开始

在服务器上运行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/fmys2010/Linphon/main/install.sh)
```

这一步只安装 `linph` 命令。

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

## 查看当前 provider

当前第一阶段只支持 Psiphon provider。一般不用管这个。

如果你想看当前 provider：

```bash
linph provider get
```

如果需要重新设置为 Psiphon：

```bash
linph provider set psiphon
```

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
