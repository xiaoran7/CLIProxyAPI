package assets

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file locks the two hand-built seams between the classic scripts that
// the v8.0.76 split introduced.
//
// Classic scripts share one global scope, so two mechanisms were needed to
// move symbols across a file boundary:
//
//   - window.GATE_OVW, the namespace console-overview-charts.js exports and
//     console-overview.js destructures;
//   - window.GATE_I18N_DICT, the namespace the five console-i18n-dict-*.js
//     files extend and console-i18n.js reads.
//
// Both are lists written twice: once where the symbol is published, once where
// it is consumed.  Nothing in the browser or the compiler checks that the two
// lists still agree, and a mismatch fails at run time, in the page, not in the
// build.

// jsCommentPattern strips a single-line JS comment.
var jsCommentPattern = regexp.MustCompile(`(?m)//[^\n]*`)

// jsIdentifierPattern matches a bare JS identifier.
var jsIdentifierPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// parseIdentifierList reads a comma separated list of identifiers, dropping
// comments and empty entries.
func parseIdentifierList(raw string) []string {
	raw = jsCommentPattern.ReplaceAllString(raw, "")
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !jsIdentifierPattern.MatchString(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// TestOverviewNamespaceExportsMatchTheConsumer 锁定 GATE_OVW 的发布端与消费端一致。
//
// charts.js 把 17 个符号挂到 window.GATE_OVW，overview.js 再把这 17 个符号
// 解构出来用。两份清单各写一遍，仓库里没有 JS 测试工具链（没有 package.json），
// 所以两边一旦漂移，只有页面上那个标签页会炸：少一个名字就是
// "fmtBucketSpan is not a function"，总览页的数字永远显示 --。
//
// 这里把两份清单读出来对集合，任何一边多写或少写都会让构建失败。
func TestOverviewNamespaceExportsMatchTheConsumer(t *testing.T) {
	charts, err := embeddedAssets.ReadFile("console-overview-charts.js")
	if err != nil {
		t.Fatalf("read console-overview-charts.js: %v", err)
	}
	overview, err := embeddedAssets.ReadFile("console-overview.js")
	if err != nil {
		t.Fatalf("read console-overview.js: %v", err)
	}

	exportRe := regexp.MustCompile(`(?s)window\.GATE_OVW\s*=\s*\{(.*?)\n\s*\};`)
	m := exportRe.FindSubmatch(charts)
	if m == nil {
		t.Fatal("console-overview-charts.js no longer publishes window.GATE_OVW")
	}
	exported := parseIdentifierList(string(m[1]))

	destructureRe := regexp.MustCompile(`(?s)const\s*\{(.*?)\}\s*=\s*window\.GATE_OVW`)
	m = destructureRe.FindSubmatch(overview)
	if m == nil {
		t.Fatal("console-overview.js no longer destructures window.GATE_OVW")
	}
	consumed := parseIdentifierList(string(m[1]))

	if len(exported) == 0 {
		t.Fatal("window.GATE_OVW exports an empty list")
	}
	if len(consumed) == 0 {
		t.Fatal("console-overview.js destructures an empty list from window.GATE_OVW")
	}

	exportedSet := map[string]bool{}
	for _, name := range exported {
		exportedSet[name] = true
	}
	consumedSet := map[string]bool{}
	for _, name := range consumed {
		consumedSet[name] = true
	}

	var missing, extra []string
	for _, name := range consumed {
		if !exportedSet[name] {
			missing = append(missing, name)
		}
	}
	for _, name := range exported {
		if !consumedSet[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("console-overview.js consumes %v from window.GATE_OVW but console-overview-charts.js does not export them", missing)
	}
	if len(extra) > 0 {
		t.Errorf("console-overview-charts.js exports %v from window.GATE_OVW but console-overview.js never consumes them", extra)
	}
	t.Logf("window.GATE_OVW: %d exported, %d consumed, sets agree", len(exported), len(consumed))
}

// TestI18nDictionariesLoadBeforeTheRuntime 锁定 i18n 字典先于运行时加载。
//
// 五个 console-i18n-dict-*.js 各自往 window.GATE_I18N_DICT 上 Object.assign，
// console-i18n.js 再读取它。classic script 按文档顺序执行，所以只要有一个
// 字典文件排在 console-i18n.js 之后（或者被新增的字典文件忘了加进页面），
// t(key) 就会在读取未定义的字典时抛错，整页文案退化。
//
// 这里锁两件事：每个已嵌入的字典文件都在页面里，且都排在 console-i18n.js 前面。
func TestI18nDictionariesLoadBeforeTheRuntime(t *testing.T) {
	shell, err := embeddedAssets.ReadFile("console.html")
	if err != nil {
		t.Fatalf("read console shell: %v", err)
	}
	page := string(shell)

	entries, err := embeddedAssets.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded root: %v", err)
	}

	runtimeAt := strings.Index(page, "console-i18n.js")
	if runtimeAt < 0 {
		t.Fatal("the console shell does not load console-i18n.js")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "console-i18n-dict-") || !strings.HasSuffix(name, ".js") {
			continue
		}
		at := strings.Index(page, name)
		if at < 0 {
			t.Errorf("dictionary %s is embedded but never loaded by the console shell", name)
			continue
		}
		if at > runtimeAt {
			t.Errorf("dictionary %s loads after console-i18n.js; the runtime would read an undefined dictionary", name)
		}
	}
}

// TestI18nRuntimeToleratesAMissingDictionary 锁定字典缺失时不会抛异常。
//
// console-i18n.js 通过 window.GATE_I18N_DICT 取字典，而 /admin/i18n.js 这类
// 历史别名路由单独取到的正是这个文件——此时字典文件一个都没加载。没有兜底时
// t(key) 会抛 "Cannot read properties of undefined"，文案整片变成空白。
//
// 仓库里没有 JS 测试工具链，这条契约只能以源码形式锁定：字典赋值必须带空
// 对象兜底。
func TestI18nRuntimeToleratesAMissingDictionary(t *testing.T) {
	runtime, err := embeddedAssets.ReadFile("console-i18n.js")
	if err != nil {
		t.Fatalf("read console-i18n.js: %v", err)
	}
	assign := regexp.MustCompile(`const\s+DICT\s*=\s*window\.GATE_I18N_DICT\s*(\|\|\s*\{\s*\})?\s*;`)
	if !assign.Match(runtime) {
		t.Error("console-i18n.js reads window.GATE_I18N_DICT without an empty-object fallback; " +
			"loading it without the dictionary files makes every t(key) call throw")
	}
}
