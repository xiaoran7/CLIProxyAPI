package assets

import (
	"io/fs"
	"regexp"
	"strconv"
	"testing"
)

// TestTopologyNodeIconDoesNotOverflow 锁定拓扑图节点尺寸契约：
// 节点框高度、图标缩放、行距三者必须成比例，否则图标会溢出节点框
// 或缩到看不清（生产曾出现图标仅占节点高度 35%、用户反馈"图标不对"）。
func TestTopologyNodeIconDoesNotOverflow(t *testing.T) {
	raw, err := fs.ReadFile(embeddedAssets, "console-traffic.js")
	if err != nil {
		t.Fatalf("read console-traffic.js: %v", err)
	}
	src := string(raw)

	// 1) 不再允许在 SVG 内部再嵌套 <svg>（会造成第二坐标系、缩放失控）
	nested := regexp.MustCompile(`(?s)<g transform="translate\(8, 9\)">\s*<svg `)
	if nested.MatchString(src) {
		t.Error("console-traffic.js still nests <svg> inside the topology <g>; " +
			"the icon must be injected flat so the outer viewBox governs scaling")
	}

	// 2) 节点高度必须 >= 38，图标才有足够空间
	m := regexp.MustCompile(`const nodeH = (\d+);`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("nodeH not found")
	}
	if got := m[1]; got != "38" {
		t.Errorf("nodeH = %s, want 38 (34 was too short: icons filled only 35%% of the box)", got)
	}

	// 3) 图标必须带 scale，且缩放后不超出节点框
	//    16 逻辑单位 * 1.375 = 22，配 38px 框 => 垂直居中 translate(7, 8)
	sm := regexp.MustCompile(`translate\(7, 8\) scale\(([0-9.]+)\)`).FindStringSubmatch(src)
	if sm == nil {
		t.Fatal("icon scale transform not found; expected translate(7, 8) scale(1.375)")
	}
	scale, err := strconv.ParseFloat(sm[1], 64)
	if err != nil {
		t.Fatalf("parse scale %q: %v", sm[1], err)
	}
	const iconCanvas = 16.0
	const nodeHeight = 38.0
	iconSize := iconCanvas * scale
	if iconSize > nodeHeight-8 {
		t.Errorf("scaled icon %.1f units leaves less than 8 units of padding in a %.0f-unit box",
			iconSize, nodeHeight)
	}
	if iconSize < 18 {
		t.Errorf("scaled icon %.1f units is still too small to read (want >= 18)", iconSize)
	}

	// 4) 行距必须大于节点高度，否则节点会互相贴合
	pm := regexp.MustCompile(`maxNodes \* (\d+) \+ 48`).FindStringSubmatch(src)
	if pm == nil {
		t.Fatal("row pitch not found")
	}
	if got := pm[1]; got != "56" {
		t.Errorf("row pitch = %s, want 56 (must exceed nodeH 38 with breathing room)", got)
	}
}
