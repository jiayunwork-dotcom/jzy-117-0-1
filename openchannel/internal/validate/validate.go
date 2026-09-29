// Package validate 集中放置水力计算之前的输入校验。
//
// 规则（对应需求中“在算之前拦下返回错误”）：
//   - 糙率 n 必须为正
//   - 渠底纵坡 S0 必须为正
//   - 底宽 b 必须为正，边坡 m 必须非负
//   - 流量 Q 不允许为负（均匀流反算要求 Q>0，零流量退化为水深 0）
//   - 堰上水头 H 必须为正，堰宽 b 必须为正
//   - 所有数值必须是有限数（拒绝 NaN / ±Inf）
package validate

import (
	"errors"
	"fmt"
	"math"

	"openchannel/internal/geometry"
)

// 预定义错误，便于调用方按语义区分；具体参数信息用 %w 包在外面。
var (
	ErrNotFinite      = errors.New("数值必须是有限数，不能为 NaN 或无穷")
	ErrNotPositive    = errors.New("数值必须大于 0")
	ErrNegative       = errors.New("数值不能为负")
	ErrNotNonNeg      = errors.New("数值必须大于等于 0")
	ErrInvalidSection = errors.New("断面参数非法")
)

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// Finite 校验数值是否有限。
func Finite(name string, v float64) error {
	if !isFinite(v) {
		return fmt.Errorf("%w: %s=%v", ErrNotFinite, name, v)
	}
	return nil
}

// Positive 校验数值为正且有限。
func Positive(name string, v float64) error {
	if !isFinite(v) {
		return fmt.Errorf("%w: %s=%v", ErrNotFinite, name, v)
	}
	if v <= 0 {
		return fmt.Errorf("%w: %s=%v", ErrNotPositive, name, v)
	}
	return nil
}

// NonNegative 校验数值非负且有限。
func NonNegative(name string, v float64) error {
	if !isFinite(v) {
		return fmt.Errorf("%w: %s=%v", ErrNotFinite, name, v)
	}
	if v < 0 {
		return fmt.Errorf("%w: %s=%v", ErrNotNonNeg, name, v)
	}
	return nil
}

// NonNegativeFlow 校验流量：允许 0（退化为水深 0），拒绝负值。
func NonNegativeFlow(Q float64) error {
	if !isFinite(Q) {
		return fmt.Errorf("%w: 流量 Q=%v", ErrNotFinite, Q)
	}
	if Q < 0 {
		return fmt.Errorf("%w: 流量 Q=%v", ErrNegative, Q)
	}
	return nil
}

// Section 校验断面几何：底宽为正、边坡非负。
func Section(s geometry.Section) error {
	if !s.IsValid() {
		return fmt.Errorf("%w: 底宽 b=%v，边坡 m=%v",
			ErrInvalidSection, s.BottomWidth, s.SideSlope)
	}
	return nil
}

// ManningInputs 校验均匀流反算的全部输入。
// Q 必须严格为正（零流量没有正常水深可迭代）。
type ManningInputs struct {
	Section   geometry.Section
	Roughness float64 // 曼宁糙率 n
	BedSlope  float64 // 渠底纵坡 S0
	Discharge float64 // 流量 Q (m³/s)
}

// ManningInputs.Validate 校验均匀流输入。
func (in ManningInputs) Validate() error {
	if err := Section(in.Section); err != nil {
		return err
	}
	if err := Positive("糙率 n", in.Roughness); err != nil {
		return err
	}
	if err := Positive("渠底纵坡 S0", in.BedSlope); err != nil {
		return err
	}
	if err := Positive("流量 Q", in.Discharge); err != nil {
		return err
	}
	return nil
}

// WeirInputs 校验薄壁矩形堰输入。
// DischargeCoeff <= 0 时由堰流层回落到默认系数，因此这里不强制；
// 若调用方显式给出，则必须为正。
func WeirInputs(weirWidth, head float64) error {
	if err := Positive("堰宽 b", weirWidth); err != nil {
		return err
	}
	if err := Positive("堰上水头 H", head); err != nil {
		return err
	}
	return nil
}

// WeirCoeff 校验显式给出的堰流流量系数。
func WeirCoeff(cd float64) error {
	return Positive("流量系数 Cd", cd)
}
