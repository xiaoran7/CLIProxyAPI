package assets

import (
	"strings"
	"testing"
)

// TestConsoleAssemblyIsLineEndingSafe 锁定拼装对行尾约定不敏感。
//
// v8.0.76 把 1467 行的 console.html 拆成壳 + 14 个 partial，由服务端在请求
// 时拼装。拼装靠"去掉每个 partial 的行尾、再用行尾把它们接起来"还原原始
// 标记。如果行尾约定被写死成 "\n"，在 CRLF 检出下构建就会在每个拼接边界
// 留下一个多余的 "\r"，并给每个 partial 多插一个空行 —— 页面能打开，但
// 文档字节已经和拆分前不同，之后再对比就分不清是"重构引入的差异"还是
// "检出方式引入的差异"。
//
// 这里用同一份标记分别按 LF 和 CRLF 喂进拼装器，断言两种行尾都得到与各自
// 原始文档逐字节一致的结果。
func TestConsoleAssemblyIsLineEndingSafe(t *testing.T) {
	partials := map[string]string{
		"a.html": "<div class=\"a\">A</div>\n",
		"b.html": "<div class=\"b\">B</div>\n",
	}
	groups := []consolePartialGroup{{
		marker: "<!--CONSOLE_PAGES-->",
		files:  []string{"a.html", "b.html"},
	}}

	for _, term := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(term, "\n", "LF"), func(t *testing.T) {
			shell := "<html>" + term + "<!--CONSOLE_PAGES-->" + term + "</html>"
			body := make(map[string]string, len(partials))
			for name, raw := range partials {
				body[name] = strings.ReplaceAll(raw, "\n", term)
			}

			got, err := assembleShell(shell, groups, func(name string) (string, error) {
				return body[name], nil
			})
			if err != nil {
				t.Fatalf("assembleShell: %v", err)
			}

			want := "<html>" + term +
				"<div class=\"a\">A</div>" + term +
				"<div class=\"b\">B</div>" + term +
				"</html>"
			if got != want {
				t.Errorf("assembled document does not match the original:\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// TestConsoleAssemblyRejectsAMissingPlaceholder 锁定占位标记必须唯一。
//
// 壳里少写一个标记，多出来的 partial 就会静默丢失（页面少一整块功能）；标记
// 写两次，第二个标记留在响应里，浏览器会把注释后的内容当正文渲染。
func TestConsoleAssemblyRejectsAMissingPlaceholder(t *testing.T) {
	groups := []consolePartialGroup{{
		marker: "<!--CONSOLE_PAGES-->",
		files:  []string{"a.html"},
	}}
	read := func(name string) (string, error) { return "<div>A</div>\n", nil }

	if _, err := assembleShell("<html><!--CONSOLE_PAGES--></html>", groups, read); err != nil {
		t.Fatalf("a well-formed shell failed to assemble: %v", err)
	}

	if _, err := assembleShell("<html></html>", groups, read); err == nil {
		t.Error("assembling a shell with no placeholder succeeded; a missing marker would silently drop a partial")
	}

	if _, err := assembleShell("<html><!--CONSOLE_PAGES--><!--CONSOLE_PAGES--></html>", groups, read); err == nil {
		t.Error("assembling a shell with a duplicated placeholder succeeded; the leftover marker would ship to the browser")
	}
}
