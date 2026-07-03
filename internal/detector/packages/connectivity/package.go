package connectivity

import (
	"encoding/json"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/plugins/dnscheck"
	"github.com/aak1247/logtap/internal/detector/plugins/sslcheck"
	"github.com/aak1247/logtap/internal/detector/plugins/tcpcheck"
)

const PackageID = "builtin.connectivity"

type Package struct{}

func New() Package { return Package{} }

func (Package) Manifest() detector.PackageManifest {
	return detector.PackageManifest{
		ID:          PackageID,
		Name:        "连接性检查",
		Version:     "1.0.0",
		Builtin:     true,
		Description: "内置 TCP、DNS、SSL 可用性检查插件包",
		Detectors:   []string{"tcp_check", "dns_check", "ssl_check"},
	}
}

func (Package) Detectors() []detector.DetectorPlugin {
	return []detector.DetectorPlugin{
		tcpcheck.New(),
		dnscheck.New(),
		sslcheck.New(),
	}
}

func (Package) Views() []detector.ViewDescriptor {
	return []detector.ViewDescriptor{
		overviewCard("tcp_check", "TCP 状态", "TCP 连通性、延迟和成功率"),
		overviewCard("dns_check", "DNS 状态", "DNS 解析状态、响应时间和期望值匹配"),
		overviewCard("ssl_check", "SSL 状态", "证书有效期、链路校验和 SAN 信息"),
		{
			ID:      "builtin.connectivity.analysis",
			Surface: detector.ViewSurfaceAnalyticsTab,
			Title:   "连接性分析",
			View: json.RawMessage(`{
				"type": "panel",
				"title": "连接性分析",
				"children": [
					{"type": "text", "text": "TCP、DNS、SSL 检查结果会在这里聚合展示。"}
				]
			}`),
		},
	}
}

func overviewCard(detectorType string, title string, description string) detector.ViewDescriptor {
	metricPath := "summary.successRate"
	metricLabel := "成功率"
	metricUnit := "%"
	if detectorType == "ssl_check" {
		metricPath = "summary.worstCertDays"
		metricLabel = "最短剩余"
		metricUnit = "天"
	}
	return detector.ViewDescriptor{
		ID:           "builtin.connectivity." + detectorType + ".overview",
		DetectorType: detectorType,
		Surface:      detector.ViewSurfaceOverviewCard,
		Title:        title,
		View: json.RawMessage(`{
			"type": "stack",
			"gap": "md",
			"children": [
				{"type": "grid", "columns": 3, "children": [
					{"type": "metric", "label": "状态", "valuePath": "summary.status"},
					{"type": "metric", "label": "` + metricLabel + `", "valuePath": "` + metricPath + `", "unit": "` + metricUnit + `"},
					{"type": "metric", "label": "平均延迟", "valuePath": "summary.avgElapsedMs", "unit": "ms"}
				]},
				{"type": "text", "text": "` + description + `", "tone": "muted"},
				{"type": "table", "valuePath": "monitors", "columns": [
					{"label": "名称", "valuePath": "name"},
					{"label": "目标", "valuePath": "target"},
					{"label": "状态", "valuePath": "status"},
					{"label": "最近错误", "valuePath": "lastError"}
				]}
			]
		}`),
	}
}
