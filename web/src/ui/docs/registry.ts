import overview from "../../../../docs/OVERVIEW.md?raw";
import deployment from "../../../../docs/DEPLOYMENT.md?raw";
import ingest from "../../../../docs/INGEST.md?raw";
import sdks from "../../../../docs/SDKs.md?raw";
import sdkSpec from "../../../../docs/SDK_SPEC.md?raw";

import integrationJavascript from "../../../../docs/integrations/javascript.md?raw";
import integrationGo from "../../../../docs/integrations/go.md?raw";
import integrationFlutter from "../../../../docs/integrations/flutter.md?raw";
import integrationPython from "../../../../docs/integrations/python.md?raw";
import integrationJava from "../../../../docs/integrations/java.md?raw";
import integrationDotnet from "../../../../docs/integrations/dotnet.md?raw";
import integrationPhp from "../../../../docs/integrations/php.md?raw";
import integrationRuby from "../../../../docs/integrations/ruby.md?raw";

export type DocItem = {
  id: string;
  title: string;
  content: string;
  group?: string;
};

export const docs: DocItem[] = [
  { id: "overview", title: "简介", content: overview, group: "开始" },
  { id: "deployment", title: "部署指引", content: deployment, group: "开始" },

  { id: "ingest", title: "上报接口与模型", content: ingest, group: "协议" },
  { id: "sdks", title: "SDK 快速开始", content: sdks, group: "SDK" },
  { id: "sdk-spec", title: "SDK 统一规范", content: sdkSpec, group: "SDK" },

  {
    id: "integrations/javascript",
    title: "JavaScript / TypeScript",
    content: integrationJavascript,
    group: "语言集成",
  },
  { id: "integrations/go", title: "Go", content: integrationGo, group: "语言集成" },
  {
    id: "integrations/flutter",
    title: "Flutter",
    content: integrationFlutter,
    group: "语言集成",
  },
  {
    id: "integrations/python",
    title: "Python",
    content: integrationPython,
    group: "语言集成",
  },
  {
    id: "integrations/java",
    title: "Java",
    content: integrationJava,
    group: "语言集成",
  },
  {
    id: "integrations/dotnet",
    title: ".NET",
    content: integrationDotnet,
    group: "语言集成",
  },
  {
    id: "integrations/php",
    title: "PHP",
    content: integrationPhp,
    group: "语言集成",
  },
  {
    id: "integrations/ruby",
    title: "Ruby",
    content: integrationRuby,
    group: "语言集成",
  },
];

export function findDoc(id: string) {
  return docs.find((d) => d.id === id) ?? docs[0];
}

export function groupedDocs() {
  const groups = new Map<string, DocItem[]>();
  for (const d of docs) {
    const g = d.group ?? "其他";
    const arr = groups.get(g) ?? [];
    arr.push(d);
    groups.set(g, arr);
  }
  return [...groups.entries()];
}

