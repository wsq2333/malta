# 用例管理

用例保存发起测试使用的完整 Plan JSON，包括负载、环境、软件版本、目标目录和资源组。普通数据 I/O、元数据定义和 `scenario/v1alpha1` 混合场景共用一个用例库。保存只校验定义，不执行 I/O；执行时仍按执行器、目录白名单、容量与时间预算检查。独立 mdtest 元数据定义可保存与导出，当前仍走离线结果导入，不支持托管执行。

## 在 UI 中使用

1. 打开侧栏「用例管理」→「新建用例」，填写名称、描述与变更说明。
2. 粘贴完整计划 JSON、导入 `.json` 文件，或从已有测试运行载入计划。
3. 保存生成 v1。列表显示类型、最新版本、创建时间和更新时间，支持按名称、描述、ID 搜索。
4. 查看用例，切换历史版本，查看或导出该版本的 JSON。点击「编辑为新版本」修改后生成 v2、v3……，旧版本保持不变。可以从历史版本内容创建新的最新版本。
5. 点击「使用此版本测试」，核对固定版本、执行器、目标和 Request ID。也可在通用「发起测试」窗口中切换到「选择已保存的用例」。运行详情记录用例 ID 与版本号。

名称、描述、变更说明和计划都属于版本快照。首次创建时间属于用例，更新时间对应最新版本创建时间；时间以 UTC 保存，UI 按浏览器时区显示。

编辑时 `trials` 和 `hash` 是派生字段，由服务重新生成。混合负载的名称与重复数以 `spec.scenario` 为准，外层 `spec.name`、`spec.repeats` 同步重建。保存后显示的是重新校验的规范化计划。更改环境或软件快照内容时仍须更换对应快照 ID，不能覆盖已登记的环境或版本。

## CLI 与 Agent

CLI 默认管理 `--data-dir` 下的本地用例库；加 `--server` 后管理服务端同一用例库。`capabilities` 提供 `case_library` 和 `case_revisions` 能力标志。

先用 `malta plan` 生成 `/tmp/plan.json`，再创建保存请求：

```sh
python3 - <<'PYTHON'
import json
from pathlib import Path
request = {
    "name": "随机读基准",
    "description": "用于指定环境和版本的随机读功能验收",
    "change_note": "首次建立",
    "base_revision": 0,
    "plan": json.loads(Path("/tmp/plan.json").read_text()),
}
Path("/tmp/case.json").write_text(json.dumps(request, ensure_ascii=False, indent=2))
PYTHON
malta --server http://127.0.0.1:8080 case save --file /tmp/case.json
malta --server http://127.0.0.1:8080 case list
malta --server http://127.0.0.1:8080 case history CASE_ID
malta --server http://127.0.0.1:8080 case get CASE_ID 1
```

编辑请求中的 `plan`、描述和变更说明，将 `base_revision` 设为读取到的最新版本号，然后保存：

```sh
malta --server http://127.0.0.1:8080 case save --id CASE_ID --file /tmp/case.json
malta --server http://127.0.0.1:8080 run start \
  --case CASE_ID --case-revision 1 --request-id case-experiment-001 --source agent
```

`--case` 和 `--plan` 互斥，必须指定其中之一。`--case` 必须搭配正整数 `--case-revision`，不隐式使用 latest，避免 Agent 重试时运行定义漂移。同一次实验重试复用 Request ID；换版本或换计划必须使用新的 Request ID。保存版本不合并冲突：如果最新版本已经变化，返回 `conflict`（HTTP 409、CLI 退出码 6），客户端应重新读取并检查变更。

## API

所有端点使用现有 Bearer 认证和 JSON envelope。保存请求最大 4 MiB；未知字段、非法计划、空名称会被拒绝。名称、描述和变更说明上限分别为 200、10000、2000 UTF-8 字节。

| 方法与路径 | 含义 |
| --- | --- |
| `GET /api/v1/cases` | 按更新时间倒序返回用例及最新快照 |
| `POST /api/v1/cases` | 创建用例与 v1，`base_revision=0` |
| `GET /api/v1/cases/{id}/versions` | 按版本号倒序返回历史快照 |
| `POST /api/v1/cases/{id}/versions` | 要求 `base_revision` 匹配最新版本，追加新版本 |
| `GET /api/v1/cases/{id}/versions/{revision}` | 获取固定版本；不存在返回 404 |
| `POST /api/v1/runs` | 接受原有 `plan` 或新的 `case_ref`，二选一 |

按版本提交测试的请求：

```json
{
  "case_ref": {"id": "case-example", "revision": 1},
  "request_id": "case-experiment-001",
  "source": "agent",
  "executor": "local"
}
```

服务读取固定版本的计划，将完整计划与 `case_ref` 一起保存到 Run。结果 Bundle 也携带该引用，导出到其他实例时仍能追溯原用例身份；导入 Bundle 不会自动创建原用例库。相同 Request ID 引用不同用例版本，即使计划哈希相同，也返回 409。

## 存储

SQLite schema 升至 v2，新增两张表：

- `test_cases`：`id`、`created_at`、`latest_revision`，保存稳定身份和当前版本指针。
- `case_versions`：`case_id`、`revision`、`body`，联合主键；`case_id` 外键指向 `test_cases.id`，body 保存完整不可变版本。

`test_cases` 与 `case_versions` 是一对多关系。追加版本与更新最新指针在同一事务内完成，通过 `base_revision` 检测过期写入。Run 内的 `case_ref` 是可移植的逻辑引用，不设置 SQL 外键，便于导入外部结果。

打开旧数据库会自动建表并迁移，不改动原来的 `objects`、`runs` 和结果文件。库及所有版本随服务 `--data-dir` 持久化，Server Pod 应使用原有元数据卷。当前不提供删除历史版本、草稿、自动合并、标签或用户级权限；访问权限沿用服务 Token。JSON 用例用于实验参数，不用于保存 kubeconfig、LLM Key 等凭证。

## 验收

Linux 开发机可执行以下脚本；使用独立临时目录及本地执行器，无 Kubernetes 依赖：

```sh
python3 scripts/case-smoke.py --malta /absolute/path/malta \
  --fio /absolute/path/fio --root /tmp
```

脚本覆盖创建、编辑重算哈希、旧版本不变、冲突拒绝、非法请求、固定版本发起、幂等重试、fio 与原生混合负载真实 I/O、Bundle 来源记录、服务重启恢复。数据及验收摘要保留在输出的目录，便于复查。
