# 多后端入口与认证

`endpoints.example.yaml` 是当前运行时支持的 ConfigMap：`data.workflow.yaml` 使用统一的 `connections`，每项明确 `id`、`backend`、`enabled`、URL、认证及执行范围。默认关闭 UCE/Argo，尚未创建 Secret 时仍可启动、展示入口和管理模板。

之前部署的 `data.endpoints.yaml` 只是入口目录，**不能直接作为运行配置加载**。升级到本实现后，按本例增加 `workflow.yaml` 并挂载 `--workflow-config /etc/malta-workflows/workflow.yaml`；旧运行镜像不支持新的认证字段和 Argo 后端。

配置中的 URL、client_id、执行 namespace 与路径均为占位值。真实配置只放集群或被 Git 忽略的 `config/` 副本。

## Secret 挂载

`malta-uce-auth` 保存 `username` / `password`，可选 `client-secret`；`malta-argo-auth` 保存 `token`。均在 Malta Server 所在 namespace 创建，**不要将真实值写进 ConfigMap、仓库或用例**。按已有 Secret 管理方式注入用户提供的凭据。

在 Deployment 的现有配置上合并以下片段，保留旧 executor 配置、状态盘和结果卷：

```yaml
# spec.template.spec
volumes:
  - name: workflow-config
    configMap:
      name: malta-workflow-endpoints
  - name: uce-auth
    secret:
      secretName: malta-uce-auth
      optional: true
      defaultMode: 0440
  - name: argo-auth
    secret:
      secretName: malta-argo-auth
      optional: true
      defaultMode: 0440
# server container.volumeMounts
volumeMounts:
  - name: workflow-config
    mountPath: /etc/malta-workflows
    readOnly: true
  - name: uce-auth
    mountPath: /var/run/secrets/malta-uce
    readOnly: true
  - name: argo-auth
    mountPath: /var/run/secrets/malta-argo
    readOnly: true
```

Server 参数追加 `--workflow-config /etc/malta-workflows/workflow.yaml`，不能同时使用 `--management-only`。通过 Pod 的 `fsGroup` 让服务 UID 能读取 0440 凭据文件；沿用现有非 root 安全配置。不要使用 `subPath` 挂载 Secret，否则轮换不会更新文件。

修改连接配置需要滚动重启；凭据文件每次取 Token 时重新读取，变更后清除缓存重新认证。Token、Refresh Token 只保存在内存，重启后重新获取。暂缺凭据时入口显示“待配置 Secret”，提交按钮不可用。`configured` 只代表文件就绪，预检查会实际取得 OAuth Token，业务权限由原生接口判定。

## 认证模式

- `bearer_token_file`：每次 HTTP 请求读取 Token 文件；用于既有 UCE Token 或 Argo ServiceAccount Token。外部签发与轮换由提供方负责。
- `oauth2_password`：从 Secret 读取账号密码，使用 `grant_type=password`；优先刷新 Refresh Token，失效时重新登录。兼容已有 Keycloak Direct Access Grants。
- `oauth2_client_credentials`：推荐服务间长期运行，配置 `token_url`、`client_id`、`client_secret_file`；到期前重新获取 Token。若返回 Refresh Token，也支持使用它。

OAuth Token endpoint 强制 HTTPS（仅单元测试 loopback 可用 HTTP），禁止跳转，不回显认证响应正文。按 `expires_in` 提前续期，同一入口并发请求共用刷新；失败有短暂退避。业务 API 明确返回 401 时仅刷新并重试一次，保持幂等键和请求体；网络中断不会直接盲重试提交。

新 client 必须被 UCE/Argo 信任并有相应权限。UCE 当前检查 issuer/audience/azp；不能只取得 Token 就认为业务接口可调用。Argo Server 需要正确的 client/SSO 认证模式与 namespace RBAC。

## 启用前核验

1. 验证业务环境、API TLS 与凭据权限，确认执行 cluster / namespace / subtype 等真实路由。
2. Native-only 用例不要求 Malta 挂结果卷；需要 fio/JUnit 等解析时，核验模板生产者与 Server 访问的是同一 NAS 目录，并设置两个结果根路径。
3. 创建 Secret，设置相应连接 `enabled: true`，滚动服务后检查 `/api/v1/workflow/connections` 和 capabilities。
4. 原生模板的卷、Secret、镜像、调度约束和运行身份由作者与后端负责。Malta 不再把模板重写成自己的 DAG，也不凭预检查宣称节点/PVC 已现场验收。
5. Argo 适配器通过 Argo Server REST API 创建/读取 WorkflowTemplate 与 Workflow，并终止 Workflow。没有部署 Argo 控制器、没有自动创建执行 namespace 或授予 RBAC。

旧任务绑定连接配置指纹；修改 URL、backend 或路由会暂停其恢复，不能将旧任务悄悄切到新后端。
