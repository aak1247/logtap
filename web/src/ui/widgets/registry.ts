import type { ComponentType } from "react";

export interface WidgetProps {
  settings: {
    apiBase: string;
    token: string;
    projectId: string;
  };
}

export interface WidgetDescriptor {
  type: string;
  detectorType?: string;
  component: ComponentType<WidgetProps>;
  defaultSize: { w: number; h: number };
  title: string;
}

class WidgetRegistryImpl {
  private map = new Map<string, WidgetDescriptor>();

  register(descriptor: WidgetDescriptor): void {
    this.map.set(descriptor.type, descriptor);
  }

  getByDetectorType(detectorType: string): WidgetDescriptor[] {
    const out: WidgetDescriptor[] = [];
    for (const d of this.map.values()) {
      if (d.detectorType === detectorType) out.push(d);
    }
    return out;
  }

  getAll(): WidgetDescriptor[] {
    return Array.from(this.map.values());
  }

  get(type: string): WidgetDescriptor | undefined {
    return this.map.get(type);
  }
}

export const widgetRegistry = new WidgetRegistryImpl();
