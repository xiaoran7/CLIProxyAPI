package assets

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// This file locks the contract between the console shell, the HTML partials
// and the static asset routes.
//
// v8.0.76 split console.html into a shell plus 14 partials and split the JS
// and CSS into many files.  A split only stays safe if three hand-maintained
// lists are kept in agreement:
//
//   - the //go:embed glob, which decides what is embedded at all;
//   - the consolePartials table, which decides which partials are injected
//     into the shell and in what order;
//   - the assetFiles route table, which decides which URLs are served.
//
// Nothing in the compiler connects them.  A partial that is embedded but not
// listed is silently absent from the served page; an asset that is referenced
// by the shell but not routed is a 404 that breaks the page at runtime.  The
// tests below turn both classes of drift into build failures.

// partialPrefixes are the file-name prefixes that make an embedded HTML file
// a console partial rather than the shell itself.
var partialPrefixes = []string{"console-page-", "console-modal-"}

// TestConsolePartialsCoverEveryPartialFile 锁定"嵌入的 partial 一定在表里"。
//
// 只走表不看盘，表外的 partial 会被静默丢掉：文件已经被 //go:embed 收进二进制，
// 编译是绿的，页面也照常打开，只是那个标签页的内容永远不出现。
func TestConsolePartialsCoverEveryPartialFile(t *testing.T) {
	listed := map[string]bool{}
	for _, group := range consolePartials {
		for _, name := range group.files {
			if listed[name] {
				t.Errorf("partial %s is listed in consolePartials more than once", name)
			}
			listed[name] = true
		}
	}

	onDisk := map[string]bool{}
	entries, err := embeddedAssets.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, prefix := range partialPrefixes {
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".html") {
				onDisk[name] = true
			}
		}
	}

	for name := range onDisk {
		if !listed[name] {
			t.Errorf("partial %s is embedded but not listed in consolePartials; it would never reach the served page", name)
		}
	}
	for name := range listed {
		if !onDisk[name] {
			t.Errorf("consolePartials lists %s, which is not embedded; the console would fail to assemble", name)
		}
	}
}

// htmlTagPattern matches an opening or closing HTML tag.
var htmlTagPattern = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)([^>]*?)(/?)>`)

// voidElements never have a closing tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// scriptOrStyleBlock matches the content of a script/style element, whose
// body is not markup and must not be scanned for tags.
var scriptOrStyleBlock = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)

// htmlCommentPattern matches an HTML comment, whose body is not markup.
var htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)

// htmlTagScan scans markup and returns the element stack afterwards, plus the
// first orphan closing tag it met (if any).
//
// The stack holds the names of the elements that are still open when the scan
// ends.  An orphan close is a closing tag that arrives with an empty stack.
// The caller passes the stack in so the scan can run across several partials
// that share one container.
func htmlTagScan(markup string, stack []string) ([]string, string) {
	markup = scriptOrStyleBlock.ReplaceAllString(markup, "")
	markup = htmlCommentPattern.ReplaceAllString(markup, "")

	for _, m := range htmlTagPattern.FindAllStringSubmatch(markup, -1) {
		closing := m[1] == "/"
		name := strings.ToLower(m[2])
		selfClosing := m[4] == "/"
		if voidElements[name] || selfClosing {
			continue
		}
		if closing {
			if len(stack) == 0 {
				return stack, "</" + name + ">"
			}
			stack = stack[:len(stack)-1]
			continue
		}
		stack = append(stack, name)
	}
	return stack, ""
}

// TestConsolePartialGroupsAreTagBalanced 锁定 partial 序列的标签闭合关系。
//
// 拆分是按行切的，而原始单文件里有一行同时写着"上一页的收尾标签"和"下一页的
// 开头标签"（形如 </div></div><div class="tab-page" ...>）。切在行中间之后，
// 单个 partial 自身并不闭合：console-page-status.html 留下两个未闭合的 div，
// console-page-upstream-stats.html 以两个孤立的 </div> 开头。
//
// 拼装出来的整页因此是正确的（原始单文件本来就是这样嵌套的），但只要有人
// 调整 consolePartials 的顺序、删掉一个 partial，或者单独渲染某个 partial，
// 容器就会跨文件串位，把整个 DOM 树搞坏——而且没有任何测试会报错。
//
// 这里锁的正是"顺序"这个契约：按声明顺序拼起来的一组 partial，标签栈在过程
// 中不能出现孤立的闭合标签，结束时必须为空。任何重排、漏项都会让这条断言
// 失败。
func TestConsolePartialGroupsAreTagBalanced(t *testing.T) {
	for _, group := range consolePartials {
		var stack []string
		for _, name := range group.files {
			raw, err := embeddedAssets.ReadFile(name)
			if err != nil {
				t.Fatalf("read partial %s: %v", name, err)
			}
			var orphan string
			stack, orphan = htmlTagScan(string(raw), stack)
			if orphan != "" {
				t.Errorf("partial group %s: %s closes a container that a previous partial opened; "+
					"reordering or dropping a partial would corrupt the DOM", group.marker, orphan)
				break
			}
		}
		if len(stack) > 0 {
			t.Errorf("partial group %s leaves %d element(s) unclosed (%v); "+
				"the last partial must close every container it opens", group.marker, len(stack), stack)
		}
	}
}

// TestEveryReferencedAssetIsRouted 锁定 shell 引用的每个资源都有路由。
//
// 拆分把 22 个 script + 15 个样式表变成 39 + 18，每多一个文件就多一条
// "文件存在但没注册路由"的机会。漏注册的表现是浏览器拿到 404、控制台直接
// 白屏或某个标签页失去功能，而 Go 测试全绿——因为没有任何一条测试把
// console.html 里引用的资源和服务端实际注册的路由对起来。
func TestEveryReferencedAssetIsRouted(t *testing.T) {
	shell, err := embeddedAssets.ReadFile("console.html")
	if err != nil {
		t.Fatalf("read console shell: %v", err)
	}

	reference := regexp.MustCompile(`(?:src|href)="(/console/[^"?]+)(?:\?[^"]*)?"`)
	matches := reference.FindAllStringSubmatch(string(shell), -1)
	if len(matches) == 0 {
		t.Fatal("no asset reference found in the console shell")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	seen := map[string]bool{}
	for _, m := range matches {
		path := m[1]
		if path == "/console" || path == "/console/" {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true

		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("the console shell references %s but the server answers %d; the browser would get a 404 and the page would break", path, rec.Code)
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	t.Logf("verified %d referenced asset routes", len(paths))
}
