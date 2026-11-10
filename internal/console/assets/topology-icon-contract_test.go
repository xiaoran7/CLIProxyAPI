package assets

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestTopologyNodeIconDoesNotOverflow 锁定拓扑图节点尺寸契约：
// 节点框高度、图标缩放、行距三者必须成比例，否则图标会溢出节点框
// 或缩到看不清（生产曾出现图标仅占节点高度 35%、用户反馈"图标不对"）。
func TestTopologyNodeIconDoesNotOverflow(t *testing.T) {
	raw, err := fs.ReadFile(embeddedAssets, "console-traffic-topology.js")
	if err != nil {
		t.Fatalf("read console-traffic-topology.js: %v", err)
	}
	src := string(raw)

	// 1) 不再允许在 SVG 内部再嵌套 <svg>（会造成第二坐标系、缩放失控）
	nested := regexp.MustCompile(`(?s)<g transform="translate\(8, 9\)">\s*<svg `)
	if nested.MatchString(src) {
		t.Error("console-traffic-topology.js still nests <svg> inside the topology <g>; " +
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

	// 3) 图标必须归一化到固定尺寸的 viewBox 盒子，而不是靠一个统一 scale 硬放大。
	//    各品牌 path 的有效图形尺寸实测从 6x14（gateway 波形线）到 16x16（stepfun）不等，
	//    同一个 scale 必然有的过大有的过小——gateway 放大后仍是一条细线。
	//    用 <svg viewBox="0 0 16 16" width height> 把每个图标都塞进同一个 22x22 盒子，
	//    缩放比例由 SVG 自己按 viewBox 算，与原始图形占多少画布无关。
	nodeHeight := 38.0
	box := regexp.MustCompile(`<svg class="node-icon" x="7" y="8" width="(\d+)" height="(\d+)" viewBox="0 0 16 16"`).FindStringSubmatch(src)
	if box == nil {
		t.Fatal("icon is not normalised into a fixed viewBox box; " +
			`expected <svg x="7" y="8" width="22" height="22" viewBox="0 0 16 16"`)
	}
	iconW, _ := strconv.Atoi(box[1])
	iconH, _ := strconv.Atoi(box[2])
	if iconW != iconH {
		t.Errorf("icon box %dx%d is not square; brands will render at different optical sizes", iconW, iconH)
	}
	if iconW < 18 {
		t.Errorf("icon box is %d units, still too small to read (want >= 18)", iconW)
	}
	if float64(iconW) > nodeHeight-8 {
		t.Errorf("icon box %d units leaves less than 8 units of padding in a %.0f-unit box",
			iconW, nodeHeight)
	}

	// 4) 必须带运行时 bbox 归一化：各品牌 path 的有效图形实测从 6x14 到 16x16 不等，
	//    只统一 viewBox 盒子只能让容器等大，图形仍然大小不一。
	if !strings.Contains(src, "normalizeNodeIcon") {
		t.Error("console-traffic-topology.js has no runtime bbox normalisation; " +
			"brands with small effective canvases (e.g. the gateway waveform) will render as thin lines")
	}

	// 5) 不得回退到旧的统一 scale 注入方式
	if regexp.MustCompile(`translate\(7, 8\) scale\(`).MatchString(src) {
		t.Error("console-traffic-topology.js still scales every brand with one transform; " +
			"brands have different effective canvas sizes and must be normalised via viewBox")
	}

	// 6) 行距必须大于节点高度，否则节点会互相贴合
	pm := regexp.MustCompile(`maxNodes \* (\d+) \+ 48`).FindStringSubmatch(src)
	if pm == nil {
		t.Fatal("row pitch not found")
	}
	if got := pm[1]; got != "56" {
		t.Errorf("row pitch = %s, want 56 (must exceed nodeH 38 with breathing room)", got)
	}
}
