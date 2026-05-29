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

### 2）引导安装 / 系统安装 / 卸载

推荐直接使用仓库根目录脚本先引导安装 `linph`，再显式执行系统安装：

```
bash ./install.sh
linph install --binary ./archive/psiphon-tunnel-core-x86_64
linph start
bash ./install.sh --legacy-full-install
bash ./uninstall.sh
bash ./uninstall.sh --purge
```

- `install.sh` 默认只会本地构建并安装 `linph` 命令，然后提示下一步运行 `linph install`；它不会安装 tunnel-core、配置、兼容别名、provider state，也不会启动槽位
- `bash ./install.sh --legacy-full-install` 暂时保留旧的完整交互安装路径：会先问 `english/中文`，再问是否安装，并在继续收集端口/地区前先预检 `go`：如果缺失且检测到受支持的包管理器，会询问是否自动安装；之后再进入单端口/多端口、端口与地区，并在执行前预览**最终推导后的每槽端口**
- 交互完整安装会按 VPS 有效内存自动计算默认槽位上限：`floor(totalMiB / 100)`，等价于“50%% 内存 / 50 MiB 每隧道”；会优先使用 cgroup 限制，其次才是宿主机总内存
- `bash ./install.sh --legacy-full-install --fk`、`linph install --fk`、`plinstaller2 --fk` 会忽略这个内存上限，但绝对硬上限仍然是 `28` 个槽位
- `linph install` 默认只写入系统 artifacts 和 Psiphon provider state；需要启动已安装槽位时再运行 `linph start`，或显式传 `linph install --start`
- `linph psi set` 可更新 Psiphon provider 的槽位数、地区和 HTTP/SOCKS 起始端口；`linph provider get` / `linph provider set psi` 用于查看或选择 active provider
- `install.sh` 的 curl-pipe/脚本路径只是便利引导路径，不等同于加密可信安装路径；需要真实性保证时应使用经过签名验证的 release package 或本地审核制品
- 显式传入 install 参数，或在非交互环境调用时，`install.sh` 不会弹出依赖安装提示，而是对缺失的 `go` / `sudo` 直接快速失败并给出手动处理提示
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
- 如果安装阶段最后一步只是**首轮自动启动失败**，文件和 profile 仍可能已经写入；这时先看 `linph log`，再执行 `linph start`

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
- `linph install` / `plinstaller2`：从**本地人工审核制品**安装 `linph`、兼容别名、tunnel-core 和配置，并持久化 Psiphon provider state；默认不启动槽位，槽位数还会受到主机有效内存上限约束
- `linph uninstall` / `pluninstaller`：卸载 `linph` 和兼容别名；默认保留配置，`--purge` 删除整个配置目录

默认安装位置：

- `linph` -> `/usr/local/bin/linph`
- `psiphon` / `plinstaller2` / `pluninstaller` -> `/usr/local/bin/*`
- `archive/psiphon-tunnel-core-x86_64` -> `/etc/psiphon/psiphon-tunnel-core-x86_64`
- `psiphon.config` -> `/etc/psiphon/psiphon.config`

安装槽位上限规则：

- 默认上限 = `floor(totalMiB / 100)`，等价于“取有效内存的 50%%，再按每槽 50 MiB 预算折算”
- “有效内存”优先取 cgroup limit，取不到时再回退到宿主机总内存
- 默认至少允许 `1` 槽，绝对最多允许 `28` 槽
- 需要临时忽略内存上限时，可显式传 `--fk`

安装完成后可直接运行前台兼容入口：

```
linph run
```

也可以先启动再使用多槽 control plane：

```bash
linph start
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

当前主线的运行逻辑不依赖 shell 包装层；`install.sh` 默认只是为了引导安装 `linph`，完整系统安装由 `linph install` 承担。

## 项目状态

Linphon 当前是一个 **Go-only orchestration repo**：

- 入口层已经全部迁到 Go
- `linph` 是新的主命令面
- 测试与 CI 也已经迁到 Go
- 仍然依赖外部 `psiphon-tunnel-core-x86_64` 二进制，但不再依赖 shell 包装层
