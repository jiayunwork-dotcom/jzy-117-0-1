package regime

import (
	"math"
	"testing"

	"openchannel/internal/geometry"
	"openchannel/internal/physics"
)

func approxEq(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// 重点回归：矩形断面临界水深闭式解 y_c = (Q²/(g·b²))^(1/3)，
// 用它抽查通用临界水深迭代与流态判定的正确性。
func TestRectangularCriticalDepthClosedForm(t *testing.T) {
	cases := []struct {
		b, Q float64
	}{
		{2, 1.2599210498948732}, // 与曼宁手算算例同一断面/流量
		{5, 50},
		{10, 1e3},
		{1.5, 0.3},
	}
	for _, c := range cases {
		want := RectangularCriticalDepth(c.b, c.Q)
		if want <= 0 {
			t.Fatalf("闭式解非正: b=%v Q=%v", c.b, c.Q)
		}
		s := geometry.NewRectangle(c.b)

		// 1) 通用迭代（梯形公式在 m=0 时）必须与闭式解一致。
		got, err := CriticalDepth(s, c.Q)
		if err != nil {
			t.Fatalf("b=%v Q=%v 迭代临界水深失败: %v", c.b, c.Q, err)
		}
		if !approxEq(got, want, 1e-9) {
			t.Errorf("b=%v Q=%v: 迭代 y_c=%.10f 与闭式解 %.10f 不一致",
				c.b, c.Q, got, want)
		}

		// 2) 在闭式临界水深处，临界条件 Q²·T = g·A³ 必须成立，
		//    且以临界水深对应的临界流速计算 Fr 必须为 1（critical）。
		A := s.Area(want)
		T := s.TopWidth(want)
		lhs := c.Q * c.Q * T
		rhs := physics.G * A * A * A
		if !approxEq(lhs, rhs, 1e-9) {
			t.Errorf("临界条件不成立: Q²T/gA³=%.10f", lhs/rhs)
		}
		vCritical := c.Q / A
		fr := Froude(s, want, vCritical)
		if !approxEq(fr, 1.0, 1e-9) {
			t.Errorf("临界水深处 Fr=%.10f，期望 1", fr)
		}
		if Classify(fr) != Critical {
			t.Errorf("Fr≈1 应判为 critical，得到 %v", Classify(fr))
		}
	}
}

// 流态方向：同一矩形断面 Q 固定时，水深大于 y_c 为缓流，小于 y_c 为急流。
// 弗劳德数必须用 geometry 的同一套面积/水面宽，不能自相矛盾。
func TestFroudeClassifiesEitherSideOfCritical(t *testing.T) {
	const b, Q = 4.0, 20.0
	s := geometry.NewRectangle(b)
	yc := RectangularCriticalDepth(b, Q)

	vAt := func(y float64) float64 { return Q / s.Area(y) }

	frSub := Froude(s, 1.5*yc, vAt(1.5*yc))
	frSuper := Froude(s, 0.5*yc, vAt(0.5*yc))

	if frSub >= 1 || Classify(frSub) != Subcritical {
		t.Errorf("水深 1.5·y_c 应为缓流，Fr=%.6f", frSub)
	}
	if frSuper <= 1 || Classify(frSuper) != Supercritical {
		t.Errorf("水深 0.5·y_c 应为急流，Fr=%.6f", frSuper)
	}
}

// 矩形 Fr 与教科书式 Fr = V/sqrt(g·y) 必须一致（m=0 时 A/T=y）。
func TestRectangularFroudeMatchesTextbook(t *testing.T) {
	const b, y = 3.0, 1.2
	s := geometry.NewRectangle(b)
	V := 2.5
	want := V / math.Sqrt(physics.G*y)
	if got := Froude(s, y, V); !approxEq(got, want, 1e-12) {
		t.Errorf("矩形 Fr=%.10f，教科书式 %.10f", got, want)
	}
}

func TestTrapezoidCriticalDepthSatisfiesCriticalCondition(t *testing.T) {
	cases := []struct {
		b, m, Q float64
	}{
		{4, 1.5, 25},
		{3, 1, 5},
		{6, 3, 100},
	}
	for _, c := range cases {
		s := geometry.NewTrapezoid(c.b, c.m)
		yc, err := CriticalDepth(s, c.Q)
		if err != nil {
			t.Fatalf("梯形临界水深求解失败: %v", err)
		}
		A := s.Area(yc)
		T := s.TopWidth(yc)
		ratio := c.Q * c.Q * T / (physics.G * A * A * A)
		if !approxEq(ratio, 1.0, 1e-9) {
			t.Errorf("b=%v m=%v: 临界条件 Q²T/(gA³)=%.10f，期望 1", c.b, c.m, ratio)
		}
		// 梯形临界水深也必须大于零且随 Q 增大。
		if yc <= 0 {
			t.Errorf("临界水深非正: %v", yc)
		}
	}
}

func TestCriticalDepthMonotonicInQ(t *testing.T) {
	s := geometry.NewTrapezoid(4, 1.5)
	var prev float64
	for _, Q := range []float64{1, 5, 20, 80, 300} {
		yc, err := CriticalDepth(s, Q)
		if err != nil {
			t.Fatal(err)
		}
		if yc <= prev {
			t.Errorf("Q=%v 时 y_c=%.6f 未大于上一个 %.6f", Q, yc, prev)
		}
		prev = yc
	}
}

func TestCriticalDepthInvalid(t *testing.T) {
	if _, err := CriticalDepth(geometry.Section{BottomWidth: -1, SideSlope: 0}, 5); err == nil {
		t.Error("非法断面应报错")
	}
	if _, err := CriticalDepth(geometry.NewRectangle(2), -1); err == nil {
		t.Error("负流量应报错")
	}
	if y, err := CriticalDepth(geometry.NewRectangle(2), 0); err != nil || y != 0 {
		t.Errorf("零流量临界水深应为 0，得到 %v, %v", y, err)
	}
}
