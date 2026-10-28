# Sample dev：真实 Curvine 执行接入验收

日期：2026-10-28。**部署与任务调度/回收已完成，真实 I/O 验收未通过。**
运行失败于 fio 准备阶段，没有生成性能指标或基线。

## 已落地的配置

- Server 保持提交 `f3db77c72675a49d703baae4faeadded4b16058e`，不混入后续 UI 改动。
- Runner 使用同一已校验二进制，Malta 版本 `0.1.0-dev.f3db77c`，fio `3.33`。
  镜像标签 `runner-dev-f3db77c`，已推送并核验 digest
  `sha256:1ad4d42446d7da4e83ed43ee2cd13e9203cc17e6f12a874f716d408c8ece18be`。
  开发机无 Docker daemon，按 `build/docker/Dockerfile.runner` 的固定 Debian
  digest、`fio=3.33-3`、COPY/USER/ENTRYPOINT 配方生成 OCI 镜像；开发机 chroot
  验证 UID 10001、版本和 4 MiB fio I/O。构建脚本、镜像及记录保存在开发机。
- `malta-dev` 新建专用静态 Curvine PV/PVC（10 GiB 声明容量，Retain），
  Runner 挂载 `/bench`。另建 NAS RWX PVC（20 GiB、独立动态子目录）挂载
  Server/Runner 的 `/results`。SQLite 继续使用原有独立 RWO 状态盘。
- 客户端固定在单个 `ecs.g8i.2xlarge` 节点，与 Curvine 服务端分开；
  Runner requests=limits 为 1 vCPU/1 GiB，UID/GID `10001:10001`。
  节点与其他 dev 服务共享，不属于独占性能测试环境。
- 实际 FUSE 是 CSI 创建的独立 Pod，release `0.2.2`，镜像标签 `5fcc1b02`，
  digest `sha256:c9345ec51bfa9436fdcd6ea72a54d7f0b34a2beec0b45909d3862ed74fa1de7c`。
  `--version-json` 的 Git commit 为 `unknown`，不将镜像标签冒充已验证的 Git commit。
  FUSE requests 为 500m/512Mi，limits 为 4 CPU/8Gi，单独记录于环境快照。
- Server 启用 Kubernetes executor，保留原有内网 Service/IP、单副本 Recreate
  和用户明确选择的无 API Token 访问方式。执行预算为 1 GiB/10 分钟、串行任务。
  Server 具有 namespace 内 Job/ConfigMap 管理、Pod 日志和指定 PVC 读取权限，
  集群级仅允许读取指定 Node。Runner 无管理 Role 且不挂载 ServiceAccount Token。
  验证 Server 不能读取 Secret、Runner 不能创建 Job 或读取 ConfigMap。

实际节点、卷路径、部署与 executor 参数仅保存于忽略的 `config/` 和开发机，
不把真实配置或仓库密码写入 Git。部署时应以执行 overlay 为准；旧的仅管理
服务配置不能覆盖当前 executor。保持原有 Service，避免通用模板改变内网入口。

## 验证与失败记录

1. 新卷 Bound；以 UID/GID 10001 验证 mkdir、排他创建、读写和 rename，
   NAS 在客户端/Server 两个节点间验证共享可见性和排他创建。
2. 初次挂载发现 CSI 忽略 RWX fsGroup，初始化本次新建目录的属主。
   Curvine 还会检查完整路径：新建父目录也必须允许测试 UID 遍历。
   仅修改 Malta 专用目录，没有关闭 FUSE 权限校验或修改其他业务目录。
3. 开发机和 Server Pod 内的 `malta kubernetes check` 均通过。
4. 新环境快照：`sample-dev-curvine-client-20261028-r2`。
   用例 `case-e6bb9444ef4f1ff919e8a94e` 保存为 **v2**，v1 保持不变。
   计划 hash：`a20eb9a8f6c57e34aab82600dcf8080e813443ee3e71621b68b66c43a8094bdc`。
5. 提交 `run-01c3d66cb95c406db6d252a7`，Request ID
   `sample-dev-curvine-4mib-v2-first-20261028`。固定用例 v2、Kubernetes executor。
   计划为 4 MiB 文件、128 KiB 块、单并发顺序读、warm/direct=false，
   10s 测量 + 2s warmup、3 次独立重复。
6. Runner 成功拉取指定 digest，完成共享目录握手。首轮准备阶段 fio 返回
   `errno 13 (EACCES)`，最终状态 `failed`、`result_state=complete`。
   Job/Pod 按正常流程回收，原始命令、fio 输出及运行证据已持久化。

## 未通过原因与后续条件

在同一 Runner 镜像和 UID 10001 下复现：目录属主正确，但新建普通文件
被 Curvine 记录为 **root:root**。创建时持有的文件描述符可以写入；关闭后
fio 再次以读写模式打开文件时被拒绝。这说明仅验证新文件首次写入不足以
验收文件系统；还需检查持久化属主和关闭后重开。

在独立诊断目录中，仅把该诊断文件 chown 为 `10001:10001` 后，同一条
fio 命令成功写入 `4194304` 字节，job error 为 0。没有修改正式失败 Run
的数据，也没有把诊断结果导入性能基线。

后续需要定位并修复 Curvine CREATE/Open 链路中的 UID/GID 传递或持久化，
再选择经过验证的 Curvine 组件版本。不能用让 Runner 改为 root、关闭权限
检查、后台递归 chown 或预置正式试验文件来掩盖问题。修复后若组件版本或
挂载条件改变，登记新的环境快照和用例版本，并使用新的 Request ID 重新
执行全部 3 次重复；旧失败证据保持不变。

临时权限/诊断 Pod 已删除，专用卷、正式失败实验和诊断文件保留便于复查。
未升级 Curvine 集群组件；API 服务继续 Ready，Kubernetes 后端保持已启用。
