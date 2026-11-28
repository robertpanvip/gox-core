package gxscene

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// animation.go —— 三端统一的「声明式动画」协议（受限内置动画，非任意 @keyframes）。
//
// 背景：组件样式绝大多数已由强类型 style prop 表达，唯一残留在宿主 CSS 里的能力
// 缺口是「周期闪烁」（Editor 光标）。为此引入一个受限、声明式、三端一致的动画
// prop，使宿主无需再内联 @keyframes。
//
// 语法（标量 StyleValue，兼容 CSS animation 简写的常用子集）：
//
//	animation = "<name> <duration> [<iterations>]"
//	name       : 内置动画名，当前支持 "blink"（opacity 方波闪烁）
//	duration   : "1.06s" / "1060ms" / "1.06" （缺省单位视为秒）
//	iterations : "infinite"（默认）或正整数；有限次数结束后不覆盖基础样式
//
// 例：animation="blink 1.06s infinite"
//
// 三端语义（严格统一）：
//   - Web：翻译成原生 CSS animation + 注入一次全局 @keyframes gx-blink，
//     由浏览器合成器驱动，不占用 JS/RAF。
//   - Skia/Rust：渲染帧按 now 采样 Spec.Sample 得到 opacity，临时覆盖到节点
//     props（绘制后恢复，不污染常驻树）；绘制层把 opacity 应用到背景/文字 alpha。
//
// 采样相位使用进程纪元毫秒（UnixMilli）取模，三端同机运行时相位一致；不同进程
// 仅相位偏移不同，视觉同为 1.06s 方波，不影响语义。

const (
	// AnimationBlink 是内置的闪烁动画：前半周期 opacity=1，后半周期 opacity=0，
	// 对齐原生 CSS 的 @keyframes gx-blink {0%,49%{opacity:1} 50%,100%{opacity:0}}。
	AnimationBlink = "blink"

	// AnimationCustom 是内嵌 keyframes 动画的解析期内部占位名。此类动画展平后
	// 的 animation 串不含协议名（如 "2000ms 3"），后端凭 keyframes 串识别；
	// 解析期用此名让 AnimationSpec.Active() 成立，Sample() 不按 blink 处理。
	AnimationCustom = "__custom__"

	// PropAnimation 是动画 prop 在后端的协议键。
	PropAnimation = "animation"
)

// ---------- 内嵌关键帧（静态结构，编译期强类型） ----------
//
// animation 是静态 struct，关键帧内聚其中：开发者写
// animation: { duration: 1000, iterations: -1, keyframes: [...] }，
// 编译器生成静态 gxscene.AnimationStyle 结构，字段拼错在编译期报错。
// Props() 展平时编码成现有字符串协议：
//   - name/duration/iterations → "animation" 简写串（兼容内置 blink 链路）
//   - keyframes → "animation-keyframes" 紧凑 JSON 串（新增协议键）
// 与伪状态子表 "&:hover" 的编码方式同构——后端协议仍是字符串，ABI 零改动。

// Keyframe 是一个关键帧：Offset 为 0..1 的进度，Props 是该帧的样式覆盖。
// Props 复用 Style 结构，故 keyframes 里的属性同样受固定字段集约束。
type Keyframe struct {
	Offset StyleValue `gx:"offset"`
	Props  Style      `gx:"props"`
}

// AnimationStyle 是 animation 的静态结构（含内嵌关键帧，非 map）。
// Name 留空且 Keyframes 非空时表示自定义关键帧动画；Name="blink" 走内置。
type AnimationStyle struct {
	Name       StyleValue `gx:"name"`
	Duration   StyleValue `gx:"duration"`
	Iterations StyleValue `gx:"iterations"`
	Keyframes  []Keyframe `gx:"keyframes"`
}

// PropAnimationKeyframes 是内嵌关键帧在后端 props 里的协议键。
const PropAnimationKeyframes = "animation-keyframes"

// animationString 把 animation 结构编码成简写串 "<name> <duration> <iterations>"。
// 等价于旧的字符串写法，后端 ParseAnimation 无需改动。
func (a *AnimationStyle) animationString() string {
	parts := make([]string, 0, 3)
	if a.Name != "" {
		parts = append(parts, string(a.Name))
	}
	if a.Duration != "" {
		parts = append(parts, string(a.Duration))
	}
	if a.Iterations != "" {
		parts = append(parts, string(a.Iterations))
	}
	return strings.Join(parts, " ")
}

// keyframesJSON 把内嵌关键帧编码成紧凑 JSON 串：
//
//	[{"offset":"0","props":{"opacity":"0"}},{"offset":"1","props":{"opacity":"1"}}]
//
// 无帧返回空串，后端据此忽略。props 为空时用 {} 保证 JSON 合法。
func (a *AnimationStyle) keyframesJSON() string {
	if len(a.Keyframes) == 0 {
		return ""
	}
	var parts []string
	for _, f := range a.Keyframes {
		props := encodeSubStyle(f.Props)
		if props == "" {
			props = "{}"
		}
		off, _ := json.Marshal(string(f.Offset))
		parts = append(parts, `{"offset":`+string(off)+`,"props":`+props+`}`)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// AnimationSpec 是 animation prop 的解析结果。
type AnimationSpec struct {
	Name       string // 内置动画名（当前仅 blink）
	DurationMS int64  // 周期毫秒，>0
	Iterations int    // -1 表示 infinite；>0 为有限次数
}

// Active 报告该 spec 是否表达一个有效的、周期大于零的动画。
func (a AnimationSpec) Active() bool {
	return a.Name != "" && a.DurationMS > 0
}

// Infinite 报告是否无限循环。
func (a AnimationSpec) Infinite() bool { return a.Iterations < 0 }

// Sample 计算动画在时刻 nowMS 的 opacity 覆盖值。
// active=false 表示不应覆盖基础 opacity（动画名未知 / 有限次数已结束）。
//
// blink：以 DurationMS 为周期取模，相位 < 50% 返回 1，否则返回 0。
// 有限次数结束后 active=false，让节点回到原始 opacity（对齐 CSS fill-mode:none）。
func (a AnimationSpec) Sample(nowMS int64) (opacity float32, active bool) {
	if !a.Active() {
		return 1, false
	}
	elapsed := nowMS
	if a.Iterations > 0 && elapsed >= a.DurationMS*int64(a.Iterations) {
		return 1, false
	}
	if a.Name != AnimationBlink {
		return 1, false
	}
	phase := elapsed % a.DurationMS
	if phase < 0 {
		phase += a.DurationMS
	}
	if phase*2 < a.DurationMS {
		return 1, true
	}
	return 0, true
}

// ParseAnimation 解析 animation 标量值。非法值返回零值 spec（Active=false），
// 后端据此忽略动画——与「解析失败静默回退」的既有风格一致。
func ParseAnimation(s string) AnimationSpec {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	if len(fields) == 0 {
		return AnimationSpec{}
	}
	spec := AnimationSpec{Name: fields[0], Iterations: -1}
	// 内置 blink 需按名字识别；其余 token 按简写逐项解析。自定义关键帧动画
	// 展平出的串无 name（如 "2000ms 3"），此时 fields[0] 是时长而非名字——
	// 只要能解析出有效时长就作为匿名动画接受，否则先前的源头解析会把
	// 自定义动画判为 inactive，导致 duration/iterations 静默丢失。
	named := spec.Name == AnimationBlink
	// 匿名自定义串的首 token 必须是时长（"2000ms"/"1.5s"/"0.5"）或 infinite，
	// 不能是任意标识符（如 "pulse"）——未知动画名必须保持 inactive。
	anonymous := false
	if !named {
		_, isDuration := parseDurationMS(fields[0])
		anonymous = isDuration || fields[0] == "infinite"
	}
	if !named && !anonymous {
		return AnimationSpec{}
	}
	toks := fields
	if named {
		toks = fields[1:]
	}
	for _, tok := range toks {
		if tok == "infinite" {
			spec.Iterations = -1
			continue
		}
		if n, err := strconv.Atoi(tok); err == nil && n > 0 {
			spec.Iterations = n
			continue
		}
		if d, ok := parseDurationMS(tok); ok && d > 0 {
			spec.DurationMS = d
		}
	}
	if spec.DurationMS <= 0 {
		return AnimationSpec{}
	}
	if !named {
		// 匿名自定义动画（内嵌 keyframes）无协议名。用内部占位名让 Active()
		// 成立；Sample() 因非 blink 不覆盖 opacity，实际帧采样由后端完成。
		spec.Name = AnimationCustom
	}
	return spec
}

// parseDurationMS 解析 "1.06s" / "1060ms" / "1.06"（缺省单位秒）为毫秒。
func parseDurationMS(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "ms") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64)
		if err != nil {
			return 0, false
		}
		return int64(v), true
	}
	if strings.HasSuffix(s, "s") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64)
		if err != nil {
			return 0, false
		}
		return int64(v * 1000), true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(v * 1000), true
}

// animationStyleFromAny 把动态输入（map/字符串/结构）转成静态 AnimationStyle，
// 供 setStyleValue 的动态构造路径使用。未知字段报明确错误。
func animationStyleFromAny(v any) (AnimationStyle, error) {
	switch x := v.(type) {
	case AnimationStyle:
		return x, nil
	case *AnimationStyle:
		if x != nil {
			return *x, nil
		}
		return AnimationStyle{}, nil
	case string:
		// 简写串反解成结构：按空白切分，首 Token 是 name，含单位的是 duration，
		// 其余是 iterations。与 ParseAnimation 的字段顺序约定一致。
		var a AnimationStyle
		for _, tok := range strings.Fields(strings.ToLower(strings.TrimSpace(x))) {
			if tok == "infinite" || (len(tok) > 0 && tok[0] >= '0' && tok[0] <= '9' && !strings.ContainsAny(tok, "sm")) {
				if a.Iterations == "" {
					a.Iterations = tok
				}
				continue
			}
			if _, ok := parseDurationMS(tok); ok {
				if a.Duration == "" {
					a.Duration = tok
				}
				continue
			}
			if a.Name == "" {
				a.Name = tok
			}
		}
		return a, nil
	case map[string]any:
		var a AnimationStyle
		for k, val := range x {
			switch strings.ToLower(k) {
			case "keyframes":
				kfs, err := keyframesFromAny(val)
				if err != nil {
					return AnimationStyle{}, err
				}
				a.Keyframes = kfs
			case "name", "duration", "iterations":
				sv, ok := scalarToStyleValue(val)
				if !ok {
					return AnimationStyle{}, fmt.Errorf("字段 %q 的值不是合法标量", k)
				}
				switch strings.ToLower(k) {
				case "name":
					a.Name = sv
				case "duration":
					a.Duration = sv
				case "iterations":
					a.Iterations = sv
				}
			default:
				return AnimationStyle{}, fmt.Errorf("未知字段 %q（仅 name/duration/iterations/keyframes）", k)
			}
		}
		return a, nil
	default:
		return AnimationStyle{}, fmt.Errorf("值类型 %T 不是 animation 结构", v)
	}
}

// keyframesFromAny 把 []any / []Keyframe 转成静态 []Keyframe。
func keyframesFromAny(v any) ([]Keyframe, error) {
	switch x := v.(type) {
	case []Keyframe:
		return x, nil
	case []any:
		out := make([]Keyframe, 0, len(x))
		for i, item := range x {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("第 %d 帧不是对象", i)
			}
			var kf Keyframe
			for k, val := range m {
				switch strings.ToLower(k) {
				case "offset":
					kf.Offset, _ = scalarToStyleValue(val)
				case "props":
					st, err := styleFromAny(val)
					if err != nil {
						return nil, fmt.Errorf("第 %d 帧 props: %w", i, err)
					}
					kf.Props = st
				default:
					return nil, fmt.Errorf("第 %d 帧未知字段 %q（仅 offset/props）", i, k)
				}
			}
			out = append(out, kf)
		}
		return out, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("keyframes 值类型 %T 不是数组", v)
	}
}

// AnimationFromProps 从后端 props 读取 animation 标量并解析。
func AnimationFromProps(props map[string]any) (AnimationSpec, bool) {
	if props == nil {
		return AnimationSpec{}, false
	}
	v, ok := props[PropAnimation]
	if !ok {
		return AnimationSpec{}, false
	}
	s, ok := v.(string)
	if !ok {
		return AnimationSpec{}, false
	}
	spec := ParseAnimation(s)
	return spec, spec.Active()
}
