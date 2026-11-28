package compiler

import (
	"strings"
	"testing"
)

// 本文件专测 transition 对象字面量的编译期 lower。
// 核心契约：对象写法产出的 Transition 字段值，与手写字符串写法字节一致。

// transitionDiagCodes 收集指定诊断码。
func transitionDiagCodes(res *Result) map[string]bool {
	out := map[string]bool{}
	for _, d := range res.Analysis.Diagnostics() {
		out[d.Code] = true
	}
	return out
}

func TestTransitionObjByteIdenticalToString(t *testing.T) {
	// 同样语义，对象写法与字符串写法必须产出完全相同的 Transition 字面量。
	cases := []struct {
		name string
		obj  string
		str  string
		want string
	}{
		{
			name: "单属性_数字时长缺省缓动",
			obj:  `style={{ transition: { opacity: { duration: 300 } } }}`,
			str:  `style={{ transition: "opacity 300ms ease" }}`,
			want: `Transition:"opacity 300ms ease"`,
		},
		{
			name: "多属性_混合时长单位",
			obj:  `style={{ transition: { opacity: { duration: 300, easing: "ease" }, transform: { duration: "200ms", easing: "linear" } } }}`,
			str:  `style={{ transition: "opacity 300ms ease, transform 200ms linear" }}`,
			want: `Transition:"opacity 300ms ease, transform 200ms linear"`,
		},
		{
			name: "小数时长字符串_点三秒",
			obj:  `style={{ transition: { width: { duration: ".4s", easing: "ease-in-out" } } }}`,
			str:  `style={{ transition: "width .4s ease-in-out" }}`,
			want: `Transition:"width .4s ease-in-out"`,
		},
		{
			name: "属性名camel转kebab",
			obj:  `style={{ transition: { fontSize: { duration: 250, easing: "ease-out" }, borderRadius: { duration: "0.3s" } } }}`,
			str:  `style={{ transition: "font-size 250ms ease-out, border-radius 0.3s ease" }}`,
			want: `Transition:"font-size 250ms ease-out, border-radius 0.3s ease"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			objRes := compileStyleJSX(t, c.obj)
			assertNoDiag(t, objRes)
			strRes := compileStyleJSX(t, c.str)
			assertNoDiag(t, strRes)

			if !strings.Contains(objRes.GoSource, c.want) {
				t.Errorf("对象写法产物缺少 %q\n%s", c.want, objRes.GoSource)
			}
			// 抽出两边 Transition 字段值逐字节比对。
			objField := extractTransitionField(objRes.GoSource)
			strField := extractTransitionField(strRes.GoSource)
			if objField != strField {
				t.Errorf("对象与字符串写法产物不一致:\n对象: %q\n字符串: %q", objField, strField)
			}
		})
	}
}

// extractTransitionField 从产物里抽出 Transition 字段的整段字面量。
func extractTransitionField(src string) string {
	i := strings.Index(src, "Transition:")
	if i < 0 {
		return ""
	}
	rest := src[i:]
	// 字段值是一个带引号字符串，找到收尾引号。
	if j := strings.Index(rest[11:], `"`); j >= 0 {
		end := 11 + j
		if k := strings.Index(rest[end+1:], `"`); k >= 0 {
			return rest[:end+1+k+1]
		}
	}
	return rest
}

func TestTransitionObjAllEasings(t *testing.T) {
	// 五档缓动全部接受，缺省为 ease。
	for _, e := range []string{"linear", "ease", "ease-in", "ease-out", "ease-in-out"} {
		t.Run(e, func(t *testing.T) {
			res := compileStyleJSX(t, `style={{ transition: { opacity: { duration: 100, easing: "`+e+`" } } }}`)
			assertNoDiag(t, res)
			want := `Transition:"opacity 100ms ` + e + `"`
			if !strings.Contains(res.GoSource, want) {
				t.Errorf("产物缺少 %q\n%s", want, res.GoSource)
			}
		})
	}
	res := compileStyleJSX(t, `style={{ transition: { opacity: { duration: 100 } } }}`)
	assertNoDiag(t, res)
	if !strings.Contains(res.GoSource, `Transition:"opacity 100ms ease"`) {
		t.Errorf("缺省缓动应为 ease\n%s", res.GoSource)
	}
}

func TestTransitionObjDurationForms(t *testing.T) {
	cases := []struct {
		val  string
		want string
	}{
		{`300`, `300ms`},
		{`0.5`, `0.5ms`},
		{`"200ms"`, `200ms`},
		{`".4s"`, `.4s`},
		{`"1.06s"`, `1.06s`},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			res := compileStyleJSX(t, `style={{ transition: { width: { duration: `+c.val+` } } }}`)
			assertNoDiag(t, res)
			want := `Transition:"width ` + c.want + ` ease"`
			if !strings.Contains(res.GoSource, want) {
				t.Errorf("产物缺少 %q\n%s", want, res.GoSource)
			}
		})
	}
}

func TestTransitionObjInvalidDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		expr string
		code string
	}{
		{"all不支持", `style={{ transition: { all: { duration: 300 } } }}`, "style-transition-object"},
		{"缺少duration", `style={{ transition: { opacity: { easing: "ease" } } }}`, "style-transition-field"},
		{"duration为零", `style={{ transition: { opacity: { duration: 0 } } }}`, "style-transition-field"},
		{"duration为负", `style={{ transition: { opacity: { duration: -5 } } }}`, "style-transition-field"},
		{"未知缓动", `style={{ transition: { opacity: { duration: 300, easing: "bounce" } } }}`, "style-transition-field"},
		{"未知字段", `style={{ transition: { opacity: { duration: 300, delay: 100 } } }}`, "style-transition-field"},
		{"配置项非对象", `style={{ transition: { opacity: 300 } }}`, "style-transition-object"},
		{"顶层非对象", `style={{ transition: "opacity 300ms" }}`, ""}, // 字符串写法仍合法，不应报错
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := compileStyleJSX(t, c.expr)
			if c.code == "" {
				assertNoDiag(t, res)
				return
			}
			if res.InvalidGo {
				t.Fatalf("产物被抑制（InvalidGo）")
			}
			codes := transitionDiagCodes(res)
			if !codes[c.code] {
				t.Errorf("应报 %s，得到 %v\n%s", c.code, codes, res.GoSource)
			}
		})
	}
}

func TestTransitionObjInsidePseudoState(t *testing.T) {
	// 对象形式 transition 可用在伪状态子表里。
	res := compileStyleJSX(t, `style={{ hover: { transition: { opacity: { duration: 120, easing: "ease-in" } } } }}`)
	assertNoDiag(t, res)
	if !strings.Contains(res.GoSource, `Hover:&gxscene.Style{`) {
		t.Errorf("伪状态子表未生成\n%s", res.GoSource)
	}
	if !strings.Contains(res.GoSource, `Transition:"opacity 120ms ease-in"`) {
		t.Errorf("伪状态内 transition 未正确 lower\n%s", res.GoSource)
	}
}

func TestTransitionObjCoexistsWithOtherProps(t *testing.T) {
	// transition 对象与其他普通属性、伪状态并存。
	res := compileStyleJSX(t, `style={{ bg: "#fff", transition: { opacity: { duration: 300 } }, hover: { bg: "#eee" } }}`)
	assertNoDiag(t, res)
	for _, want := range []string{
		`Bg:"#fff"`,
		`Transition:"opacity 300ms ease"`,
		`Hover:&gxscene.Style{`,
	} {
		if !strings.Contains(res.GoSource, want) {
			t.Errorf("产物缺少 %q\n%s", want, res.GoSource)
		}
	}
}

func TestTransitionObjEmptyRejected(t *testing.T) {
	res := compileStyleJSX(t, `style={{ transition: {} }}`)
	codes := transitionDiagCodes(res)
	if !codes["style-transition-object"] {
		t.Errorf("空 transition 对象应报 style-transition-object，得到 %v", codes)
	}
}

func TestTransitionStringWriteUnchanged(t *testing.T) {
	// 回归：既有字符串写法不受影响。
	res := compileStyleJSX(t, `style={{ transition: "opacity 300ms ease, transform 200ms linear" }}`)
	assertNoDiag(t, res)
	if !strings.Contains(res.GoSource, `Transition:"opacity 300ms ease, transform 200ms linear"`) {
		t.Errorf("字符串写法被破坏\n%s", res.GoSource)
	}
}

// ---------- animation 对象形式 ----------

func TestAnimationObjByteIdenticalToString(t *testing.T) {
	cases := []struct {
		name string
		obj  string
		str  string
		want string
	}{
		{
			name: "数字时长无限循环",
			obj:  `style={{ animation: { name: "blink", duration: 1060, iterations: -1 } }}`,
			str:  `style={{ animation: "blink 1060ms infinite" }}`,
			want: `Animation:"blink 1060ms infinite"`,
		},
		{
			name: "字符串时长有限次数",
			obj:  `style={{ animation: { name: "blink", duration: "1.06s", iterations: 3 } }}`,
			str:  `style={{ animation: "blink 1.06s 3" }}`,
			want: `Animation:"blink 1.06s 3"`,
		},
		{
			name: "缺省迭代为infinite",
			obj:  `style={{ animation: { name: "blink", duration: 500 } }}`,
			str:  `style={{ animation: "blink 500ms infinite" }}`,
			want: `Animation:"blink 500ms infinite"`,
		},
		{
			name: "iterations写infinite字符串",
			obj:  `style={{ animation: { name: "blink", duration: 200, iterations: "infinite" } }}`,
			str:  `style={{ animation: "blink 200ms infinite" }}`,
			want: `Animation:"blink 200ms infinite"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			objRes := compileStyleJSX(t, c.obj)
			assertNoDiag(t, objRes)
			strRes := compileStyleJSX(t, c.str)
			assertNoDiag(t, strRes)
			if !strings.Contains(objRes.GoSource, c.want) {
				t.Errorf("对象写法产物缺少 %q\n%s", c.want, objRes.GoSource)
			}
			objField := extractAnimationField(objRes.GoSource)
			strField := extractAnimationField(strRes.GoSource)
			if objField != strField {
				t.Errorf("对象与字符串写法产物不一致:\n对象: %q\n字符串: %q", objField, strField)
			}
		})
	}
}

func extractAnimationField(src string) string {
	i := strings.Index(src, "Animation:")
	if i < 0 {
		return ""
	}
	rest := src[i:]
	if j := strings.Index(rest[10:], `"`); j >= 0 {
		end := 10 + j
		if k := strings.Index(rest[end+1:], `"`); k >= 0 {
			return rest[:end+1+k+1]
		}
	}
	return rest
}

func TestAnimationObjInvalidDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		expr string
		code string
	}{
		{"未知name", `style={{ animation: { name: "fade", duration: 300 } }}`, "style-animation-field"},
		{"缺少name", `style={{ animation: { duration: 300 } }}`, "style-animation-field"},
		{"缺少duration", `style={{ animation: { name: "blink" } }}`, "style-animation-field"},
		{"duration为零", `style={{ animation: { name: "blink", duration: 0 } }}`, "style-animation-field"},
		{"迭代为零", `style={{ animation: { name: "blink", duration: 300, iterations: 0 } }}`, "style-animation-field"},
		{"迭代小数", `style={{ animation: { name: "blink", duration: 300, iterations: 1.5 } }}`, "style-animation-field"},
		{"未知字段", `style={{ animation: { name: "blink", duration: 300, delay: 100 } }}`, "style-animation-field"},
		{"非对象", `style={{ animation: 300 }}`, ""}, // 数字标量仍合法（旧写法）
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := compileStyleJSX(t, c.expr)
			if c.code == "" {
				// 数字标量走 old path，不报对象相关诊断（可能产出非法 Go 但不应报 style-animation-*）
				for _, d := range res.Analysis.Diagnostics() {
					if strings.HasPrefix(d.Code, "style-animation") {
						t.Errorf("数字标量不应报对象诊断，得到 %v", res.Analysis.Diagnostics())
					}
				}
				return
			}
			codes := transitionDiagCodes(res)
			if !codes[c.code] {
				t.Errorf("应报 %s，得到 %v\n%s", c.code, codes, res.GoSource)
			}
		})
	}
}

func TestAnimationObjCoexists(t *testing.T) {
	res := compileStyleJSX(t, `style={{ bg: "#fff", animation: { name: "blink", duration: 1060, iterations: -1 }, transition: { opacity: { duration: 300 } } }}`)
	assertNoDiag(t, res)
	for _, want := range []string{
		`Bg:"#fff"`,
		`Animation:"blink 1060ms infinite"`,
		`Transition:"opacity 300ms ease"`,
	} {
		if !strings.Contains(res.GoSource, want) {
			t.Errorf("产物缺少 %q\n%s", want, res.GoSource)
		}
	}
}

func TestAnimationStringWriteUnchanged(t *testing.T) {
	res := compileStyleJSX(t, `style={{ animation: "blink 1.06s infinite" }}`)
	assertNoDiag(t, res)
	if !strings.Contains(res.GoSource, `Animation:"blink 1.06s infinite"`) {
		t.Errorf("字符串写法被破坏\n%s", res.GoSource)
	}
}
