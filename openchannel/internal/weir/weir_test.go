package weir

import (
	"math"
	"testing"

	"openchannel/internal/physics"
)

func TestDischargeBasicValue(t *testing.T) {
	// b=1, H=0.2, Cd=0.42:
	// Q = 0.42·1·sqrt(2·9.81)·0.2^1.5
	q, err := Discharge(1, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultDischargeCoeff * math.Sqrt(2*physics.G) * math.Pow(0.2, 1.5)
	if math.Abs(q-want) > 1e-12 {
		t.Errorf("Q=%.10f, 期望 %.10f", q, want)
	}
	if q <= 0 {
		t.Errorf("堰流量应为正: %v", q)
	}
}

// 同一 Cd、同一堰宽下，水头抬高，流量严格按 H 的 3/2 次幂增长。
func TestDischargeScalesAsHeadToThreeHalves(t *testing.T) {
	const b = 2.0
	q1, err := Discharge(b, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	q2, err := Discharge(b, 0.4) // H 扩大 4 倍
	if err != nil {
		t.Fatal(err)
	}
	ratio := q2 / q1
	want := math.Pow(4, 1.5) // = 8
	if math.Abs(ratio-want) > 1e-12 {
		t.Errorf("水头×4 后流量比 = %.10f，期望 4^(3/2)=8", ratio)
	}

	// 任意倍比 r：Q(rH)/Q(H) = r^1.5
	for _, r := range []float64{1.5, 2.7, 10} {
		qa, _ := Discharge(b, 0.3)
		qb, _ := Discharge(b, 0.3*r)
		if math.Abs(qb/qa-math.Pow(r, 1.5)) > 1e-12 {
			t.Errorf("倍比 %v 不满足 3/2 次幂", r)
		}
	}
}

func TestDischargeLinearInWidthAndCoeff(t *testing.T) {
	q1, _ := Discharge(2, 0.25)
	q2, _ := Discharge(4, 0.25)
	if math.Abs(q2/q1-2) > 1e-12 {
		t.Errorf("堰宽加倍流量应加倍，比值 %.10f", q2/q1)
	}

	qa, _ := DischargeWithCoeff(2, 0.25, 0.42)
	qb, _ := DischargeWithCoeff(2, 0.25, 0.84)
	if math.Abs(qb/qa-2) > 1e-12 {
		t.Errorf("系数加倍流量应加倍，比值 %.10f", qb/qa)
	}
}

func TestDischargeInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		b, H float64
	}{
		{"零堰宽", 0, 0.2},
		{"负堰宽", -1, 0.2},
		{"零水头", 2, 0},
		{"负水头", 2, -0.5},
		{"NaN 水头", 2, math.NaN()},
		{"Inf 水头", 2, math.Inf(1)},
	}
	for _, c := range cases {
		if _, err := Discharge(c.b, c.H); err == nil {
			t.Errorf("%s 应当报错", c.name)
		}
	}
	if _, err := DischargeWithCoeff(2, 0.2, 0); err == nil {
		t.Error("零流量系数应当报错")
	}
	if _, err := DischargeWithCoeff(2, 0.2, -0.4); err == nil {
		t.Error("负流量系数应当报错")
	}
}
