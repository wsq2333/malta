import { useState } from "react";
import {
  ArrowRight,
  BookOpen,
  Check,
  Copy,
  Download,
  FileJson,
  GitCompareArrows,
  Play,
} from "lucide-react";
import examples from "./guide-examples.json";
import issueExample from "./issue-example.json";

function CodeExample({
  title,
  text,
  filename,
}: {
  title: string;
  text: string;
  filename?: string;
}) {
  const [message, setMessage] = useState("");
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setMessage("已复制");
    } catch {
      setMessage("无法访问剪贴板，请选中下方内容手动复制。");
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([text + "\n"], { type: "application/json" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = filename || "example.json";
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <div className="guide-code">
      <div className="guide-code-header">
        <strong>{title}</strong>
        <div>
          <button className="secondary" onClick={copy}>
            <Copy size={14} />
            复制
          </button>
          {filename && (
            <button className="secondary" onClick={download}>
              <Download size={14} />
              下载 JSON
            </button>
          )}
        </div>
      </div>
      <details>
        <summary>展开 JSON 示例</summary>
        <pre tabIndex={0} aria-label={title}>
          <code>{text}</code>
        </pre>
      </details>
      <span className="guide-copy-status" role="status">
        {message}
      </span>
    </div>
  );
}
const fields = [
  [
    "schema_version",
    "普通计划填 1.0；混合场景填 scenario/v1alpha1。使用对应示例，不混用两种结构。",
  ],
  [
    "spec",
    "测试负载。普通 I/O 使用 workload、matrix 和 repeats；混合负载在 spec.scenario 中配置 stages。",
  ],
  [
    "environment",
    "实际硬件快照：云厂商、区域、机器型号、数量、CPU、内存、磁盘、网络、OS 和挂载配置。示例 unknown 只用于功能验证。",
  ],
  [
    "version",
    "文件系统身份 filesystem、版本标签 label、commit 或 build。软件升级后使用新的 id；同一快照 ID 不能覆盖不同内容。",
  ],
  [
    "target",
    "执行器内已存在的绝对测试目录，不能是根目录。local 对应服务节点目录，Kubernetes 对应 Runner 挂载路径。",
  ],
  [
    "resource_group",
    "被测资源的稳定标识，例如同一块盘或同一个文件系统。名称可使用字母、数字、点、下划线和短横线。",
  ],
  [
    "trials / hash",
    "由负载展开生成，不需要手算。编辑 JSON 后在用例管理中保存新版本，或用 malta plan 重新生成；直接发起测试会校验哈希。",
  ],
];
const workloadFields = [
  [
    "operation",
    "read / write：顺序读写；randread / randwrite：随机读写；randrw：随机混合读写。",
  ],
  [
    "file_size_bytes / files_per_job",
    "每个文件大小与每个 worker 的文件数。8388608 是 8 MiB。总准备量随文件数、并发和展开后的计划变化，由服务检查预算。",
  ],
  [
    "block_size_bytes",
    "单次 I/O 大小，4096 是 4 KiB；需为 512 的倍数，并整除文件大小。它与文件总大小是两个参数。",
  ],
  [
    "read_percent",
    "randrw 的读取比例，0–100。比如 70 表示 70% 读、30% 写；不能用它代替多个子负载的权重。",
  ],
  [
    "matrix.numjobs / matrix.iodepth",
    "并发 worker 数与每个 worker 的队列深度；两个数组按笛卡尔积展开。psync / sync 的 iodepth 必须为 1。",
  ],
  [
    "runtime_seconds / warmup_seconds / repeats",
    "每次测量时长、预热时长与独立重复次数。repeats 建议至少 3；示例短时运行只验证流程，不代表性能极限。",
  ],
  [
    "ioengine / direct / cache_state",
    "fio 引擎、是否使用 direct I/O、缓存状态声明。psync + direct=false 适合开始验证；warm 不等于已验证的冷缓存。",
  ],
  [
    "seed / fsync_every",
    "固定随机种子便于复现。fsync_every=0 表示不按写入次数调用 fsync；原生引擎目前要求此值为 0。",
  ],
];
export function Guide({
  onNavigate,
}: {
  onNavigate: (page: "cases" | "runs" | "trends") => void;
}) {
  const [example, setExample] = useState("plan");
  const origin = location.origin;
  const exampleValue =
    example === "spec"
      ? examples.data.spec
      : example === "mixed"
        ? examples.mixed
        : examples.data;
  const sections = [
    ["workflow", "开始使用"],
    ["json", "填写 JSON"],
    ["examples", "可复制示例"],
    ["execute", "发起测试"],
    ["compare", "对比结果"],
    ["issues", "问题跟踪"],
    ["faq", "常见问题"],
  ];
  return (
    <div className="guide">
      <section className="panel guide-intro" id="workflow">
        <div>
          <span className="eyebrow">YOUR FIRST BENCHMARK</span>
          <h2>从一份 JSON，到可追溯的性能对照</h2>
          <p>
            先保存一个小型用例，确认路径与执行器可用，再扩大文件大小和并发。每次修改都保留版本，每次运行都有独立证据。
          </p>
          <button className="primary" onClick={() => onNavigate("cases")}>
            前往用例管理 <ArrowRight size={16} />
          </button>
        </div>
        <BookOpen size={48} strokeWidth={1.3} />
      </section>
      <nav className="guide-toc" aria-label="指南目录">
        {sections.map(([id, label], i) => (
          <a key={id} href={`#${id}`}>
            <span>{String(i + 1).padStart(2, "0")}</span>
            {label}
          </a>
        ))}
      </nav>
      <div className="guide-steps">
        {[
          [FileJson, "01", "保存用例", "填写描述和完整计划 JSON，保存为 v1。"],
          [Play, "02", "发起测试", "选定用例版本与执行器，等待证据持久化。"],
          [
            GitCompareArrows,
            "03",
            "建立对照",
            "选择基线和候选运行，先检查条件，再看变化。",
          ],
        ].map(([Icon, n, title, body]) => {
          const Symbol = Icon as typeof FileJson;
          return (
            <div className="panel" key={String(n)}>
              <Symbol size={20} />
              <span className="eyebrow">STEP {String(n)}</span>
              <h3>{String(title)}</h3>
              <p>{String(body)}</p>
            </div>
          );
        })}
      </div>
      <section className="panel guide-section" id="json">
        <div className="guide-heading">
          <span>02</span>
          <h2>JSON 要填哪些内容？</h2>
        </div>
        <p>
          「用例管理」和「发起测试」接收的是完整执行计划 <code>Plan</code>
          ，需要包含负载、环境、软件版本和目标目录。只有 <code>
            workload
          </code>{" "}
          或 <code>spec</code> 的 JSON 不能直接粘贴到发起测试窗口。
        </p>
        <div className="guide-note">
          <Check size={18} />
          <p>
            推荐在「用例管理」中编辑并保存，系统会重新生成 <code>trials</code>{" "}
            与 <code>hash</code>
            。从已有测试载入计划，也可以省去重复填写环境和版本。
          </p>
        </div>
        <h3>完整计划字段</h3>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>字段</th>
                <th>填写方法</th>
              </tr>
            </thead>
            <tbody>
              {fields.map(([field, text]) => (
                <tr key={field}>
                  <td>
                    <code>{field}</code>
                  </td>
                  <td>{text}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <h3>普通文件 I/O 参数</h3>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>字段</th>
                <th>填写方法</th>
              </tr>
            </thead>
            <tbody>
              {workloadFields.map(([field, text]) => (
                <tr key={field}>
                  <td>
                    <code>{field}</code>
                  </td>
                  <td>{text}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>
          JSON
          使用双引号，不支持注释或末尾多余的逗号；数字、布尔值不要写成字符串。填写完整
          Plan 本身，不要包含 API 返回的 <code>data</code> 外层。集群凭证与模型
          Key 放在专用配置中，不填写到用例 JSON。
        </p>
      </section>
      <section className="panel guide-section" id="examples">
        <div className="guide-heading">
          <span>03</span>
          <h2>选择示例，复制后开始</h2>
        </div>
        <div
          className="tabs guide-example-tabs"
          role="group"
          aria-label="示例类型"
        >
          {[
            ["plan", "普通 I/O · 完整计划"],
            ["mixed", "读写 + stat · 完整计划"],
            ["spec", "负载定义 · CLI 输入"],
          ].map(([value, label]) => (
            <button
              key={value}
              className={example === value ? "selected" : ""}
              aria-pressed={example === value}
              onClick={() => setExample(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <p>
          {example === "spec"
            ? "这是 Spec，可以保存为 spec.json 并交给 malta plan --spec。它不是 UI 发起测试所需的完整 Plan。"
            : example === "mixed"
              ? "这是原生 weighted 场景：4 KiB 随机读、1 MiB 写入与 stat 按提交操作数 70:20:10 混合。可导入用例管理；权重不是字节比例，也不是 worker 数量比例。"
              : "这是 8 MiB 文件、4 KiB 顺序读取、1 个 worker、队列深度 1、3 次重复的完整计划。可直接导入用例管理。"}
        </p>
        <div className="guide-note">
          <FileJson size={18} />
          <p>
            示例目录为 <code>/tmp/malta-guide-data</code>
            。执行前由服务管理员在测试节点准备目录并配置{" "}
            <code>--allow-target</code>
            。请填写实际环境、软件版本与路径，再保存用例以更新计划哈希。示例硬件中的
            unknown 不能用于正式基线结论。
          </p>
        </div>
        <CodeExample
          key={example}
          title={
            example === "spec"
              ? "spec.json · 负载定义"
              : "plan.json · 完整执行计划"
          }
          filename={
            example === "spec"
              ? "spec.json"
              : example === "mixed"
                ? "mixed-plan.json"
                : "plan.json"
          }
          text={JSON.stringify(exampleValue, null, 2)}
        />
        <details>
          <summary>通过 CLI 生成完整计划</summary>
          <p>
            把上面的负载定义保存为
            spec.json。环境与版本文件可使用项目模板并改为实际值；命令在 Malta
            项目目录执行，<code>malta</code> 需已安装到 PATH。
          </p>
          <CodeExample
            title="生成计划（不执行 I/O）"
            text={String.raw`malta plan --spec spec.json \
  --environment environments/local.yaml \
  --version environments/version-example.yaml \
  --target /tmp/malta-guide-data \
  --resource-group guide-device --out plan.json`}
          />
          <p>
            混合场景的源文件是 ScenarioSpec，使用{" "}
            <code>--scenario benchmarks/scenarios/weighted-metadata.yaml</code>{" "}
            替代 <code>--spec spec.json</code>。上方混合示例已是完整
            Plan，不要把它当作 ScenarioSpec 再传给 --scenario。
          </p>
        </details>
        <details>
          <summary>混合负载与元数据怎么填写？</summary>
          <p>
            在 <code>spec.scenario.stages</code> 中定义阶段，阶段依次执行。
            <code>sequential</code> 为顺序 fio，<code>parallel</code>{" "}
            为同阶段并行；<code>weighted</code> 按操作次数控制 native
            子负载权重；<code>pipeline</code> 按 create → write → fsync → stat →
            read → unlink 等步骤处理同一个文件。
          </p>
          <p>
            每个阶段填写测量与预热时间，每个子负载填写唯一
            id、kind、engine、numjobs、iodepth 和 workload。weighted 的
            mix.weights 必须覆盖每个子负载
            id；不要在子负载中重复设置非零测量时长。
          </p>
          <p>
            原生引擎当前使用 buffered
            psync、iodepth=1、cache_state=warm、块大小不超过 1
            MiB。独立原生元数据支持 stat；创建和删除用 pipeline。mdtest 的独立
            metadata
            定义目前用于结果导入，不能直接提交托管执行。同一阶段不能混用 fio 与
            native。
          </p>
        </details>
      </section>
      <section className="panel guide-section" id="execute">
        <div className="guide-heading">
          <span>04</span>
          <h2>保存用例并发起测试</h2>
        </div>
        <ol>
          <li>
            <strong>保存定义。</strong>{" "}
            在「用例管理」新建用例，填写名称、描述和变更说明；导入完整 Plan
            JSON，或从已有运行载入。保存后得到 v1。
          </li>
          <li>
            <strong>固定版本。</strong>{" "}
            在用例详情选择历史版本，点击「使用此版本测试」。也可在「发起测试」中切换计划来源，选择已保存的用例及版本。修改参数用「编辑为新版本」，已有测试继续关联旧版本。
          </li>
          <li>
            <strong>选择执行器。</strong> local 在服务节点运行；kubernetes
            只在服务已配置此后端时可选，使用独立 Runner
            Job。测试目录必须位于实际执行环境，且通过服务的目录和资源预算检查。
          </li>
          <li>
            <strong>提交并等待。</strong> 每次新实验使用新的 Request ID（例如{" "}
            <code>read-v1-20261027-001</code>
            ）；同一次提交遇到网络问题重试时复用原
            ID。到「全部测试」观察排队、准备、运行和日志。
          </li>
          <li>
            <strong>确认完成。</strong> 只有状态为「已完成」且证据为{" "}
            <code>complete</code>{" "}
            才适合设为基线。准备阶段也可能耗时；失败时先查看日志，修复定义后生成新版本并使用新的
            Request ID。
          </li>
        </ol>
        <CodeExample
          title="CLI / Agent 按固定用例版本执行"
          text={String.raw`malta --server ${origin} case list
malta --server ${origin} run start \
  --case CASE_ID --case-revision 1 \
  --request-id read-v1-20261027-001 --source agent
malta --server ${origin} run wait RUN_ID --timeout 2m`}
        />
        <p>
          将 CASE_ID、RUN_ID 替换为实际返回值。服务启用了认证时，通过{" "}
          <code>MALTA_TOKEN</code> 提供 Token；远程 CLI 将 --server
          改为可访问的服务地址。等待超时不会自动取消运行，可以继续查询或显式取消。
        </p>
        <button className="secondary" onClick={() => onNavigate("runs")}>
          前往全部测试 <ArrowRight size={15} />
        </button>
      </section>
      <section className="panel guide-section" id="compare">
        <div className="guide-heading">
          <span>05</span>
          <h2>建立基线，比较版本与日期</h2>
        </div>
        <ol>
          <li>
            在成功运行详情中点击「设为基线」，填写有意义的名称，例如{" "}
            <code>release-1.0-random-read</code>
            。也可以直接选择成功运行参与比较。
          </li>
          <li>
            测试新版软件时保留同样的负载与硬件条件，只更新{" "}
            <code>version.id</code>、label 和对应
            commit/build，保存为新的用例版本后运行。比较不同日期时重复执行同一个固定用例版本即可。
          </li>
          <li>
            在「测试中心 →
            性能测试」打开「对比与趋势」，选择基线运行与候选运行。要跨软件版本比较，先展开「版本、环境与日期筛选」，把「软件版本」恢复为「全部版本」，避免隐藏其中一份结果；也要确认日期和环境筛选。
          </li>
          <li>
            先看「条件一致，可进行对照」或具体差异。负载、矩阵、重复数、硬件/OS/配置、文件系统身份、工具版本、Malta
            版本和指标完整性都会影响可比性。当前不同硬件或不同文件系统身份不会计算变化百分比。
          </li>
          <li>
            吞吐与 IOPS 通常越高越好，延迟越低越好。变化率为（候选 − 基线）/
            基线 ×
            100%；延迟下降属于改善。基线为零时不计算百分比，缺失值也不会当成零。
          </li>
        </ol>
        <div className="guide-note">
          <GitCompareArrows size={18} />
          <p>
            展示的是独立重复结果的中位数及 Min–Max。至少 3
            次重复更有助于判断稳定性；混合场景按子负载看指标，不把各组 P99
            相加。native 的 P99 桶上界也不能当作 fio 的精确 P99 直接比较。
          </p>
        </div>
        <p>
          例如：同一硬件和同一负载下，v1 吞吐为 100 MiB/s，v2 为 120
          MiB/s，变化为 +20%。如果同时把块大小从 4 KiB 改为 1
          MiB，平台会指出负载不一致，不能将差异归因于软件版本。
        </p>
        <button className="secondary" onClick={() => onNavigate("trends")}>
          前往对比与趋势 <ArrowRight size={15} />
        </button>
      </section>
      <section className="panel guide-section" id="issues">
        <div className="guide-heading">
          <span>06</span>
          <h2>登记与跟踪问题</h2>
        </div>
        <p>
          在左侧“问题管理”填写表单或上传 JSON。type
          是自定义类型，例如“功能”“性能”或“稳定性”；软件和硬件填写可识别的版本与机器描述。
        </p>
        <p>
          创建时 base_revision 为 0，issue_id
          使用稳定且唯一的标识。关联现有数据可填写 version_id、environment_id 和
          run_id，这些 ID 必须存在；关联运行时版本与环境必须一致。
        </p>
        <p>
          编辑时使用最新 revision 作为 base_revision，保存后保留旧版本。解决时将
          resolved 改为 true 并填写 resolution 和验证结果；重新打开时设为
          false、清空 resolution，并在 change_note
          中记录原因。历史解决方式仍可查阅。
        </p>
        <CodeExample
          title="问题 JSON 模板（示例，需替换实际信息）"
          text={JSON.stringify(issueExample, null, 2)}
          filename="issue.example.json"
        />
        <CodeExample
          title="Agent / CLI 上传与查询"
          text={
            "malta --server http://localhost:8080 issue save --file issue.json\nmalta --server http://localhost:8080 issue get issue-example-fio-reopen\nmalta --server http://localhost:8080 issue history issue-example-fio-reopen"
          }
        />
        <p>
          发现时间使用含时区的
          RFC3339；创建、更新时间及解决时间由服务记录。导出快照包含只读字段，继续编辑请使用编辑窗口中的“下载提交
          JSON”。HTTP 409 表示版本冲突，先重新读取并合并，不要强行覆盖。
        </p>
      </section>
      <section className="panel guide-section" id="faq">
        <div className="guide-heading">
          <span>07</span>
          <h2>遇到问题时</h2>
        </div>
        {[
          [
            "plan schema or checksum mismatch",
            "JSON 修改后没有更新哈希。先到用例管理保存为新版本，或重新运行 malta plan，再使用返回的完整计划。",
          ],
          [
            "target does not exist / outside configured allowed roots",
            "核对实际执行节点上的目录是否存在、是否是测试挂载路径，并请管理员检查服务 --allow-target 或执行器配置；浏览器所在电脑的目录不一定是测试目录。",
          ],
          [
            "plan exceeds server data budget / deadline",
            "减少文件大小、文件数、矩阵或时长，或由管理员调整预算。超时预算还需要给数据准备和结果收集留出时间。",
          ],
          [
            "conflict / HTTP 409",
            "用例已有更新时，重新读取最新版本后再编辑；Request ID 已用于其他计划或用例版本时，为新实验使用新的 ID；环境或软件快照发生变化时也要换新的快照 ID。",
          ],
          [
            "列表没有预期结果 / 无法比较",
            "检查数据、元数据或混合场景分类，以及环境、软件版本和日期筛选。确认运行成功、证据完整，再阅读对比页给出的具体差异。",
          ],
          [
            "unauthorized / HTTP 401",
            "在右上角 API 连接设置中填写服务 Token；CLI 使用 MALTA_TOKEN。使用指南本身不依赖 API 返回实验数据。",
          ],
        ].map(([title, body]) => (
          <details key={title}>
            <summary>{title}</summary>
            <p>{body}</p>
          </details>
        ))}
      </section>
    </div>
  );
}
