import type { JsonViewNode } from "../pluginViews/types";

export type PluginSurface = "overview_card" | "analytics_tab" | "page";

export type PluginExtensionDescriptor = {
  id: string;
  packageId: string;
  detectorType?: string;
  surface: PluginSurface;
  title: string;
  path?: string;
  view: JsonViewNode;
};

class PluginExtensionRegistry {
  private map = new Map<string, PluginExtensionDescriptor>();

  register(descriptor: PluginExtensionDescriptor): void {
    this.map.set(descriptor.id, descriptor);
  }

  getOverviewCards(): PluginExtensionDescriptor[] {
    return this.getBySurface("overview_card");
  }

  getAnalyticsTabs(): PluginExtensionDescriptor[] {
    return this.getBySurface("analytics_tab");
  }

  getPages(): PluginExtensionDescriptor[] {
    return this.getBySurface("page");
  }

  get(id: string): PluginExtensionDescriptor | undefined {
    return this.map.get(id);
  }

  getByPath(path: string): PluginExtensionDescriptor | undefined {
    return this.getPages().find((item) => item.path === path);
  }

  private getBySurface(surface: PluginSurface): PluginExtensionDescriptor[] {
    return Array.from(this.map.values())
      .filter((item) => item.surface === surface)
      .sort((a, b) => a.title.localeCompare(b.title));
  }
}

export const pluginExtensionRegistry = new PluginExtensionRegistry();
