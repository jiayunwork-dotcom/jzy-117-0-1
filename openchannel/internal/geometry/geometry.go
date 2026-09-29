// Package geometry 是明渠断面几何的唯一来源。
//
// 矩形与梯形断面共用同一套公式：矩形只是底宽 b、边坡 m=0 的梯形特例。
// 曼宁迭代所需的过流面积、水力半径，与流态判定（弗劳德数）所需的
// 水面宽，都必须取自本包的同一组几何函数，杜绝“两套几何互相抄歪”。
//
// 全部物理量采用 SI 单位：长度 m，面积 m²。
package geometry

import "math"

// Section 描述一个梯形明渠断面。
//
//	BottomWidth: 渠底宽 b，必须 > 0
//	SideSlope:   边坡系数 m（水平 : 竖直 = m : 1），必须 >= 0；
//	             矩形断面取 0
type Section struct {
	BottomWidth float64 `json:"bottom_width"`
	SideSlope   float64 `json:"side_slope"`
}

// NewRectangle 是矩形断面的便捷构造，等价于边坡为 0 的梯形断面。
func NewRectangle(bottomWidth float64) Section {
	return Section{BottomWidth: bottomWidth, SideSlope: 0}
}

// NewTrapezoid 构造梯形断面。
func NewTrapezoid(bottomWidth, sideSlope float64) Section {
	return Section{BottomWidth: bottomWidth, SideSlope: sideSlope}
}

// Area 返回水深 y 时的过流面积：A = (b + m·y)·y。
// y < 0 视为无水（0 面积）；正常使用前应由校验层保证 y >= 0。
func (s Section) Area(y float64) float64 {
	if y < 0 {
		return 0
	}
	return (s.BottomWidth + s.SideSlope*y) * y
}

// WettedPerimeter 返回水深 y 时的湿周：P = b + 2·y·sqrt(1+m²)。
// 湿周只计底板与两侧边坡与水接触的长度，不含自由水面。
func (s Section) WettedPerimeter(y float64) float64 {
	if y < 0 {
		return s.BottomWidth
	}
	return s.BottomWidth + 2*y*math.Sqrt(1+s.SideSlope*s.SideSlope)
}

// TopWidth 返回水深 y 时的自由水面宽（水面宽）：T = b + 2·m·y。
// 该函数是流态判定/弗劳德数的唯一水面宽来源，
// 保证判流态与曼宁迭代出自同一套断面几何。
func (s Section) TopWidth(y float64) float64 {
	if y < 0 {
		return s.BottomWidth
	}
	return s.BottomWidth + 2*s.SideSlope*y
}

// HydraulicRadius 返回水力半径 R = A/P，即过流面积除以湿周。
func (s Section) HydraulicRadius(y float64) float64 {
	p := s.WettedPerimeter(y)
	if p <= 0 {
		return 0
	}
	return s.Area(y) / p
}

// IsValid 做断面形状自身的合法性检查（底宽为正、边坡非负、数值有限）。
func (s Section) IsValid() bool {
	return isFinitePositive(s.BottomWidth) && isFiniteNonNegative(s.SideSlope)
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func isFinitePositive(v float64) bool {
	return isFinite(v) && v > 0
}

func isFiniteNonNegative(v float64) bool {
	return isFinite(v) && v >= 0
}
