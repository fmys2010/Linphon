# Linphon Archive

这是 Linphon 保留下来的旧版手动安装/兼容目录，主要用于：

- 参考旧的目录式安装方法
- 保留旧脚本命名和历史流程
- 在不使用 repo-local manager 的情况下手动运行 Psiphon

> 注意：`archive/` 不是当前主推荐路径。根目录 `plinstaller2` 的自动远程下载安装也已经禁用；如果不使用 repo-local manager，请优先采用这里的人工审核本地制品流程。

## 手动安装（旧流程）

```bash
git clone https://github.com/fmys2010/Linphon.git
cd Linphon/archive
sudo chmod +x psiphon-tunnel-core-x86_64
sudo chmod +x psiphon.sh
```

然后直接运行：

```bash
./psiphon-tunnel-core-x86_64 -config ../psiphon.config
```

## 旧版自动安装脚本（已禁用）

`archive/plinstaller.sh` 这个旧入口仍然保留，但现在会失败关闭（exit 66），直到仓库具备“下载后二进制真实性校验”为止。

## 代理端口

默认仍然是：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS4/5：`127.0.0.1:1081`

## 说明

- `archive/plinstaller.sh` 与 `archive/psiphon.sh` 保留只是为了兼容旧流程；其中自动下载安装行为当前已禁用
- 当前主 README 才是 Linphon 的正式文档入口
