# Kubernetes 部署

部署清单位于 `deploy/k8s/`，共三个 YAML：

- `deployment.yaml`：单副本 PicFast，所有环境变量直接放在 `containers[].env`。
- `service.yaml`：ClusterIP Service，端口 `8080`。
- `pvc.yaml`：10 GiB、ReadWriteOnce 的数据卷。

## 准备与部署

1. 准备可访问的 PostgreSQL 数据库和有权限执行迁移的数据库账号。应用启动时自动迁移；这些 YAML 不安装 PostgreSQL。
2. 修改 `deployment.yaml` 中的数据库连接、JWT 密钥和站点访问地址。数据库连接的 `sslmode` 按数据库要求设置。管理员邮箱、密码同时留空时使用首次访问向导；同时填写可自动初始化管理员。
3. 修改 `pvc.yaml` 的容量。默认使用集群的默认 StorageClass；没有默认 StorageClass 时，指定 `storageClassName` 或绑定已有 PV。PVC、Deployment、Service 必须在同一 namespace。
4. 使用包含本 fork 代码的镜像。默认 `ghcr.io/bilbilmyc/picfast:dev-main` 为开发镜像；推荐部署时固定 `dev-<commit>` 标签或镜像 digest。镜像发布与访问方式见下文。

```bash
kubectl apply -f deploy/k8s/
kubectl rollout status deployment/picfast
kubectl get pods,svc,pvc -l app=picfast
kubectl port-forward svc/picfast 8080:8080
```

打开 `http://localhost:8080` 完成初始化。域名与 HTTPS 可接入已有 Ingress 或反向代理；将 `PICFAST_SERVER_BASE_URL` 改为最终对外地址。代理头和可信代理配置见 [反向代理部署](deploy.md)。这套模板使用 ClusterIP，不创建 Ingress。

## 存储与运行方式

`/app/data` 挂载到 PVC，其中 `uploads` 为本地存储的原图、`thumbnails` 为缩略图、`backups` 为本地备份目录。使用 TOS 策略后，新上传的原图写入 TOS，缩略图和本地备份仍需持久化；PostgreSQL 保存图片及策略等元数据。配置方法见 [火山引擎 TOS](tos.md)。

容器使用 UID/GID `10001`，Pod 通过 `fsGroup: 10001` 赋予卷访问权限。NFS 等不能自动设置组权限的存储，应提前在存储端设置可写权限。单副本配合 `Recreate` 避免升级时新旧 Pod 同时挂载 RWO 卷。

`/health` 用于启动和就绪探针，检查数据库及本地目录；存活探针使用 TCP。TOS 的连接健康检查在管理后台“维护”中查看。Prometheus 可从 Pod 的 `9190/metrics` 抓取，Service 仅暴露业务端口。PVC、本地备份和 PostgreSQL 均需要独立备份安排。

## Fork 镜像

本 fork 的 `Fork Development Image` 工作流在 `main` 更新或手动触发时构建 `linux/amd64`、`linux/arm64` 镜像，发布至仓库对应的 GHCR：

- `ghcr.io/bilbilmyc/picfast:dev-main`：滚动开发标签。
- `ghcr.io/bilbilmyc/picfast:dev-<完整 commit SHA>`：对应提交的开发标签。

在 GitHub Actions 中确认镜像工作流完成后再部署。[GHCR 新包默认是私有](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)；公开拉取需要在 GitHub Package 设置中将包设为 Public，私有包则需集群配置镜像拉取凭据。上游 `xbeta/picfast:v0.24.1` 不包含此 fork 的 TOS 支持。

使用私有包时，将有 `read:packages` 权限的 GitHub token 放入本地环境变量 `GHCR_TOKEN`，在应用所在 namespace 创建拉取凭据，然后取消 Deployment 中 `imagePullSecrets` 的注释。该凭据通过命令创建，不增加 YAML 文件；应用环境变量仍全部保留在 Deployment 中。

```bash
kubectl create secret docker-registry picfast-ghcr \
  --docker-server=ghcr.io \
  --docker-username=bilbilmyc \
  --docker-password="$GHCR_TOKEN"
```

也可以用现有 Dockerfile 构建到自己的镜像仓库，再替换 Deployment 的 `image`：

```bash
docker build -f docker/Dockerfile -t YOUR_REGISTRY/picfast:dev-tos .
docker push YOUR_REGISTRY/picfast:dev-tos
```
