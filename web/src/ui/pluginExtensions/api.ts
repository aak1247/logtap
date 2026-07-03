import type { PluginViewDescriptor } from "../../lib/api";
import type { JsonViewNode } from "../pluginViews/types";
import type { PluginExtensionDescriptor, PluginSurface } from "./registry";

const surfaces = new Set<PluginSurface>(["overview_card", "analytics_tab", "page"]);

export function extensionFromApi(
  item: PluginViewDescriptor,
): PluginExtensionDescriptor | null {
  if (!surfaces.has(item.surface)) return null;
  if (!item.view || typeof item.view !== "object") return null;
  return {
    id: item.id,
    packageId: item.packageId,
    detectorType: item.detectorType,
    surface: item.surface,
    title: item.title,
    path: item.path,
    view: item.view as JsonViewNode,
  };
}
