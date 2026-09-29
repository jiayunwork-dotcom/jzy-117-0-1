// Package regime 负责明渠流态判定：弗劳德数 Fr 与临界水深 y_c。
//
// 弗劳德数定义为断面平均流速与小波（重力波）波速之比：
//
//	Fr = V / sqrt(g · A / T)
//
// 其中 T 是自由水面宽、A 是过流面积，二者都来自 geometry 包，
// 与曼宁迭代使用完全相同的一套断面几何——绝不允许判流态时另抄一份水面宽。
//
// Fr < 1 为缓流（subcritical），Fr > 1 为急流（supercritical），
// Fr ≈ 1 为临界流。
package regime

import (
	"errors"
	"fmt"
	"math"

	"openchannel/internal/geometry"
	"openchannel/internal/physics"
)

// State 表示流态。
type State string

const (
	Subcritical   State = "subcritical"   // 缓流 Fr < 1
	Critical      State = "critical"      // 临界流 Fr ≈ 1
	Supercritical State = "supercritical" // 急流 Fr > 1
)

// Label 返回流态的中文说明。
func (s State) Label() string {
	switch s {
	case Subcritical:
		return "缓流"
	case Critical:
		return "临界流"
	case Supercritical:
		return "急流"
	default:
		return "未知流态"
	}
}

// criticalTol 是判定临界流的 Fr 容差。
const criticalTol = 1e-6

// ErrNoConvergence 表示临界水深迭代未收敛。
var ErrNoConvergence = errors.New("临界水深迭代未能收敛")

// Froude 由断面平均流速 V 与水深 y 计算弗劳德数。
// 水面宽 T 取自 geometry.Section.TopWidth，保证几何自洽。
// y <= 0 或波速为零时返回 0（由调用方/校验层保证输入为正水深）。
func Froude(s geometry.Section, y, V float64) float64 {
	if y <= 0 || V < 0 {
		return 0
	}
	A := s.Area(y)
	T := s.TopWidth(y)
	if A <= 0 || T <= 0 {
		return 0
	}
	celerity := math.Sqrt(physics.G * A / T) // 小波相对波速
	if celerity == 0 {
		return 0
	}
	return V / celerity
}

// Classify 按弗劳德数判定流态。
func Classify(fr float64) State {
	switch {
	case fr < 1-criticalTol:
		return Subcritical
	case fr > 1+criticalTol:
		return Supercritical
	default:
		return Critical
	}
}

// CriticalDepth 用比能极小条件 Q²·T / (g·A³) = 1 反求临界水深 y_c。
//
// 对任意梯形断面（含矩形 m=0），h(y)=Q²·T/(g·A³) 关于 y 严格单调递减
// （A 随 y 增长快于 T），故同样可以用“倍区间 + 对分”稳健求解。
// 矩形断面另有闭式解 y_c = (Q²/(g·b²))^(1/3)，见 RectangularCriticalDepth，
// 供测试抽查本函数与流态判定是否正确。
func CriticalDepth(s geometry.Section, Q float64) (float64, error) {
	if !s.IsValid() {
		return 0, fmt.Errorf("临界水深求解: %w: b=%v m=%v",
			errors.New("断面参数非法"), s.BottomWidth, s.SideSlope)
	}
	if math.IsNaN(Q) || math.IsInf(Q, 0) || Q < 0 {
		return 0, fmt.Errorf("临界水深求解: 流量 Q=%v 不能为负或非有限", Q)
	}
	if Q == 0 {
		return 0, nil
	}

	// h(y)-1：h 在 y→0+ 时 → +∞（矩形）或更大（梯形），随 y 单调递减到 0。
	h := func(y float64) float64 {
		A := s.Area(y)
		T := s.TopWidth(y)
		return Q*Q*T/(physics.G*A*A*A) - 1
	}

	lo := math.SmallestNonzeroFloat64
	hi, err := bracketCritical(s, Q)
	if err != nil {
		return 0, err
	}

	const (
		relTol        = 1e-12
		maxIterations = 200
	)
	for i := 0; i < maxIterations; i++ {
		mid := 0.5 * (lo + hi)
		if h(mid) > 0 {
			lo = mid // 临界水深还在右边
		} else {
			hi = mid
		}
		if hi-lo <= relTol*math.Max(1.0, hi) {
			return 0.5 * (lo + hi), nil
		}
	}
	return 0, fmt.Errorf("%w: 达到最大迭代次数", ErrNoConvergence)
}

// bracketCritical 找到使 h(hi) <= 0 的上界。
func bracketCritical(s geometry.Section, Q float64) (float64, error) {
	hi := 1.0
	for i := 0; i < 200; i++ {
		A := s.Area(hi)
		T := s.TopWidth(hi)
		if Q*Q*T <= physics.G*A*A*A {
			return hi, nil
		}
		hi *= 2
		if math.IsInf(hi, 0) {
			return 0, fmt.Errorf("%w: 临界水深搜索上界溢出", ErrNoConvergence)
		}
	}
	return 0, fmt.Errorf("%w: 无法夹住临界水深区间", ErrNoConvergence)
}

// RectangularCriticalDepth 返回矩形断面临界水深闭式解：
//
//	y_c = (Q² / (g · b²))^(1/3)
//
// 这是教科书闭式解，测试中用它抽查迭代求解与流态判定的正确性。
func RectangularCriticalDepth(bottomWidth, Q float64) float64 {
	if bottomWidth <= 0 || Q <= 0 {
		return 0
	}
	return math.Cbrt(Q * Q / (physics.G * bottomWidth * bottomWidth))
}
