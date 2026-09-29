package manning

import (
	"math"
	"testing"

	"openchannel/internal/geometry"
)

// 手算矩形渠算例（回归基线，数值可独立笔算核对）：
//
//	矩形 b=2 m，n=0.030，S0=0.0009，设 y=1 m
//	A = b·y = 2.0 m²
//	P = b+2y = 4.0 m,  R = A/P = 0.5 m
//	Q = (1/n)·A·R^(2/3)·sqrt(S0)
//	  = (1/0.030)·2·0.5^(2/3)·0.03
//	  = 2·0.5^(2/3)
//	  = 1.2599210498948732... m³/s
//
// 因此 Q≈1.2599 m³/s 时正常水深必须在 1.0 m 附近。
// （该工况 y_c=(Q²/(g b²))^(1/3)≈0.343 m，y_n>y_c，为缓流。）
func TestHandCalculatedRectangleExample(t *testing.T) {
	const (
		b      = 2.0
		n      = 0.030
		S0     = 0.0009
		yGiven = 1.0
	)
	s := geometry.NewRectangle(b)

	qGiven := Discharge(s, n, S0, yGiven)
	const wantQ = 1.2599210498948732
	if math.Abs(qGiven-wantQ) > 1e-12 {
		t.Fatalf("手算算例正算流量 Q = %.15f, 期望 %.15f", qGiven, wantQ)
	}

	yn, err := NormalDepth(s, n, S0, wantQ)
	if err != nil {
		t.Fatalf("反求正常水深失败: %v", err)
	}
	// 正常水深量级必须对得上手算的 1 m。
	if math.Abs(yn-yGiven) > 1e-6 {
		t.Errorf("正常水深 y_n = %.8f m, 期望 1.0 m（手算算例）", yn)
	}
	t.Logf("手算矩形渠算例: Q=%.4f m³/s -> y_n=%.6f m", wantQ, yn)
}

// 重点回归：对多组矩形/梯形断面，给定 Q 解出 y_n 后，
// 必须能从曼宁公式收回原来的流量（相对误差进容差）。
func TestNormalDepthRoundTripRecoversDischarge(t *testing.T) {
	const relTol = 1e-9

	cases := []struct {
		name                string
		b, m, nRough, S0, Q float64
	}{
		{"矩形-小流量", 2, 0, 0.030, 0.0009, 0.5},
		{"矩形-手算量级", 2, 0, 0.030, 0.0009, 1.2599210498948732},
		{"矩形-大流量", 5, 0, 0.025, 0.001, 120},
		{"矩形-陡坡", 3, 0, 0.015, 0.05, 40},
		{"梯形-常规", 4, 1.5, 0.022, 0.0008, 25},
		{"梯形-陡边坡", 6, 3, 0.035, 0.002, 5},
		{"梯形-近矩形", 2, 0.001, 0.03, 0.001, 2},
		{"梯形-极小流量", 1, 1, 0.03, 0.0004, 0.01},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := geometry.Section{BottomWidth: c.b, SideSlope: c.m}
			yn, err := NormalDepth(s, c.nRough, c.S0, c.Q)
			if err != nil {
				t.Fatalf("NormalDepth 失败: %v", err)
			}
			if yn <= 0 {
				t.Fatalf("正常水深非正: %v", yn)
			}
			qBack := Discharge(s, c.nRough, c.S0, yn)
			relErr := math.Abs(qBack-c.Q) / c.Q
			if relErr > relTol {
				t.Errorf("流量不自洽: 给定 Q=%.6f, 回代 Q'=%.6f, 相对误差 %.2e > %g",
					c.Q, qBack, relErr, relTol)
			}
			// 流速连续性：V = Q/A，也必须与曼宁正算一致。
			vManning := Velocity(s, c.nRough, c.S0, yn)
			vContinuity := c.Q / s.Area(yn)
			if math.Abs(vManning-vContinuity) > 1e-9*math.Max(1, vManning) {
				t.Errorf("流速不自洽: 曼宁 V=%.8f, 连续方程 V=%.8f", vManning, vContinuity)
			}
		})
	}
}

// 单调性：同一断面、同一流量，渠底坡度越大，正常水深必须越小（严格下降）。
func TestSteeperSlopeGivesShallowerDepth(t *testing.T) {
	s := geometry.NewRectangle(3)
	const Q, nRough = 10.0, 0.025

	var prevY = math.Inf(1)
	for _, S0 := range []float64{0.0001, 0.0005, 0.002, 0.01, 0.05} {
		yn, err := NormalDepth(s, nRough, S0, Q)
		if err != nil {
			t.Fatalf("S0=%v 求解失败: %v", S0, err)
		}
		if !(yn < prevY) {
			t.Errorf("坡度 %v 下 y_n=%.6f 未小于更缓坡度的 %.6f", S0, yn, prevY)
		}
		prevY = yn
	}

	// 梯形断面同样应满足。
	st := geometry.NewTrapezoid(4, 1.5)
	prevY = math.Inf(1)
	for _, S0 := range []float64{0.0002, 0.002, 0.02} {
		yn, err := NormalDepth(st, nRough, S0, Q)
		if err != nil {
			t.Fatalf("梯形 S0=%v 求解失败: %v", S0, err)
		}
		if !(yn < prevY) {
			t.Errorf("梯形坡度 %v 下 y_n=%.6f 未小于更缓坡度的 %.6f", S0, yn, prevY)
		}
		prevY = yn
	}
}

// Discharge 本身关于水深严格单调递增——这是对分法收敛的前提，直接钉住。
func TestDischargeMonotonicInDepth(t *testing.T) {
	cases := []geometry.Section{
		geometry.NewRectangle(2),
		geometry.NewTrapezoid(3, 1),
		geometry.NewTrapezoid(5, 4),
	}
	for _, s := range cases {
		prev := 0.0
		for _, y := range []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10, 25} {
			q := Discharge(s, 0.03, 0.001, y)
			if !(q > prev) {
				t.Errorf("%+v: Q(y=%v)=%.6f 未严格大于 Q(前)=%.6f", s, y, q, prev)
			}
			prev = q
		}
	}
}

func TestNormalDepthInvalidInputs(t *testing.T) {
	s := geometry.NewRectangle(2)
	if _, err := NormalDepth(s, 0, 0.001, 1); err == nil {
		t.Error("糙率为零应当报错")
	}
	if _, err := NormalDepth(s, -0.03, 0.001, 1); err == nil {
		t.Error("糙率为负应当报错")
	}
	if _, err := NormalDepth(s, 0.03, 0, 1); err == nil {
		t.Error("坡度为零应当报错")
	}
	if _, err := NormalDepth(s, 0.03, -0.001, 1); err == nil {
		t.Error("坡度为负应当报错")
	}
	if _, err := NormalDepth(s, 0.03, 0.001, -1); err == nil {
		t.Error("流量为负应当报错")
	}
	if y, err := NormalDepth(s, 0.03, 0.001, 0); err != nil || y != 0 {
		t.Errorf("零流量应退化为水深 0，得到 y=%v err=%v", y, err)
	}
}

func TestExtremeDischargeConverges(t *testing.T) {
	// 很大的流量也必须靠倍区间夹住根并收敛，不能依赖经验初值。
	s := geometry.NewRectangle(10)
	yn, err := NormalDepth(s, 0.03, 0.0001, 1e6)
	if err != nil {
		t.Fatalf("大流量求解失败: %v", err)
	}
	qBack := Discharge(s, 0.03, 0.0001, yn)
	if math.Abs(qBack-1e6)/1e6 > 1e-9 {
		t.Errorf("大流量回代误差过大: %.6f", qBack)
	}
}
