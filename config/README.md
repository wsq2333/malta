# 本地配置

此目录保存环境相关连接、应用参数和未来 LLM 凭证。Git 默认忽略这里的所有内容，仅允许提交本说明和下列四个指定示例文件。真实配置、证书、密钥、备份和新增子目录都不提交；不要把真实值填入示例，也不要用 `git add -f` 绕过规则。

| 路径 | 用途 | 当前支持情况 |
| --- | --- | --- |
| `deploy/kubeconfig.example.yaml` | 部署与测试集群的地址、CA、用户和 context | kubectl 标准 kubeconfig |
| `parameter/executor.example.yaml` | 测试节点、PVC、Runner 镜像与资源预算 | `serve --kubernetes-config` 可加载 YAML 或 JSON |
| `parameter/application.example.yaml` | 服务与本地 Runner 参数 | 预留模板，当前需手动传 CLI flag |
| `llm/providers.example.yaml` | 供应商、模型、API 地址与密钥来源 | 预留模板，当前未接入 LLM 加载器 |

## 创建本地副本

从仓库根目录执行；已有本地配置时不要覆盖：

```sh
cp -n config/deploy/kubeconfig.example.yaml config/deploy/kubeconfig.yaml
cp -n config/parameter/executor.example.yaml config/parameter/executor.yaml
cp -n config/parameter/application.example.yaml config/parameter/application.yaml
cp -n config/llm/providers.example.yaml config/llm/providers.yaml
chmod 600 config/deploy/kubeconfig.yaml config/parameter/executor.yaml \
  config/parameter/application.yaml config/llm/providers.yaml
```

修改本地副本中的占位值。所有 `*.example.yaml` 均为模板，不能直接连接真实集群或模型。CA 文件路径相对于 kubeconfig 所在目录解析，可将实际证书放入 `config/deploy/`。保留 TLS 证书校验。

## Kubernetes 连接与运行参数

```sh
export KUBECONFIG="$PWD/config/deploy/kubeconfig.yaml"
# 只查看本地 context，不连接集群。
kubectl config get-contexts

# 填妥 executor.yaml，并让服务可访问配置中的结果 PVC 路径后启动。
bin/malta serve --listen 127.0.0.1:8080 --data-dir .malta \
  --kubernetes-config config/parameter/executor.yaml
```

部署时通过 `kubectl --context malta-deployment ...` 显式选择部署连接；执行器通过 `executor.yaml` 的 `context` 选择测试连接。二者可以使用同一集群。当前标准部署是 **Server 与 Runner 在同一集群的独立 Pod**，共用结果 PVC；此示例的两个 context 不表示已支持跨集群结果传输。

Service Pod 内优先使用 ServiceAccount 和现有 RBAC，执行器 `context` 留空；不需要把管理员 kubeconfig 打包进镜像。`config/deploy/` 保存本地连接信息，[`deploy/kubernetes/`](../deploy/kubernetes/README.md) 继续保存 Deployment、Job、RBAC 等可版本化部署清单。

`application.yaml` 不会自动应用，也不会展开环境变量。当前认证 Token 使用 `MALTA_TOKEN`；LLM 模板中的 `api_key_env` 仅定义未来接入意图，不会被当前运行时读取。

## 检查忽略规则

```sh
git check-ignore config/deploy/kubeconfig.yaml \
  config/parameter/application.yaml config/parameter/executor.yaml \
  config/llm/providers.yaml
git status --short --untracked-files=all -- config
```

第一条应列出全部四个真实配置路径；第二条只能出现指定示例与说明的修改。新增可提交示例时，需要同步更新根目录 `.gitignore` 的白名单并检查内容。
