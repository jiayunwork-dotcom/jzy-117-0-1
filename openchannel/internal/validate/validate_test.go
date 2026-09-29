package validate

import (
	"errors"
	"math"
	"testing"

	"openchannel/internal/geometry"
)

func TestPositiveAndFinite(t *testing.T) {
	if err := Positive("x", 0); !errors.Is(err, ErrNotPositive) {
		t.Errorf("零应判非正: %v", err)
	}
	if err := Positive("x", -1); !errors.Is(err, ErrNotPositive) {
		t.Errorf("负值应判非正: %v", err)
	}
	if err := Positive("x", math.NaN()); !errors.Is(err, ErrNotFinite) {
		t.Errorf("NaN 应判非有限: %v", err)
	}
	if err := Positive("x", math.Inf(-1)); !errors.Is(err, ErrNotFinite) {
		t.Errorf("-Inf 应判非有限: %v", err)
	}
	if err := Positive("x", 0.001); err != nil {
		t.Errorf("正值不应报错: %v", err)
	}
}

func TestNonNegativeFlow(t *testing.T) {
	if err := NonNegativeFlow(-0.01); !errors.Is(err, ErrNegative) {
		t.Errorf("负流量应被拦: %v", err)
	}
	if err := NonNegativeFlow(math.Inf(1)); !errors.Is(err, ErrNotFinite) {
		t.Errorf("Inf 流量应被拦: %v", err)
	}
	if err := NonNegativeFlow(0); err != nil {
		t.Errorf("零流量应允许（退化情形）: %v", err)
	}
}

// 需求中点名的五类非法输入，必须在计算前全部拦下。
func TestManningInputsAllGuards(t *testing.T) {
	good := ManningInputs{
		Section:   geometry.NewRectangle(2),
		Roughness: 0.03,
		BedSlope:  0.0009,
		Discharge: 1.5,
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}

	mut := func(fn func(*ManningInputs)) ManningInputs {
		c := good
		fn(&c)
		return c
	}

	bad := []ManningInputs{
		mut(func(c *ManningInputs) { c.Roughness = 0 }),                              // 糙率非正
		mut(func(c *ManningInputs) { c.Roughness = -0.03 }),                          // 糙率负
		mut(func(c *ManningInputs) { c.BedSlope = 0 }),                               // 坡度非正
		mut(func(c *ManningInputs) { c.BedSlope = -0.001 }),                          // 坡度负
		mut(func(c *ManningInputs) { c.Section = geometry.Section{BottomWidth: 0} }), // 底宽非正
		mut(func(c *ManningInputs) { c.Section = geometry.Section{BottomWidth: -2} }),
		mut(func(c *ManningInputs) { c.Section = geometry.Section{BottomWidth: 2, SideSlope: -1} }),
		mut(func(c *ManningInputs) { c.Discharge = -1 }), // 流量为负
		mut(func(c *ManningInputs) { c.Discharge = math.NaN() }),
	}
	for i, in := range bad {
		if err := in.Validate(); err == nil {
			t.Errorf("第 %d 组非法输入未被拦下: %+v", i, in)
		}
	}
}

func TestWeirGuards(t *testing.T) {
	if err := WeirInputs(2, 0.2); err != nil {
		t.Errorf("合法堰输入不应报错: %v", err)
	}
	if err := WeirInputs(0, 0.2); err == nil {
		t.Error("堰宽为零应被拦")
	}
	if err := WeirInputs(2, 0); err == nil {
		t.Error("堰上水头为零应被拦")
	}
	if err := WeirInputs(2, -0.3); err == nil {
		t.Error("堰上水头为负应被拦")
	}
	if err := WeirInputs(-1, 0.2); err == nil {
		t.Error("负堰宽应被拦")
	}
}
