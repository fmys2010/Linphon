# Linphon Archive

`archive/` 现在是 Linphon 的人工审核本地制品目录，不再包含 shell 入口。

它的用途主要有两类：

- 保存手工审核过的 `psiphon-tunnel-core-x86_64`
- 为不走 repo-local manager 的场景提供最小手动运行说明

> 注意：`archive/` 不是当前主推荐路径。仓库的主线入口已经迁到 `linph` 和 `tools/psiphon-mg/cmd/*` 下的 Go 命令；自动远程下载安装仍然禁用，直到仓库具备“下载后二进制真实性校验”。

## 推荐安装方式

在仓库根目录执行：

```
bash ./install.sh
```

这会基于本地人工审核制品安装：

- `/usr/local/bin/linph`
- `/usr/local/bin/psiphon`
- `/usr/local/bin/plinstaller2`
- `/usr/local/bin/pluninstaller`
- `/etc/psiphon/psiphon-tunnel-core-x86_64`
- `/etc/psiphon/psiphon.config`

卸载：

```
bash ./uninstall.sh
bash ./uninstall.sh --purge
```

- 默认卸载保留 `/etc/psiphon/psiphon.config`
- `--purge` 删除整个 `/etc/psiphon`

## 最小手动运行

```
git clone https://github.com/fmys2010/Linphon.git
cd Linphon/archive
chmod +x psiphon-tunnel-core-x86_64
./psiphon-tunnel-core-x86_64 -config ../psiphon.config
```

## 如果你需要兼容入口名

请从这些 Go 命令构建：

- `tools/psiphon-mg/cmd/linph`
- `tools/psiphon-mg/cmd/psiphon`
- `tools/psiphon-mg/cmd/pluninstaller`
- `tools/psiphon-mg/cmd/plinstaller2`

其中：

- `linph run` / `psiphon` 会执行 `/etc/psiphon/psiphon-tunnel-core-x86_64 -config /etc/psiphon/psiphon.config`
- `linph install` / `plinstaller2` 会从本地人工审核制品执行安装
- `linph uninstall` / `pluninstaller` 会卸载 `linph` 与兼容别名；默认保留配置，`--purge` 可删除整个配置目录

## 代理端口

默认仍然是：

- HTTP / HTTPS：`127.0.0.1:8081`
- SOCKS4/5：`127.0.0.1:1081`

## 说明

- 当前正式文档入口是仓库根目录的 `README.md`
- `archive/` 保留的是制品和说明，不再保留旧 shell 兼容脚本
