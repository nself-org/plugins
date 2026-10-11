// Purpose: embeds the nSelf Helm chart in the plugin binary, so
// `nself k8s install|upgrade` needs no chart repository.
//
// Inputs: the chart tree at charts/nself (Chart.yaml, values.yaml, templates/).
//
// Outputs: Chart, an embed.FS whose root holds the directory charts/nself.
//
// Constraints: the `all:` prefix is required. Without it go:embed skips every
// file whose name starts with "_" or ".", which drops templates/_helpers.tpl
// and breaks every render. internal/k8s extracts this tree to a private temp
// directory for helm; nothing reads the chart from a remote repository.
package nselfk8s

import "embed"

// ChartRoot is the path of the chart inside Chart.
const ChartRoot = "charts/nself"

// Chart holds the embedded chart under ChartRoot.
//
//go:embed all:charts/nself
var Chart embed.FS
