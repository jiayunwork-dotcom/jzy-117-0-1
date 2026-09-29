// Package manning 实现明渠均匀流的曼宁公式及“已知流量反求正常水深”的迭代。
//
// SI 单位制下的曼宁公式：
//
//	Q = (1/n) · A · R^(2/3) · S0^(1/2)
//
// 其中 A 为过流面积、R=A/P 为水力半径（面积除以湿周）、
// n 为糙率、S0 为渠底纵坡。断面几何全部取自 geometry 包。
//
// 反求正常水深采用对分法（bisection）：对任意合法断面（b>0, m>=0），
// 水深增大时过流面积 A 与湿周 P 都单调增大，且 A 增长快于 R^(2/3)
// 的下降效应，故 Q(y) 关于 y 严格单调递增，对分必然收敛。
// 不使用对初值敏感的定点迭代，避免“迭代收敛不住”。
package manning

import (
	"errors"
	"fmt"
	"math"

	"openchannel/internal/geometry"
)

// 对分法相对收敛容差与最大迭代次数。
// 2^-200 远低于 float64 可分辨极限时会被有限精度自然终止。
const (
	defaultRelTol = 1e-12
	maxIterations = 200
)

// ErrNoConvergence 表示在最大迭代次数/搜索范围内未能收敛。
var ErrNoConvergence = errors.New("曼宁迭代未能在限定范围内收敛")

// Discharge 由水深 y 正算均匀流流量 Q。
// 调用前应已通过 validate 层校验 n、S0 与断面。
func Discharge(s geometry.Section, n, S0, y float64) float64 {
	if y <= 0 || n <= 0 || S0 <= 0 {
		return 0
	}
	A := s.Area(y)
	R := s.HydraulicRadius(y)
	return A * math.Pow(R, 2.0/3.0) * math.Sqrt(S0) / n
}

// Velocity 返回均匀流断面平均流速 V = Q/A。
func Velocity(s geometry.Section, n, S0, y float64) float64 {
	if y <= 0 {
		return 0
	}
	A := s.Area(y)
	if A <= 0 {
		return 0
	}
	return Discharge(s, n, S0, y) / A
}

// NormalDepth 已知流量 Q 反求正常水深 y_n。
//
// 要求断面合法、n>0、S0>0、Q>0（输入合法性由 validate 包负责，
// 这里做防御性兜底并返回错误）。
func NormalDepth(s geometry.Section, n, S0, Q float64) (float64, error) {
	if !s.IsValid() {
		return 0, fmt.Errorf("正常水深求解: %w: b=%v m=%v",
			errors.New("断面参数非法"), s.BottomWidth, s.SideSlope)
	}
	if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("正常水深求解: 糙率 n=%v 必须为正", n)
	}
	if S0 <= 0 || math.IsNaN(S0) || math.IsInf(S0, 0) {
		return 0, fmt.Errorf("正常水深求解: 渠底纵坡 S0=%v 必须为正", S0)
	}
	if math.IsNaN(Q) || math.IsInf(Q, 0) || Q < 0 {
		return 0, fmt.Errorf("正常水深求解: 流量 Q=%v 不能为负或非有限", Q)
	}
	if Q == 0 {
		return 0, nil
	}

	// f(y) = Q(y) - Q_target；f 关于 y 严格单调递增，f(0) = -Q < 0。
	f := func(y float64) float64 {
		return Discharge(s, n, S0, y) - Q
	}

	lo := 0.0
	hi, err := bracketUpperBound(f)
	if err != nil {
		return 0, err
	}

	return bisect(f, lo, hi, defaultRelTol)
}

// bracketUpperBound 从 y=1m 起按倍扩大区间，直到 f(hi) >= 0。
// 对极端 Q（如 1e6 m³/s）也能稳定夹住根，不依赖经验初值。
func bracketUpperBound(f func(float64) float64) (float64, error) {
	hi := 1.0
	for i := 0; i < maxIterations; i++ {
		fh := f(hi)
		if math.IsNaN(fh) {
			return 0, fmt.Errorf("%w: 搜索过程中出现非数值", ErrNoConvergence)
		}
		if fh >= 0 {
			return hi, nil
		}
		hi *= 2
		if math.IsInf(hi, 0) {
			return 0, fmt.Errorf("%w: 水深搜索上界溢出", ErrNoConvergence)
		}
	}
	return 0, fmt.Errorf("%w: 无法夹住含根区间", ErrNoConvergence)
}

// bisect 在 [lo, hi] 上对单调函数 f 做对分求根，相对容差 relTol。
// 不变量：f(lo) < 0 <= f(hi)。
func bisect(f func(float64) float64, lo, hi, relTol float64) (float64, error) {
	for i := 0; i < maxIterations; i++ {
		mid := 0.5 * (lo + hi)
		fm := f(mid)
		if fm >= 0 {
			hi = mid
		} else {
			lo = mid
		}
		// 区间已被 float64 精度夹死，或相对宽度足够小。
		if hi-lo <= relTol*math.Max(1.0, hi) {
			return 0.5 * (lo + hi), nil
		}
	}
	return 0, fmt.Errorf("%w: 达到最大迭代次数 %d", ErrNoConvergence, maxIterations)
}
