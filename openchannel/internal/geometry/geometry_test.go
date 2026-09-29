package geometry

import (
	"math"
	"testing"
)

func approxEq(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func TestRectangleGeometry(t *testing.T) {
	// 矩形就是 m=0 的梯形：b=2, y=1
	r := NewRectangle(2)
	if got := r.Area(1); got != 2 {
		t.Errorf("矩形面积 A = %v, 期望 2", got)
	}
	if got := r.WettedPerimeter(1); got != 4 {
		t.Errorf("矩形湿周 P = %v, 期望 4", got)
	}
	if got := r.TopWidth(1); got != 2 {
		t.Errorf("矩形水面宽 T = %v, 期望 2（应等于底宽）", got)
	}
	if got := r.HydraulicRadius(1); !approxEq(got, 0.5, 1e-15) {
		t.Errorf("矩形水力半径 R = %v, 期望 0.5", got)
	}
}

func TestTrapezoidGeometry(t *testing.T) {
	// b=3, m=1（45°边坡）, y=2:
	// A=(3+1·2)·2=10, T=3+2·1·2=7, P=3+2·2·sqrt(2)=8.65685, R=10/P
	s := NewTrapezoid(3, 1)
	if got := s.Area(2); !approxEq(got, 10, 1e-12) {
		t.Errorf("梯形面积 A = %v, 期望 10", got)
	}
	if got := s.TopWidth(2); !approxEq(got, 7, 1e-12) {
		t.Errorf("梯形水面宽 T = %v, 期望 7", got)
	}
	wantP := 3 + 4*math.Sqrt2
	if got := s.WettedPerimeter(2); !approxEq(got, wantP, 1e-12) {
		t.Errorf("梯形湿周 P = %v, 期望 %v", got, wantP)
	}
	if got := s.HydraulicRadius(2); !approxEq(got, 10/wantP, 1e-12) {
		t.Errorf("梯形水力半径 R = %v, 期望 %v", got, 10/wantP)
	}
}

func TestRectangleIsZeroSideSlopeTrapezoid(t *testing.T) {
	// 关键自洽性：矩形与 m=0 梯形在所有几何量上必须完全一致，
	// 从结构上杜绝“矩形/梯形各抄一份公式”。
	r := NewRectangle(4)
	z := NewTrapezoid(4, 0)
	for _, y := range []float64{0.01, 0.7, 3.0} {
		if r.Area(y) != z.Area(y) {
			t.Errorf("y=%v 面积不一致: %v vs %v", y, r.Area(y), z.Area(y))
		}
		if r.WettedPerimeter(y) != z.WettedPerimeter(y) {
			t.Errorf("y=%v 湿周不一致", y)
		}
		if r.TopWidth(y) != z.TopWidth(y) {
			t.Errorf("y=%v 水面宽不一致", y)
		}
		if r.HydraulicRadius(y) != z.HydraulicRadius(y) {
			t.Errorf("y=%v 水力半径不一致", y)
		}
	}
}

func TestIsValid(t *testing.T) {
	cases := []struct {
		name string
		s    Section
		want bool
	}{
		{"矩形合法", NewRectangle(1), true},
		{"梯形合法", NewTrapezoid(1, 2), true},
		{"底宽为零", Section{0, 1}, false},
		{"底宽为负", Section{-1, 1}, false},
		{"边坡为负", Section{1, -0.5}, false},
		{"底宽 NaN", Section{math.NaN(), 1}, false},
		{"边坡 Inf", Section{1, math.Inf(1)}, false},
	}
	for _, c := range cases {
		if got := c.s.IsValid(); got != c.want {
			t.Errorf("%s: IsValid=%v, 期望 %v", c.name, got, c.want)
		}
	}
}

// r_TopWidth 间接调用，保持测试可读。
func r_TopWidth(s Section, y float64) float64 { return s.TopWidth(y) }
