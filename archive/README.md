# Linphon Archive

`archive/` 现在是 Linphon 的人工审核本地制品目录，不再包含 shell 入口。

它的用途主要有两类：

- 保存手工审核过的 `psiphon-tunnel-core-x86_64`
- 为不走 repo-local manager 的场景提供最小手动运行说明

> 注意：`archive/` 不是当前主推荐路径。仓库的主线入口已经迁到 `tools/psiphon-mg/cmd/*` 下的 Go 命令；自动远程下载安装仍然禁用，直到仓库具备“下载后二进制真实性校验”。

## 最小手动运行

```
git clone https://github.com/fmys2010/Linphon.git
cd Linphon/archive
chmod +x psiphon-tunnel-core-x86_64
./psiphon-tunnel-core-x86_64 -config ../psiphon.config
```

## 如果你需要兼容入口名

请从这些 Go 命令构建：

- `tools/psiphon-mg/cmd/psiphon`
- `tools/psiphon-mg/cmd/pluninstaller`
- `tools/psiphon-mg/cmd/plinstaller2`

其中：

- `psiphon` 会执行 `/etc/psiphon/psiphon-tunnel-core-x86_64 -config /etc/psiphon/psiphon.config`
- `pluninstaller` 会删除 `/etc/psiphon` 和 `/usr/bin/psiphon`
- `plinstaller2` 会继续以 `exit 66` 失败关闭，提示改用人工审核本地制品流程

## 代理端口

默认仍然是：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS4/5：`127.0.0.1:1081`

## 说明

- 当前正式文档入口是仓库根目录的 `README.md`
- `archive/` 保留的是制品和说明，不再保留旧 shell 兼容脚本
