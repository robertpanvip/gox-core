package gxscene

import "testing"

func TestParseAnimation(t *testing.T) {
	cases := []struct {
		in       string
		active   bool
		durMS    int64
		infinite bool
	}{
		{"blink 1.06s infinite", true, 1060, true},
		{"blink 1060ms", true, 1060, true},
		{"blink 1.06", true, 1060, true},
		{"blink 500ms 3", true, 500, false},
		{"", false, 0, false},
		{"none", false, 0, false},
		{"pulse 1s", false, 0, false}, // 未内置动画名 → 忽略
		{"blink", false, 0, false},    // 缺时长 → 忽略
	}
	for _, c := range cases {
		spec := ParseAnimation(c.in)
		if spec.Active() != c.active {
			t.Errorf("ParseAnimation(%q).Active=%v, want %v", c.in, spec.Active(), c.active)
			continue
		}
		if !c.active {
			continue
		}
		if spec.DurationMS != c.durMS {
			t.Errorf("ParseAnimation(%q).DurationMS=%d, want %d", c.in, spec.DurationMS, c.durMS)
		}
		if spec.Infinite() != c.infinite {
			t.Errorf("ParseAnimation(%q).Infinite=%v, want %v", c.in, spec.Infinite(), c.infinite)
		}
	}
}

func TestAnimationBlinkSample(t *testing.T) {
	spec := ParseAnimation("blink 1000ms infinite")
	// 半周期方波：相位 <500ms → 1，>=500ms → 0。
	cases := []struct {
		nowMS  int64
		op     float32
		active bool
	}{
		{0, 1, true},
		{499, 1, true},
		{500, 0, true},
		{999, 0, true},
		{1000, 1, true}, // 下一周期起点
		{1500, 0, true},
	}
	for _, c := range cases {
		op, active := spec.Sample(c.nowMS)
		if op != c.op || active != c.active {
			t.Errorf("Sample(%d)=(%v,%v), want (%v,%v)", c.nowMS, op, active, c.op, c.active)
		}
	}
}

func TestAnimationFiniteIterations(t *testing.T) {
	spec := ParseAnimation("blink 1000ms 2")
	// 2 次共 2000ms；之后不覆盖基础 opacity。
	if _, active := spec.Sample(1999); !active {
		t.Error("有限次数未结束时应仍活跃")
	}
	if _, active := spec.Sample(2000); active {
		t.Error("有限次数结束后不应再覆盖基础样式")
	}
}

func TestAnimationFromProps(t *testing.T) {
	p := map[string]any{PropAnimation: "blink 1.06s infinite"}
	spec, ok := AnimationFromProps(p)
	if !ok || spec.DurationMS != 1060 {
		t.Fatalf("AnimationFromProps 失败: spec=%+v ok=%v", spec, ok)
	}
	if _, ok := AnimationFromProps(map[string]any{}); ok {
		t.Error("空 props 不应有动画")
	}
}

// ---------- 内嵌关键帧静态结构 ----------

func TestAnimationKeyframesEncode(t *testing.T) {
	a := &AnimationStyle{
		Duration: "1000ms", Iterations: "-1",
		Keyframes: []Keyframe{
			{Offset: "0", Props: Style{Opacity: "0", Transform: "translateX(-100%)"}},
			{Offset: "1", Props: Style{Opacity: "1"}},
		},
	}
	want := `[{"offset":"0","props":{"opacity":"0","transform":"translateX(-100%)"}},{"offset":"1","props":{"opacity":"1"}}]`
	if got := a.keyframesJSON(); got != want {
		t.Errorf("keyframesJSON 不符:\ngot  %s\nwant %s", got, want)
	}
	if str := a.animationString(); str != "1000ms -1" {
		t.Errorf("animationString = %q, want %q", str, "1000ms -1")
	}
}

func TestAnimationKeyframesEncodeEmptyProps(t *testing.T) {
	// 空 props 必须用 {} 保证 JSON 合法。
	a := &AnimationStyle{Keyframes: []Keyframe{{Offset: "0.5"}}}
	if got := a.keyframesJSON(); got != `[{"offset":"0.5","props":{}}]` {
		t.Errorf("空 props 编码 = %s", got)
	}
}

func TestAnimationNoKeyframes(t *testing.T) {
	// 无关键帧时 keyframesJSON 为空，animationString 仍输出简写。
	a := &AnimationStyle{Name: "blink", Duration: "1.06s", Iterations: "infinite"}
	if a.keyframesJSON() != "" {
		t.Errorf("无帧应返回空串")
	}
	if str := a.animationString(); str != "blink 1.06s infinite" {
		t.Errorf("animationString = %q", str)
	}
}

func TestStylePropsEmitsAnimationKeyframes(t *testing.T) {
	// Props() 展平必须产出 animation 简写串 + animation-keyframes JSON。
	s := Style{
		Bg: "#fff",
		Animation: &AnimationStyle{
			Duration: "1000ms", Iterations: "-1",
			Keyframes: []Keyframe{
				{Offset: "0", Props: Style{Opacity: "0"}},
				{Offset: "1", Props: Style{Opacity: "1"}},
			},
		},
	}
	p := s.Props()
	if p["bg"] != "#fff" {
		t.Errorf("普通属性丢失: bg=%q", p["bg"])
	}
	if p["animation"] != "1000ms -1" {
		t.Errorf("animation 简写串 = %q", p["animation"])
	}
	wantKF := `[{"offset":"0","props":{"opacity":"0"}},{"offset":"1","props":{"opacity":"1"}}]`
	if p[PropAnimationKeyframes] != wantKF {
		t.Errorf("animation-keyframes = %q, want %q", p[PropAnimationKeyframes], wantKF)
	}
}

func TestStylePropsBlinkNoKeyframeKey(t *testing.T) {
	// 内置 blink 不产生 animation-keyframes 键（零破坏旧链路）。
	s := Style{Animation: &AnimationStyle{Name: "blink", Duration: "1.06s", Iterations: "infinite"}}
	p := s.Props()
	if p["animation"] != "blink 1.06s infinite" {
		t.Errorf("animation = %q", p["animation"])
	}
	if _, ok := p[PropAnimationKeyframes]; ok {
		t.Errorf("blink 不应产生 animation-keyframes 键")
	}
}

func TestAnimationStyleFromAnyMap(t *testing.T) {
	// 动态构造（JSON/headless）路径：map → 静态 struct。
	a, err := animationStyleFromAny(map[string]any{
		"duration": "1000ms", "iterations": "-1",
		"keyframes": []any{
			map[string]any{"offset": "0", "props": map[string]any{"opacity": "0"}},
			map[string]any{"offset": "1", "props": map[string]any{"opacity": "1"}},
		},
	})
	if err != nil {
		t.Fatalf("animationStyleFromAny: %v", err)
	}
	if a.Duration != "1000ms" || a.Iterations != "-1" || len(a.Keyframes) != 2 {
		t.Fatalf("解析结果不符: %+v", a)
	}
	if a.Keyframes[0].Props.Opacity != "0" {
		t.Errorf("首帧 opacity = %q", a.Keyframes[0].Props.Opacity)
	}
}

func TestAnimationStyleFromAnyRejectsUnknown(t *testing.T) {
	if _, err := animationStyleFromAny(map[string]any{"bogus": "x"}); err == nil {
		t.Error("未知字段应报错")
	}
	if _, err := animationStyleFromAny(map[string]any{
		"keyframes": []any{map[string]any{"offset": "0", "bogus": "x"}},
	}); err == nil {
		t.Error("关键帧未知字段应报错")
	}
}

func TestStyleFromMapAnimation(t *testing.T) {
	// StyleFromMap 完整路径：animation 对象 → Props 展平。
	s, err := StyleFromMap(map[string]any{
		"animation": map[string]any{
			"duration": "500ms", "iterations": "3",
			"keyframes": []any{
				map[string]any{"offset": "0", "props": map[string]any{"opacity": "0"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("StyleFromMap: %v", err)
	}
	p := s.Props()
	if p["animation"] != "500ms 3" {
		t.Errorf("animation = %q", p["animation"])
	}
	if p[PropAnimationKeyframes] == "" {
		t.Error("animation-keyframes 缺失")
	}
}

// ===== 跨层契约：静态 struct → Props 展平 → 解析，闭环 =====

func TestContractParseAnimationNoNameShorthand(t *testing.T) {
	// gox-core 自定义关键帧展平出的 animation 串无 name（如 "2000ms 3"）。
	// ParseAnimation 必须能解析出时长/次数，否则全链路契约在源头就断了。
	s := ParseAnimation("2000ms 3")
	if !s.Active() {
		t.Fatal("无 name 简写串应被识别为活跃动画")
	}
	if s.DurationMS != 2000 {
		t.Errorf("DurationMS = %d, want 2000", s.DurationMS)
	}
	if s.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", s.Iterations)
	}
	s = ParseAnimation("1.5s infinite")
	if s.DurationMS != 1500 || s.Iterations != -1 {
		t.Errorf("秒单位/infinite 解析错误: %+v", s)
	}
}

func TestContractAnimationStyleRoundTrip(t *testing.T) {
	// 编译产物形态的 AnimationStyle → Props 展平 → 从 props 解析，闭环一致。
	style := Style{
		Bg: "#fff",
		Animation: &AnimationStyle{
			Duration: "2000ms", Iterations: "3",
			Keyframes: []Keyframe{
				{Offset: "0", Props: Style{Opacity: "0"}},
				{Offset: "1", Props: Style{Opacity: "1"}},
			},
		},
	}
	p := style.Props()
	if p["animation"] != "2000ms 3" {
		t.Errorf("animation 简写 = %q", p["animation"])
	}
	// 用通用 map 通道读回（AnimationFromProps 接受 map[string]any）。
	m := map[string]any{"animation": p["animation"], PropAnimationKeyframes: p[PropAnimationKeyframes]}
	spec, ok := AnimationFromProps(m)
	if !ok {
		t.Fatal("展平后的 props 应能被 AnimationFromProps 识别")
	}
	if spec.DurationMS != 2000 || spec.Iterations != 3 {
		t.Errorf("回读 spec 不一致: %+v", spec)
	}
}

func TestContractKeyframesJSONByteStable(t *testing.T) {
	// 关键帧 JSON 是跨语言契约（Rust/Web 都按此解析），字节必须稳定：
	// props 按 Style 固定字段顺序输出，offset 字符串原样。
	a := &AnimationStyle{
		Keyframes: []Keyframe{
			{Offset: "0", Props: Style{Opacity: "0", Transform: "translateX(-100px)"}},
			{Offset: "1", Props: Style{Opacity: "1"}},
		},
	}
	got := a.keyframesJSON()
	want := `[{"offset":"0","props":{"opacity":"0","transform":"translateX(-100px)"}},{"offset":"1","props":{"opacity":"1"}}]`
	if got != want {
		t.Errorf("关键帧 JSON 字节不稳定:\ngot  %s\nwant %s", got, want)
	}
}

func TestContractUnknownNameStillRejected(t *testing.T) {
	// 匿名串仅接受「时长打头」；任意未知动画名必须 inactive，防止误把
	// 拼写错误的动画名当成有效动画（回归保护）。
	for _, s := range []string{"pulse 1s", "fade-in 300ms", "typo 2000ms 3"} {
		if ParseAnimation(s).Active() {
			t.Errorf("%q 应为 inactive", s)
		}
	}
}
