# Desktop2Stereo 项目更新功能

New API“运维 → 系统维护”中的更新功能只更新本项目：

```text
https://github.com/laiyangli001/desktop2stereo-site
```

不会检查或下载 `Calcium-Ion/new-api` 上游版本。

## 安全边界

- 更新仓库、分支和执行脚本不由浏览器传入，仓库固定为 Desktop2Stereo 项目。
- 页面只能提交 GitHub 返回的 40 位 commit SHA，服务端会再次向 GitHub 校验 SHA 是否仍为 `main` 最新提交。
- Docker 部署通过固定的 `/opt/desktop2stereo-site/update-requests/request` 请求文件触发更新；宿主机
  `desktop2stereo-update-watcher` 只读取并删除该文件，再调用固定的
  `/usr/local/sbin/desktop2stereo-docker-update`，不接受浏览器传入的 shell 命令。
- `/usr/local/sbin/desktop2stereo-update` 仅适用于旧的非 Docker 部署模式，不是当前 Docker 生产模式的更新器。
- 代码发布目录和运行数据目录分离。
- 发布归档使用目标 commit 自带的 `Dockerfile`；不会被服务器工作目录中的旧 Dockerfile 覆盖。
- 成功健康检查后，固定更新器会同步安装该 commit 自带的
  `deploy/desktop2stereo-docker-update.sh`，后续发布不会继续使用旧更新器。
- `.env`、数据库、上传文件、日志和备份不会被 Git checkout 覆盖。
- 数据库备份脚本不存在或不可执行时，更新会在构建前停止。
- 构建失败、服务启动失败或健康检查失败时回滚到旧版本。

## 服务器初始化

建议使用以下目录：

```text
/opt/desktop2stereo/
├── current -> releases/<commit-sha>
├── releases/
└── shared/
    ├── .env
    ├── uploads/
    ├── logs/
    └── backups/
```

非 Docker 部署模式才安装 `deploy/desktop2stereo-update.sh`：

```bash
install -o root -g root -m 0750 deploy/desktop2stereo-update.sh \
  /usr/local/sbin/desktop2stereo-update
```

另外安装 `/usr/local/sbin/desktop2stereo-db-backup`。Docker 部署可以直接使用仓库中的 `deploy/desktop2stereo-db-backup-docker.sh`，该脚本先完成 PostgreSQL 备份，再返回成功；失败时必须返回非零退出码。备份脚本不应放在 GitHub 工作目录中。

Docker 部署不把 Docker socket 暴露给应用容器，而是使用宿主机 systemd path watcher。生产 Docker 模式安装：

```bash
install -o root -g root -m 0750 deploy/desktop2stereo-db-backup-docker.sh /usr/local/sbin/desktop2stereo-db-backup
install -o root -g root -m 0750 deploy/desktop2stereo-docker-update.sh /usr/local/sbin/desktop2stereo-docker-update
install -o root -g root -m 0750 deploy/desktop2stereo-update-watcher.sh /usr/local/sbin/desktop2stereo-update-watcher
install -o root -g root -m 0644 deploy/desktop2stereo-update.service /etc/systemd/system/desktop2stereo-update.service
install -o root -g root -m 0644 deploy/desktop2stereo-update.path /etc/systemd/system/desktop2stereo-update.path
systemctl daemon-reload
systemctl enable --now desktop2stereo-update.path
```

宝塔 Go 项目应指向：

```text
运行目录：/opt/desktop2stereo/current
启动文件：/opt/desktop2stereo/current/desktop2stereo-site
环境文件：/opt/desktop2stereo/shared/.env
```

如果宝塔项目由 Supervisor 管理，设置：

```text
D2S_UPDATE_RESTART_MODE=supervisor
D2S_UPDATE_SERVICE=<宝塔项目对应的 Supervisor 名称>
```

如果使用 systemd，则设置：

```text
D2S_UPDATE_RESTART_MODE=systemd
D2S_UPDATE_SERVICE=desktop2stereo
```

更新功能默认关闭。确认固定脚本、数据库备份脚本、运行目录和重启方式都完成后，在 `.env` 中设置：

```text
D2S_UPDATE_ENABLED=true
D2S_UPDATE_REQUEST_FILE=/opt/desktop2stereo-site/update-requests/request
D2S_UPDATE_STATUS_FILE=/opt/desktop2stereo-site/update-requests/status.json
D2S_DOCKER_APP_ROOT=/opt/desktop2stereo-site
D2S_DOCKER_RELEASE_ROOT=/opt/desktop2stereo-releases
D2S_DOCKER_UPDATE_SCRIPT=/usr/local/sbin/desktop2stereo-docker-update
D2S_UPDATE_BACKUP_SCRIPT=/usr/local/sbin/desktop2stereo-db-backup
```

非 Docker 模式使用 `D2S_UPDATE_SCRIPT`、`D2S_UPDATE_ROOT` 和
`D2S_UPDATE_HEALTH_URL`；不要把这些旧模式变量与 Docker 请求文件模式混用。

## 页面操作流程

1. 点击“检查项目更新”。服务器从固定项目的 `main` 分支读取最新 commit。
2. 核对 commit SHA 和提交说明。
3. 点击“更新服务器到此提交”。
4. 服务端启动固定更新脚本。
5. 脚本先备份数据库，再下载指定 commit。
6. 在独立 release 目录编译前端和 Go 程序。
   构建时将目标完整 commit SHA 注入前端版本、Go `common.Version` 和
   `org.opencontainers.image.revision`，并额外保留 `new-api-desktop2stereo-site:<commit-sha>` 镜像标签。
   服务器更新器默认使用腾讯云可达的 `https://goproxy.cn,direct`；由于生产服务器可能无法访问
   `sum.golang.org`，默认使用仓库已有 `go.sum` 进行依赖校验并设置 `GOSUMDB=off`。如服务器已配置
   可达的校验数据库，可通过固定服务环境变量 `D2S_BUILD_GOPROXY` 和 `D2S_BUILD_GOSUMDB` 覆盖，
   不要把代理或校验配置写入仓库 Secret。
   Docker 构建默认有 900 秒固定超时，可通过服务环境变量 `D2S_BUILD_TIMEOUT_SECONDS` 调整；
   超时会进入失败状态并保留旧容器，不会继续等待或切换未完成镜像。
   只有健康检查通过后，发布目录才会写入 `.update-complete` 标记；中途失败或被终止留下的
   不完整目录会在同一 SHA 重试时清理并重新构建，不会被误判为“已部署”。
7. 原子切换 `current` 软链接。
8. 重启服务并请求健康接口。
9. 健康检查失败时恢复旧软链接并重启旧版本。

禁止在服务器更新时执行以下操作：

```bash
git clean -fdx
rm -rf /opt/desktop2stereo/shared
rsync --delete ./ /opt/desktop2stereo/shared/
```
