# Linphon

Linphon 是一个面向 Linux 的 Psiphon 封装项目。仓库现在已经去掉 shell 包装层，所有入口、测试和多实例 harness 都改成了 Go 实现，同时保留原有的兼容命令名：`psiphon`、`plinstaller2`、`pluninstaller`。

## 项目提供什么

- 一个 repo-local 的单地区管理器：`psiphon-mg`
- 一个 repo-local 的多实例 harness：`psiphon-multi-instance`
- 一个 staged 回归入口：`run-psiphon-staged`
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
go build -o ../../tools/psiphon-mg/bin/psiphon ./cmd/psiphon
go build -o ../../tools/psiphon-mg/bin/pluninstaller ./cmd/pluninstaller
go build -o ../../tools/psiphon-mg/bin/plinstaller2 ./cmd/plinstaller2
```

### 2）启动单地区 manager

```
tools/psiphon-mg/bin/psiphon-mg start US \
  --binary ./archive/psiphon-tunnel-core-x86_64
```

### 3）切换、停止、查看状态

```
tools/psiphon-mg/bin/psiphon-mg switch CA
tools/psiphon-mg/bin/psiphon-mg status
tools/psiphon-mg/bin/psiphon-mg current-region
tools/psiphon-mg/bin/psiphon-mg stop
```

### 4）运行多实例 harness

```
tools/psiphon-mg/bin/psiphon-multi-instance run \
  --binary ./archive/psiphon-tunnel-core-x86_64 \
  --count 3
```

也可以显式绑定地区和端口：

```
tools/psiphon-mg/bin/psiphon-multi-instance run \
  --binary ./archive/psiphon-tunnel-core-x86_64 \
  --instance US:19080:12080 \
  --instance JP:19081:12081
```

`--instance` 模式不能和 `--count`、`--regions`、`--http-port-base`、`--socks-port-base` 混用。

### 5）运行 staged 回归

```
tools/psiphon-mg/bin/run-psiphon-staged \
  --binary ./archive/psiphon-tunnel-core-x86_64
```

### 6）运行测试

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

- `psiphon-mg` 使用 `./psiphon.config` 作为模板
- 运行时状态写入 `./.work/psiphon-mg`
- 多实例 harness 产物写入 `./.work/psiphon-harness`
- staged 产物写入 `./.work/psiphon-harness-staged`

如果二进制不在仓库根目录附近，可以显式指定仓库根：

```
PSIPHON_MG_REPO_ROOT=/path/to/Linphon tools/psiphon-mg/bin/psiphon-mg status
```

也可以直接覆盖关键路径：

```
tools/psiphon-mg/bin/psiphon-mg status \
  --base-config /path/to/psiphon.config \
  --runtime-root /tmp/psiphon-mg
```

### 2）全局兼容入口

`psiphon`、`pluninstaller`、`plinstaller2` 现在也都是 Go 命令。

- `psiphon`：直接执行 `/etc/psiphon/psiphon-tunnel-core-x86_64 -config /etc/psiphon/psiphon.config`
- `pluninstaller`：删除 `/etc/psiphon` 和 `/usr/bin/psiphon`
- `plinstaller2`：保留兼容入口名，但继续以 `exit 66` 失败关闭，因为仓库仍然没有“下载后二进制真实性校验”

如果你需要传统的全局安装方式，请使用**人工审核过的本地制品**：

- `archive/psiphon-tunnel-core-x86_64` -> `/etc/psiphon/psiphon-tunnel-core-x86_64`
- `psiphon.config` -> `/etc/psiphon/psiphon.config`
- 从 `./tools/psiphon-mg/cmd/psiphon` 构建出的 `psiphon` -> `/usr/bin/psiphon`

安装完成后可直接运行：

```
sudo psiphon
```

默认代理端口：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS4/5：`127.0.0.1:1081`

## 目录结构

- `psiphon.config`：基础模板配置
- `archive/`：人工审核本地制品与旧流程说明
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

- `psiphon-mg` 一次只管理一个 active region
- 地区切换采用 stop/start 语义，不做热重载
- `status` 基于进程状态和 notices 派生信号，不自动等价于完整端到端网络可用性证明
- `psiphon-multi-instance run` 的成功标准是“启动宽限期后进程仍存活”，而不是 tunnel 一定已经完全联通

出于安全考虑，下列能力仍然禁用：

- `psiphon-mg --download-if-missing`
- `psiphon-mg --download-url`
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
tools/psiphon-mg/bin/psiphon-mg start US --binary ./archive/psiphon-tunnel-core-x86_64
tools/psiphon-mg/bin/psiphon-mg switch JP
```

如果你使用人工部署的全局流程，则修改 `/etc/psiphon/psiphon.config` 里的：

```
"EgressRegion": "US"
```

### `archive/` 目录还有用吗？

有，但它现在只承担两件事：

- 保存人工审核过的本地制品
- 保存旧流程说明

当前主线已经不再依赖任何 shell 脚本。

## 项目状态

Linphon 当前是一个 **Go-only orchestration repo**：

- 入口层已经全部迁到 Go
- 测试与 CI 也已经迁到 Go
- 仍然依赖外部 `psiphon-tunnel-core-x86_64` 二进制，但不再依赖 shell 包装层
