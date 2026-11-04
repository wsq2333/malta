import type { ReactNode } from "react";
import { date } from "./model";

export function CaseList({
  items,
}: {
  items: {
    id: string;
    name: string;
    lastRunAt?: string;
    onOpen: () => void;
    actions?: ReactNode;
  }[];
}) {
  return (
    <div className="case-list-wrap">
      <table className="case-list">
        <thead>
          <tr>
            <th scope="col">测试名称</th>
            <th scope="col" title="该用例所有版本中最近一次提交测试的时间">
              最近测试时间
            </th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>
                <button className="case-name-link" onClick={item.onOpen}>
                  {item.name}
                </button>
              </td>
              <td>
                <time dateTime={item.lastRunAt}>
                  {item.lastRunAt ? date(item.lastRunAt) : "尚未测试"}
                </time>
              </td>
              <td>
                <div className="case-row-actions">
                  <button
                    className="secondary"
                    onClick={item.onOpen}
                    aria-label={`查看 ${item.name}`}
                  >
                    查看
                  </button>
                  {item.actions}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
