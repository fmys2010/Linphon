# Linphon

Linphon 是一个面向 Linux 的 Psiphon 封装项目。当前主入口是统一的 `linph` 命令；repo-local manager、多实例 harness、staged runner、安装/卸载逻辑都在 Go 内实现，同时保留原有兼容命令名：`psiphon`、`plinstaller2`、`pluninstaller`。已安装流程现在支持一个持久化的多槽 control plane。

## 项目提供什么

- 一个统一主入口：`linph`
- 一个 repo-local 的单地区管理器：`linph mg`（兼容 `psiphon-mg`）
- 一个 repo-local 的多实例 harness：`linph multi`（兼容 `psiphon-multi-instance`）
- 一个 staged 回归入口：`linph staged`（兼容 `run-psiphon-staged`）
- 一组兼容保留的全局入口名：`psiphon`、`pluninstaller`、`plinstaller2`
- 一套纯 Go 的可重复测试：`go test ./...`

## 快速开始

### 1）构建命令

```
mkdir -p tools/psiphon-mg/bin
cd tools/psiphon-mg
go build -o ../../tools/psiphon-mg/bin/psiphon-mg ./cmd/psiphon-mg
go build -o ../../tools/psiphon-mg/bin/psiphon-multi-instance ./cmd/psiphon-multi-instance
go build -o ../../tools/psiphon-mg/bin/run-psiphon-staged ./cmd/run-psiphon-staged
go build -o ../../tools/psiphon-mg/bin/linph ./cmd/linph
go build -o ../../tools/psiphon-mg/bin/psiphon ./cmd/psiphon
go build -o ../../tools/psiphon-mg/bin/pluninstaller ./cmd/pluninstaller
go build -o ../../tools/psiphon-mg/bin/plinstaller2 ./cmd/plinstaller2
```

### 2）一键安装 / 卸载

推荐直接使用仓库根目录脚本：

```
bash ./install.sh
bash ./uninstall.sh
bash ./uninstall.sh --purge
```

- `install.sh` 会本地构建 `linph`；在交互终端且未传参时会先问是否安装，再问单端口还是多端口，然后收集端口/地区并执行 `linph install`
- 显式传入 install 参数，或在非交互环境调用时，`install.sh` 会保持原有参数透传行为
- `uninstall.sh` 会优先调用已安装的 `linph uninstall`
- 默认卸载会保留 `/etc/psiphon/psiphon.config`
- `--purge` 会删除整个 `/etc/psiphon`

### 3）管理已安装实例

```
linph run
linph start
linph restart
linph stop
linph port
linph ctry
linph log
linph switch-port 18080 10880
linph switch-ctry US,CA,JP
```

`linph run` / `psiphon` 仍然是原始的已安装前台运行路径。其余命令操作已安装的多槽 profile：

- `start` / `restart` / `stop`：管理所有已安装槽
- `port`：打印所有槽的端口对
- `ctry`：打印启用地区
- `log`：跟随已安装日志，直到 Ctrl-C
- `switch-port`：更新起始端口并在运行时重启应用
- `switch-ctry`：更新地区并在运行时重启应用

### 4）启动单地区 manager

```
tools/psiphon-mg/bin/linph mg start US \
  --binary ./archive/psiphon-tunnel-core-x86_64
```

### 5）切换、停止、查看状态

```
tools/psiphon-mg/bin/linph mg switch CA
tools/psiphon-mg/bin/linph mg status
tools/psiphon-mg/bin/linph mg current-region
tools/psiphon-mg/bin/linph mg stop
```

旧命令 `psiphon-mg ...` 仍然可用。

### 6）运行多实例 harness

```
tools/psiphon-mg/bin/linph multi run \
  --binary ./archive/psiphon-tunnel-core-x86_64 \
  --count 3
```

也可以显式绑定地区和端口：

```
tools/psiphon-mg/bin/linph multi run \
  --binary ./archive/psiphon-tunnel-core-x86_64 \
  --instance US:19080:12080 \
  --instance JP:19081:12081
```

`--instance` 模式不能和 `--count`、`--regions`、`--http-port-base`、`--socks-port-base` 混用。

旧命令 `psiphon-multi-instance ...` 仍然可用。

### 7）运行 staged 回归

```
tools/psiphon-mg/bin/linph staged \
  --binary ./archive/psiphon-tunnel-core-x86_64
```

旧命令 `run-psiphon-staged ...` 仍然可用。

### 8）运行测试

```
cd tools/psiphon-mg
PSIPHON_MG_GO_INTEGRATION=1 go test ./...
```

## 两条主路径

### 1）repo-local 管理与测试

这是当前推荐路径，适合：

- 单地区启动
- 地区切换
- 状态查看
- 多实例压测/离线回归
- 开发期验证

默认情况下：

- `linph mg` / `psiphon-mg` 使用 `./psiphon.config` 作为模板
- 运行时状态写入 `./.work/psiphon-mg`
- 多实例 harness 产物写入 `./.work/psiphon-harness`
- staged 产物写入 `./.work/psiphon-harness-staged`

如果二进制不在仓库根目录附近，可以显式指定仓库根：

```
PSIPHON_MG_REPO_ROOT=/path/to/Linphon tools/psiphon-mg/bin/linph mg status
```

也可以直接覆盖关键路径：

```
tools/psiphon-mg/bin/linph mg status \
  --base-config /path/to/psiphon.config \
  --runtime-root /tmp/psiphon-mg
```

### 2）全局兼容入口

当前推荐的全局入口是 `linph`，兼容入口 `psiphon`、`pluninstaller`、`plinstaller2` 都会路由到同一套 Go 逻辑。

- `linph run` / `psiphon`：执行已安装的 `/etc/psiphon/psiphon-tunnel-core-x86_64 -config /etc/psiphon/psiphon.config`
- `linph install` / `plinstaller2`：从**本地人工审核制品**安装 `linph`、兼容别名、tunnel-core 和配置，并持久化/启动已安装多槽 profile
- `linph uninstall` / `pluninstaller`：卸载 `linph` 和兼容别名；默认保留配置，`--purge` 删除整个配置目录

默认安装位置：

- `linph` -> `/usr/local/bin/linph`
- `psiphon` / `plinstaller2` / `pluninstaller` -> `/usr/local/bin/*`
- `archive/psiphon-tunnel-core-x86_64` -> `/etc/psiphon/psiphon-tunnel-core-x86_64`
- `psiphon.config` -> `/etc/psiphon/psiphon.config`

安装完成后可直接运行：

```
linph run
```

也可以直接使用多槽 control plane：

```bash
linph port
linph ctry
linph stop
```

默认代理端口：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS4/5：`127.0.0.1:1081`

## 目录结构

- `psiphon.config`：基础模板配置
- `archive/`：人工审核本地制品与旧流程说明
- `install.sh` / `uninstall.sh`：一键安装与卸载脚本
- `tools/psiphon-mg/cmd/`：所有 Go 命令入口
- `tools/psiphon-mg/internal/mg/`：manager、harness、staged、entrypoint 的核心逻辑
- `tools/psiphon-mg/internal/testhelper/`：测试辅助二进制
- `.work/`：运行时和测试产物目录，可安全清理

## `psiphon.config` 最重要的字段

通常真正会改的只有这几个：

- `LocalHttpProxyPort`：本地 HTTP/HTTPS 代理端口
- `LocalSocksProxyPort`：本地 SOCKS 代理端口
- `EgressRegion`：目标出口地区，例如 `US`、`CA`、`JP`

对于多实例场景，还需要关注：

- `RemoteServerListDownloadFilename`

多实例运行时必须做隔离，避免多个实例写同一份缓存/续传状态。当前 Go harness 已经为每个实例生成独立文件名。

## 行为边界

当前 Go 实现的定位是：

- `linph mg` / `psiphon-mg` 一次只管理一个 active region
- 地区切换采用 stop/start 语义，不做热重载
- `status` 基于进程状态和 notices 派生信号，不自动等价于完整端到端网络可用性证明
- `linph multi run` / `psiphon-multi-instance run` 的成功标准是“启动宽限期后进程仍存活”，而不是 tunnel 一定已经完全联通

出于安全考虑，下列能力仍然禁用：

- `psiphon-mg --download-if-missing`
- `psiphon-mg --download-url`
- `linph multi download-binary`
- `linph multi run --download-if-missing`
- `linph staged --download-if-missing`
- `psiphon-multi-instance download-binary`
- `psiphon-multi-instance run --download-if-missing`
- `run-psiphon-staged --download-if-missing`

如果要运行，请显式提供已经审核过的 tunnel-core，例如：

```
--binary ./archive/psiphon-tunnel-core-x86_64
```

## 常见问题

### 浏览器怎么接代理？

浏览器代理设置里填：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS：`127.0.0.1:1081`

然后访问 IP 检测网站确认出口是否已经变化。

### 怎么切换地区？

如果你使用 repo-local manager：

```
tools/psiphon-mg/bin/linph mg start US --binary ./archive/psiphon-tunnel-core-x86_64
tools/psiphon-mg/bin/linph mg switch JP
```

如果你使用已安装的全局流程，则修改 `/etc/psiphon/psiphon.config` 里的：

```
"EgressRegion": "US"
```

### `archive/` 目录还有用吗？

有，但它现在只承担两件事：

- 保存人工审核过的本地制品
- 保存旧流程说明

当前主线的运行逻辑不依赖 shell 包装层；`install.sh` / `uninstall.sh` 只是为了方便调用 `linph install` / `linph uninstall`。

## 项目状态

Linphon 当前是一个 **Go-only orchestration repo**：

- 入口层已经全部迁到 Go
- `linph` 是新的主命令面
- 测试与 CI 也已经迁到 Go
- 仍然依赖外部 `psiphon-tunnel-core-x86_64` 二进制，但不再依赖 shell 包装层
