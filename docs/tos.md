# 火山引擎 TOS

PicFast 提供独立的 **火山引擎 TOS** 存储类型（API 标识为 `tos`），通过 TOS 的 S3 兼容接口访问对象。客户端使用 SigV4 签名与 Virtual-hosted 寻址；原有 `s3` 策略继续使用 Path-style，兼容现有 MinIO 等部署。

## 后台配置

1. 在火山引擎创建 TOS 扁平命名空间存储桶，并记录实际地域。
2. 在 PicFast 管理后台进入“存储策略”，新建策略并选择“火山引擎 TOS”。
3. 填写以下参数：

| 字段 | 示例 / 说明 |
| --- | --- |
| Endpoint | `https://tos-s3-cn-beijing.volces.com`，填写控制台中的 **S3 访问点**，不包含 Bucket，不使用 `tos-cn-beijing.volces.com` 原生 TOS 端点 |
| Region | `cn-beijing`，必须与存储桶地域一致，不能填 `auto` |
| Bucket | `your-bucket`，已存在的存储桶名称 |
| Access Key / Secret Key | 具有目标桶访问权限的火山引擎 AK/SK |
| 访问 URL | 可填公开 Bucket 域名、CDN 或自定义域名；留空时自动使用 `https://<bucket>.tos-s3-<region>.volces.com` |
| 通过 PicFast 代理访问 | 私有桶或需要 PicFast 图片权限控制时开启；服务端通过已配置的凭据读取对象 |

4. 在分组管理中绑定该策略，并按需设置为分组默认策略。已配置个人默认策略的用户还需选择 TOS；上传接口也可指定 `strategy_id`。
5. 在维护页面检查桶连接，验证上传、浏览及删除。AK/SK 至少需要目标桶的 `tos:HeadBucket` 以及对象级 `tos:PutObject`、`tos:GetObject`、`tos:DeleteObject` 权限。

公共直链模式需要桶或 CDN 允许公开读取。私有桶选择 PicFast 代理模式后，访问 URL 可以留空。策略配置保存到 PostgreSQL；Deployment 的应用环境变量不会自动创建或启用存储策略。

## API 配置示例

管理员创建存储策略接口的请求体示例（使用自己的凭据替换占位符）：

```json
{
  "name": "火山引擎 TOS",
  "strategy_type": "tos",
  "configs": {
    "endpoint": "https://tos-s3-cn-beijing.volces.com",
    "region": "cn-beijing",
    "bucket": "your-bucket",
    "access_key": "YOUR_ACCESS_KEY",
    "secret_key": "YOUR_SECRET_KEY",
    "url": "",
    "link_mode": "proxy"
  }
}
```

切换默认策略只影响后续上传，已有图片继续使用其原策略，不自动迁移。TOS 保存选中该策略的原图，缩略图仍使用 PicFast 的本地目录，K8s 部署仍需数据 PVC。

## 兼容性与验证

实现覆盖 `PutObject`、`GetObject`、`DeleteObject`、`HeadBucket`。为了兼容 TOS 的 S3 接口，SDK 的可选请求校验和计算、响应校验设为 `WHEN_REQUIRED`。参考 [TOS 官方 S3 SDK 使用说明](https://docs.volcengine.com/docs/TorchObjectStorage/AccessingTOSUsingAWSS3SDK?lang=zh)、[TOS 存储桶命名规则](https://docs.volcengine.com/docs/TorchObjectStorage/Commonconcepts-2?lang=zh) 和 [AWS SDK 校验和配置](https://docs.aws.amazon.com/sdkref/latest/guide/feature-dataintegrity.html)。

自动化测试使用本地 HTTP 服务检查实际 SDK 请求的 Bucket 主机名、SigV4 地域及服务、路径、上传内容、读取、删除、健康检查和错误返回，并验证原有 S3 的 Path-style 行为。真实 TOS 验收仍需使用自己的存储桶与凭据完成上述操作。
