import { useEffect, useState } from "react";
import {
  getDetectorAnalysis,
  getPluginPackageAnalysis,
  type ApiSettings,
  type PluginAnalysisResponse,
} from "../../lib/api";
import { JsonViewRenderer } from "../pluginViews/JsonViewRenderer";
import type { JsonViewNode } from "../pluginViews/types";
import type { PluginExtensionDescriptor } from "./registry";

export function PluginAnalysisView(props: {
  extension: PluginExtensionDescriptor;
  settings: ApiSettings;
  hours?: number;
  preferExtensionView?: boolean;
}) {
  const { extension, settings, hours, preferExtensionView = false } = props;
  const [analysis, setAnalysis] = useState<PluginAnalysisResponse | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!settings.token || !settings.projectId) return;
    let cancelled = false;
    (async () => {
      try {
        setErr("");
        const data = extension.detectorType
          ? await getDetectorAnalysis(settings, extension.detectorType, { hours })
          : await getPluginPackageAnalysis(settings, extension.packageId, { hours });
        if (!cancelled) setAnalysis(data);
      } catch (e) {
        if (!cancelled) {
          setAnalysis(null);
          setErr(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    settings.apiBase,
    settings.token,
    settings.projectId,
    extension.detectorType,
    extension.packageId,
    hours,
  ]);

  if (err) {
    return (
      <div className="rounded-lg border border-red-900/60 bg-red-950/40 p-3 text-sm text-red-200">
        {err}
      </div>
    );
  }

  const view = preferExtensionView
    ? extension.view
    : normalizeView(analysis?.view) ?? extension.view;
  return <JsonViewRenderer view={view} data={analysis ?? {}} />;
}

function normalizeView(view: unknown): JsonViewNode | null {
  if (!view || typeof view !== "object") return null;
  const node = view as { type?: unknown };
  if (typeof node.type !== "string") return null;
  return view as JsonViewNode;
}
