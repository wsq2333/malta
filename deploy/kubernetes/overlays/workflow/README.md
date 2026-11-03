# Argo 工作流入口

`endpoints.example.yaml` 提供默认关闭的 Argo Server 连接。复制到被 Git 忽略的 `config/deploy/`，填写实际 URL、cluster 和 namespace，验收权限后启用。公开版本只接受显式 `backend: argo`，不安装 Argo 控制器。

在 Malta Server 所在 namespace 创建 `malta-argo-auth` Secret，键为 `token`。真实凭据不写入 ConfigMap、模板或仓库。将配置和 Secret 挂到 Server：

```yaml
volumes:
  - name: workflow-config
    configMap:
      name: malta-workflow-endpoints
  - name: argo-auth
    secret:
      secretName: malta-argo-auth
      defaultMode: 0440
volumeMounts:
  - name: workflow-config
    mountPath: /etc/malta-workflows
    readOnly: true
  - name: argo-auth
    mountPath: /var/run/secrets/malta-argo
    readOnly: true
```

启动参数添加 `--workflow-config /etc/malta-workflows/workflow.yaml`。Secret 文件权限需允许 Malta 运行身份读取。`--management-only` 与工作流执行互斥。

默认使用 `bearer_token_file`，每次请求读取文件以支持外部轮换。通用认证层也保留 `oauth2_password` 与 `oauth2_client_credentials`：仅当实际 Argo Server 或其网关接受相应 Token 时配置，账号、密码和 client secret 均从 Secret 挂载文件读取。OAuth Token 在内存缓存并自动刷新，不写入运行快照。

配置结果解析时，Server 的 `local_results_root` 和任务的 `remote_results_root` 必须映射同一共享目录，不能用 SQLite 状态卷替代。没有 results 规则的原生模板只记录执行状态，不要求共享结果卷。

上线前独立验证 Argo API 权限、namespace、执行节点、PVC、Secret、镜像和证据目录。连接配置就绪不代表现场已验收。配置变更后重新预检查，不复用旧预检查快照。
