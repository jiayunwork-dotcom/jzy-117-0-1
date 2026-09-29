// Package weir 实现无侧收缩薄壁矩形堰（锐缘堰）的自由出流公式。
//
// 采用流量系数形式：
//
//	Q = Cd · b · sqrt(2g) · H^(3/2)
//
// 其中 Cd 为流量系数（典型值约 0.40~0.46，本服务默认 0.42），
// b 为堰宽，H 为堰顶以上的测压管水头。
// 同一 Cd 下，水头抬高时流量严格按 H 的 3/2 次幂增长。
//
// 注意：H 是“堰上水头”，与渠道正常水深 y_n 是两个不同物理量，
// 不能直接比大小；服务层若收到同时给出的二者，只做提示，不修改任何公式。
package weir

import (
	"errors"
	"fmt"
	"math"

	"openchannel/internal/physics"
)

// DefaultDischargeCoeff 是薄壁矩形堰默认流量系数。
const DefaultDischargeCoeff = 0.42

// Discharge 计算薄壁矩形堰过堰流量。
//
//	width: 堰宽 b (m)，必须 > 0
//	head:  堰上水头 H (m)，必须 > 0
//
// 输入应先经 validate.WeirInputs 校验；此处做防御性兜底。
func Discharge(width, head float64) (float64, error) {
	return DischargeWithCoeff(width, head, DefaultDischargeCoeff)
}

// DischargeWithCoeff 与 Discharge 相同，但允许显式给定流量系数 Cd（必须 > 0）。
func DischargeWithCoeff(width, head, cd float64) (float64, error) {
	if math.IsNaN(width) || math.IsInf(width, 0) || width <= 0 {
		return 0, fmt.Errorf("堰流计算: %w: 堰宽 b=%v", errors.New("堰宽必须为正"), width)
	}
	if math.IsNaN(head) || math.IsInf(head, 0) || head <= 0 {
		return 0, fmt.Errorf("堰流计算: %w: 堰上水头 H=%v", errors.New("堰上水头必须为正"), head)
	}
	if math.IsNaN(cd) || math.IsInf(cd, 0) || cd <= 0 {
		return 0, fmt.Errorf("堰流计算: %w: Cd=%v", errors.New("流量系数必须为正"), cd)
	}
	return cd * width * math.Sqrt(2*physics.G) * math.Pow(head, 1.5), nil
}
