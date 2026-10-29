export const workspaceNavigation = [
  { page: "baselines", label: "测试中心" },
  { page: "cases", label: "用例管理" },
  { page: "issues", label: "问题管理" },
  { page: "environments", label: "环境与版本" },
  { page: "guide", label: "使用指南" },
];
export function workspaceSection(page: string) {
  if (["runs", "trends", "automation"].includes(page)) return "baselines";
  if (page === "suites") return "cases";
  if (page === "versions") return "environments";
  return page;
}
export function workspaceTabs(page: string) {
  switch (workspaceSection(page)) {
    case "baselines":
      return [
        { page: "baselines", label: "性能测试" },
        { page: "automation", label: "自动化验证" },
      ];
    case "cases":
      return [
        { page: "cases", label: "性能计划" },
        { page: "suites", label: "自动化套件" },
      ];
    case "environments":
      return [
        { page: "environments", label: "硬件环境" },
        { page: "versions", label: "软件版本" },
      ];
    default:
      return [];
  }
}
