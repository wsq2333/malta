import { useState } from "react";
import { ArrowRight, FlaskConical } from "lucide-react";
import { BackToList, Facts, RecordCard, TechnicalInfo } from "./RecordViews";
import { Dialog } from "./components";

// Deliberately isolated fixture: never read or write real issue/run APIs.
const results = [
  {
    id: "create-reopen",
    name: "普通用户创建后重新打开",
    status: "失败",
    expected: "UID 10001 可再次打开自己创建的文件",
    actual: "open 返回 EACCES",
    issue: true,
  },
  {
    id: "rename",
    name: "重命名后内容保持一致",
    status: "通过",
    expected: "内容与校验和不变",
    actual: "内容一致",
    issue: false,
  },
  {
    id: "mmap",
    name: "mmap 写入可见性",
    status: "跳过",
    expected: "映射写入后可读到新内容",
    actual: "示例环境未启用此测试能力",
    issue: false,
  },
];
export function AutomationPreview({
  mode,
  detail,
  onDetail,
  onNavigate,
}: {
  mode: "runs" | "suites";
  detail: string;
  onDetail: (id: string) => void;
  onNavigate: (page: "automation" | "suites") => void;
}) {
  const [filter, setFilter] = useState("全部");
  const [issue, setIssue] = useState(false);
  const [verification, setVerification] = useState("pending");
  const issueStatus = verification === "passed" ? "已解决" : "待验证";
  const [retest, setRetest] = useState(false);
  const [expanded, setExpanded] = useState("");
  const suite = mode === "suites";
  return (
    <>
      <div className="prototype-banner">
        <FlaskConical size={18} />
        <div>
          <strong>交互预览</strong>
          <p>
            以下套件、结果和问题均为虚构示例，不代表真实集群状态。自动化执行、报告导入和复测关联尚未接入。
          </p>
        </div>
      </div>
      {!detail ? (
        <>
          <div className="section-actions">
            <p className="muted">
              {suite
                ? "版本化套件 · 固定用例与运行条件"
                : "从单项结果追溯问题，再用复测确认修复"}
            </p>
            <button
              className="secondary"
              onClick={() => onNavigate(suite ? "automation" : "suites")}
            >
              {suite ? "查看验证示例" : "查看套件示例"}
              <ArrowRight size={15} />
            </button>
          </div>
          <div className="record-grid">
            <RecordCard
              title="Curvine FUSE 基础回归"
              subtitle={
                <span className="preview-tag">
                  示例 · {suite ? "修订 v1" : "有失败"}
                </span>
              }
              onOpen={() => onDetail(suite ? "demo-suite" : "demo-run")}
            >
              <p className="muted">
                {suite
                  ? "文件创建、权限、重命名与 mmap 语义"
                  : "示例版本 A · 阿里云 · 示例运行"}
              </p>
              <p className="preview-counts">
                {suite
                  ? "3 项用例 · 单客户端 · UID 10001"
                  : "通过 1 · 失败 1 · 跳过 1"}
              </p>
              <p className="muted">
                {suite
                  ? "执行后输出逐项结果与原始证据"
                  : "关联问题 1 · 待归类失败 0"}
              </p>
            </RecordCard>
          </div>
        </>
      ) : (
        <>
          <BackToList
            label={suite ? "返回自动化套件" : "返回自动化验证"}
            onClick={() => onDetail("")}
          />
          {detail !== (suite ? "demo-suite" : "demo-run") ? (
            <p role="status">未找到此示例，请返回列表。</p>
          ) : (
            <section className="panel record-detail">
              <div className="section-actions">
                <div>
                  <span className="preview-tag">
                    示例 · {suite ? "套件修订 v1" : "已完成 · 存在失败项"}
                  </span>
                  <h2>Curvine FUSE 基础回归</h2>
                </div>
                <button className="secondary" onClick={() => setRetest(true)}>
                  {suite ? "预览执行计划" : "预览失败项复测"}
                </button>
              </div>
              <Facts
                items={[
                  ["软件", "Curvine 示例版本 A"],
                  ["环境", "阿里云 · 客户端 1 台 / 服务端 3 台"],
                  ["测试身份", "10001:10001"],
                  ["套件", "curvine-fuse-smoke · v1（示例）"],
                ]}
              />
              {!suite && (
                <div
                  className="result-filters"
                  role="group"
                  aria-label="示例结果筛选"
                >
                  {["全部", "失败", "通过", "跳过"].map((v) => (
                    <button
                      key={v}
                      aria-pressed={filter === v}
                      onClick={() => setFilter(v)}
                    >
                      {v}
                    </button>
                  ))}
                </div>
              )}
              <div className="preview-results">
                {results
                  .filter(
                    (r) => suite || filter === "全部" || r.status === filter,
                  )
                  .map((r) => (
                    <article key={r.id}>
                      <div className="result-line">
                        <button
                          className="result-title"
                          aria-expanded={expanded === r.id}
                          onClick={() =>
                            setExpanded(expanded === r.id ? "" : r.id)
                          }
                        >
                          {r.name}
                        </button>
                        <span
                          className={`badge ${r.status === "失败" ? "failed" : r.status === "通过" ? "succeeded" : "cancelled"}`}
                        >
                          {suite ? "测试定义" : r.status}
                        </span>
                        {!suite && r.issue && (
                          <button
                            className="secondary"
                            onClick={() => setIssue(true)}
                          >
                            DEMO-001 · {issueStatus}
                          </button>
                        )}
                      </div>
                      {expanded === r.id && (
                        <Facts
                          items={
                            suite
                              ? [
                                  ["预期", r.expected],
                                  ["稳定用例 ID", r.id],
                                ]
                              : [
                                  ["预期", r.expected],
                                  ["实际", r.actual],
                                  ["证据", "示例断言；未生成真实日志"],
                                ]
                          }
                        />
                      )}
                    </article>
                  ))}
              </div>
              <TechnicalInfo
                value={{
                  preview_only: true,
                  suite_id: "curvine-fuse-smoke",
                  revision: 1,
                  test_ids: results.map((r) => r.id),
                  identity: { uid: 10001, gid: 10001 },
                }}
              />
            </section>
          )}
        </>
      )}
      {issue && (
        <Dialog
          title="DEMO-001 · 文件重新打开失败（示例）"
          onClose={() => setIssue(false)}
        >
          <div className="preview-dialog">
            <div className="section-actions">
              <span
                className={`badge ${verification === "passed" ? "succeeded" : ""}`}
              >
                {issueStatus}
              </span>
              <span className="muted">
                最近验证：
                {verification === "pending"
                  ? "尚未执行"
                  : verification === "passed"
                    ? "通过（示例）"
                    : "失败（示例）"}
              </span>
            </div>
            <p>
              普通用户创建文件并关闭后，重新打开返回
              EACCES。以下时间线仅展示未来的证据组织方式。
            </p>
            <label>
              切换状态示例
              <select
                aria-label="问题验证状态示例"
                value={verification}
                onChange={(e) => setVerification(e.target.value)}
              >
                <option value="pending">待验证</option>
                <option value="passed">复测通过 → 已解决</option>
                <option value="failed">复测失败 → 保持待验证</option>
              </select>
            </label>
            <ol className="evidence-timeline">
              <li>
                <strong>首次发现</strong>
                <p>版本 A · UID 10001 · create-reopen 失败</p>
                <button
                  className="secondary"
                  onClick={() => {
                    setIssue(false);
                    setFilter("失败");
                    setExpanded("create-reopen");
                  }}
                >
                  查看失败结果
                </button>
              </li>
              <li>
                <strong>诊断记录</strong>
                <p>root 条件通过，不能替代原身份的复测。</p>
              </li>
              <li>
                <strong>声明修复</strong>
                <p>示例版本 B · 声明已修复属主处理。</p>
              </li>
              <li>
                <strong>修复验证</strong>
                <p>
                  {verification === "pending"
                    ? "尚未执行；需原失败用例和身份条件通过后才能确认。"
                    : verification === "passed"
                      ? "示例版本 B · UID 10001 · 原失败用例通过，证据完整。示例问题标记已解决，版本 A 的失败记录仍保留。"
                      : "示例版本 B · UID 10001 · 原失败用例仍失败。追加失败证据，继续调查。"}
                </p>
              </li>
            </ol>
            <button
              className="primary"
              onClick={() => {
                setIssue(false);
                setRetest(true);
              }}
            >
              预览复测计划
            </button>
          </div>
        </Dialog>
      )}
      {retest && (
        <Dialog
          title={
            suite
              ? "套件执行计划预览 · 不会提交任务"
              : "复测计划预览 · 不会提交任务"
          }
          onClose={() => setRetest(false)}
        >
          <div className="preview-dialog">
            <Facts
              items={[
                [
                  "目标版本",
                  suite ? "Curvine 示例版本 A" : "Curvine 示例版本 B",
                ],
                [
                  "验证用例",
                  suite ? "create-reopen、rename、mmap" : "create-reopen",
                ],
                ["身份", "UID/GID 10001:10001"],
                ["关联问题", suite ? "执行后按失败项关联" : "DEMO-001"],
                [
                  "确认方式",
                  suite
                    ? "逐项记录通过、失败、跳过和原始证据"
                    : "保存新结果与证据后，判断是否满足修复条件",
                ],
              ]}
            />
            <p className="muted">
              后端接入后，此处将选择实际软件版本、环境快照及执行预算。当前不会创建
              Job 或修改 Issue 状态。
            </p>
            <button className="secondary" onClick={() => setRetest(false)}>
              返回预览
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}
