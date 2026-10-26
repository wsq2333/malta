import { Badge } from "./components";
import { date, size, type Run } from "./model";
export function ScenarioView({run}:{run:Run}) {
 const scenario=run.plan.spec.scenario;if(!scenario)return null;
 return <section className="scenario-view"><h3>混合负载场景</h3><p className="muted">{scenario.repeats} 次完整场景重复 · 独立数据集 · 任一子负载失败即停止整组</p>
 {scenario.stages.map(stage=><article key={stage.id} className="scenario-stage"><header><strong>{stage.id}</strong><span className="pill">{stage.mode}</span><span className="muted">预热 {stage.warmup_seconds}s / 测量 {stage.runtime_seconds}s</span></header>
 <div className="scenario-workloads">{stage.workloads.map(w=><div key={w.id}><b>{w.id}</b><span>{w.workload.operation} · 文件 {size(w.workload.file_size_bytes)} / 块 {size(w.workload.block_size_bytes)}</span><small>{w.numjobs} workers × depth {w.iodepth}</small></div>)}</div>
 {Array.from({length:scenario.repeats},(_,i)=>i+1).map(repeat=>{
 const trials=run.trials.filter(t=>t.point.stage_id===stage.id&&t.point.repeat===repeat);
 const valid=trials.filter(t=>t.status==="succeeded"&&Date.parse(t.finished_at||"")>Date.parse(t.started_at||""));
 const submissions=trials.reduce((n,t)=>n+(t.activity?.submitted||0),0);const weightTotal=Object.values(stage.mix?.weights||{}).reduce((a,b)=>a+b,0);
 const first=Math.min(...valid.map(t=>Date.parse(t.started_at!))),last=Math.max(...valid.map(t=>Date.parse(t.finished_at!))),span=last-first;
 return <div className="scenario-repeat" key={repeat}><strong>重复 {repeat}</strong>{stage.mix&&<p className="muted">按提交操作数加权；实际比例受窗口边界影响。</p>}{stage.mix&&trials.map(t=><div key={t.point.id} className="scenario-mix"><b>{t.point.workload_id}</b> 目标 {((stage.mix!.weights[t.point.workload_id!]/weightTotal)*100).toFixed(1)}% / 实际 {submissions?((t.activity?.submitted||0)/submissions*100).toFixed(2):"—"}% · 提交 {t.activity?.submitted??"—"} · 错误 {t.activity?.errors??"—"} · 窗口后完成 {t.activity?.late??"—"}</div>)}{trials.length===0?<span className="muted">等待执行</span>:trials.map(t=><div className="scenario-timeline-row" key={t.point.id}><span>{t.point.workload_id}</span><div className="scenario-track">{valid.includes(t)&&span>0&&<div title={`${date(t.started_at!)} — ${date(t.finished_at!)}`} style={{left:`${100*(Date.parse(t.started_at!)-first)/span}%`,width:`${100*(Date.parse(t.finished_at!)-Date.parse(t.started_at!))/span}%`}}/>}</div><Badge status={t.status}/><small>{valid.includes(t)?`${((Date.parse(t.finished_at!)-Date.parse(t.started_at!))/1000).toFixed(2)}s`:t.error||"未完成"}</small></div>)}</div>;
 })}</article>)}<p className="muted">fio 时间条为 100ms IOPS 采样中实际有 I/O 的观测区间。并行阶段要求共同区间达到设定测量时间的 50%。原生引擎按固定窗口计数，窗口后完成的操作不计入吞吐；延迟为系统调用区间的对数直方图 P99 上界。各子负载 P99 独立展示，不合并为整体 P99。</p></section>
}
