// Package server 用标准库 net/http 装配对外 HTTP 接口。
//
// 路由：
//
//	GET  /healthz  健康检查
//	POST /v1/uniform-flow  明渠均匀流：已知流量反求正常水深
//	POST /v1/weir-flow     薄壁矩形堰：由堰上水头算流量
//
// 这里只做协议层的解码、校验、错误映射；断面几何、曼宁迭代、流态判定、
// 堰流公式分别来自各自的独立包。
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"openchannel/internal/geometry"
	"openchannel/internal/manning"
	"openchannel/internal/regime"
	"openchannel/internal/validate"
	"openchannel/internal/weir"
)

// maxBodyBytes 限制请求体大小，防止异常大请求。
const maxBodyBytes = 1 << 20 // 1 MiB

// Server 持有路由与共享依赖（当前无外部依赖，预留以便扩展）。
type Server struct {
	mux *http.ServeMux
}

// New 构造装配好路由的 Server。
func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/uniform-flow", s.handleUniformFlow)
	s.mux.HandleFunc("POST /v1/weir-flow", s.handleWeirFlow)
	return s
}

// Handler 返回可供 http.Server 使用的根 handler。
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ---- 均匀流 ----

// UniformFlowRequest 是均匀流反算请求。
// SideSlope 省略或为 0 即矩形断面。
// WeirHead 为可选字段：若给出，服务只提示它与正常水深不是同一物理量，
// 绝不参与或修改任何水力公式。
type UniformFlowRequest struct {
	BottomWidth float64  `json:"bottom_width"`         // 渠底宽 b (m)
	SideSlope   float64  `json:"side_slope,omitempty"` // 边坡 m，矩形=0
	Roughness   float64  `json:"roughness"`            // 曼宁糙率 n
	BedSlope    float64  `json:"bed_slope"`            // 渠底纵坡 S0
	Discharge   float64  `json:"discharge"`            // 流量 Q (m³/s)
	WeirHead    *float64 `json:"weir_head,omitempty"`  // 可选：仅供提示对比的堰上水头 H (m)
}

// UniformFlowResponse 返回正常水深及由同一套几何派生的水力要素。
type UniformFlowResponse struct {
	NormalDepth     float64 `json:"normal_depth"`       // 正常水深 y_n (m)
	Velocity        float64 `json:"velocity"`           // 断面平均流速 V (m/s)
	FroudeNumber    float64 `json:"froude_number"`      // 弗劳德数 Fr
	Regime          string  `json:"regime"`             // 流态英文标识
	RegimeLabel     string  `json:"regime_label"`       // 流态中文说明
	HydraulicRadius float64 `json:"hydraulic_radius"`   // 水力半径 R (m)
	Area            float64 `json:"area"`               // 过流面积 A (m²)
	TopWidth        float64 `json:"top_width"`          // 水面宽 T (m)
	Discharge       float64 `json:"discharge"`          // 回代确认的流量 Q (m³/s)
	Advisory        string  `json:"advisory,omitempty"` // 提示信息（如堰水头对比）
}

func (s *Server) handleUniformFlow(w http.ResponseWriter, r *http.Request) {
	var req UniformFlowRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return // 错误响应已在 decodeJSON 内发出
	}

	sec := geometry.Section{BottomWidth: req.BottomWidth, SideSlope: req.SideSlope}
	in := validate.ManningInputs{
		Section:   sec,
		Roughness: req.Roughness,
		BedSlope:  req.BedSlope,
		Discharge: req.Discharge,
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	yn, err := manning.NormalDepth(sec, req.Roughness, req.BedSlope, req.Discharge)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	// 流速、Fr 一律以解出的 y_n 和 geometry 的同一套几何重新计算，
	// 流速用 Q/A 而不是另抄曼宁公式，保证流量、水深、流态自洽。
	A := sec.Area(yn)
	V := req.Discharge / A
	fr := regime.Froude(sec, yn, V)
	state := regime.Classify(fr)

	resp := UniformFlowResponse{
		NormalDepth:     yn,
		Velocity:        V,
		FroudeNumber:    fr,
		Regime:          string(state),
		RegimeLabel:     state.Label(),
		HydraulicRadius: sec.HydraulicRadius(yn),
		Area:            A,
		TopWidth:        sec.TopWidth(yn),
		Discharge:       manning.Discharge(sec, req.Roughness, req.BedSlope, yn),
	}

	if req.WeirHead != nil {
		if err := validate.Positive("堰上水头 H（提示字段）", *req.WeirHead); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		resp.Advisory = compareWeirHeadAdvisory(yn, *req.WeirHead)
	}

	writeJSON(w, http.StatusOK, resp)
}

// compareWeirHeadAdvisory 只生成文字提示，不改动任何计算输入或公式。
func compareWeirHeadAdvisory(normalDepth, weirHead float64) string {
	note := "提示：堰上水头 H 是相对于堰顶的测压管水头，" +
		"正常水深 y_n 是渠底起算的均匀流水深，二者基准不同，" +
		"不能直接比较大小，也不会互相参与计算。"
	switch {
	case normalDepth > weirHead:
		return "数值上 y_n > H。" + note
	case normalDepth < weirHead:
		return "数值上 y_n < H。" + note
	default:
		return "数值上 y_n = H。" + note
	}
}

// ---- 堰流 ----

// WeirFlowRequest 是薄壁矩形堰流量请求。
// DischargeCoeff 省略（0）时使用默认值 0.42。
type WeirFlowRequest struct {
	WeirWidth      float64 `json:"weir_width"`                // 堰宽 b (m)
	Head           float64 `json:"head"`                      // 堰上水头 H (m)
	DischargeCoeff float64 `json:"discharge_coeff,omitempty"` // 流量系数 Cd，默认 0.42
}

// WeirFlowResponse 返回堰流结果。
type WeirFlowResponse struct {
	Discharge      float64 `json:"discharge"`       // 过堰流量 Q (m³/s)
	WeirWidth      float64 `json:"weir_width"`      // 堰宽 b (m)
	Head           float64 `json:"head"`            // 堰上水头 H (m)
	DischargeCoeff float64 `json:"discharge_coeff"` // 使用的流量系数
}

func (s *Server) handleWeirFlow(w http.ResponseWriter, r *http.Request) {
	var req WeirFlowRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := validate.WeirInputs(req.WeirWidth, req.Head); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	cd := req.DischargeCoeff
	if cd == 0 {
		cd = weir.DefaultDischargeCoeff
	} else if err := validate.WeirCoeff(cd); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	q, err := weir.DischargeWithCoeff(req.WeirWidth, req.Head, cd)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, WeirFlowResponse{
		Discharge:      q,
		WeirWidth:      req.WeirWidth,
		Head:           req.Head,
		DischargeCoeff: cd,
	})
}

// ---- 健康检查与公共辅助 ----

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type errorResponse struct {
	Error string `json:"error"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return err
	}
	// 请求体里不允许多余的第二个 JSON 对象（尾随空白可以）。
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("请求体中只允许一个 JSON 对象")
		}
		writeError(w, http.StatusBadRequest, err)
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	log.Printf("http %d: %v", status, err)
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // 提示文本中的 >、< 保持原样，无需 HTML 转义
	if err := enc.Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}
