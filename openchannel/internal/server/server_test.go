package server

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openchannel/internal/weir"
)

func postJSON(t *testing.T, h http.Handler, path string, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("响应不是合法 JSON: %v\nbody=%s", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func floatField(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("响应缺少字段 %q: %v", key, m)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("字段 %q 不是数字: %v", key, v)
	}
	return f
}

func TestHealth(t *testing.T) {
	srv := New()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz 状态码 = %d", rec.Code)
	}
}

// 端到端：手算矩形渠算例，HTTP 返回的正常水深必须在 1 m 量级，
// 且返回流量与请求流量自洽，流态为缓流（与闭式 y_c 比较一致）。
func TestUniformFlowHandExample(t *testing.T) {
	h := New().Handler()
	body := `{"bottom_width":2,"roughness":0.030,"bed_slope":0.0009,"discharge":1.2599210498948732}`
	status, out := postJSON(t, h, "/v1/uniform-flow", body)
	if status != http.StatusOK {
		t.Fatalf("状态码 %d, 响应 %v", status, out)
	}

	yn := floatField(t, out, "normal_depth")
	if math.Abs(yn-1.0) > 1e-6 {
		t.Errorf("normal_depth=%.8f，期望 ≈1.0", yn)
	}

	qReq := 1.2599210498948732
	qBack := floatField(t, out, "discharge")
	if math.Abs(qBack-qReq)/qReq > 1e-9 {
		t.Errorf("返回流量 %.10f 与请求流量 %.10f 不自洽", qBack, qReq)
	}

	// 连续方程 V=Q/A。
	V := floatField(t, out, "velocity")
	A := floatField(t, out, "area")
	if math.Abs(V*A-qReq)/qReq > 1e-9 {
		t.Errorf("V·A=%.10f ≠ Q=%.10f", V*A, qReq)
	}

	// Fr = V/sqrt(g·A/T)，必须与响应中的 froude_number 同源。
	T := floatField(t, out, "top_width")
	frWant := V / math.Sqrt(9.81*A/T)
	frGot := floatField(t, out, "froude_number")
	if math.Abs(frGot-frWant) > 1e-12 {
		t.Errorf("froude_number=%.10f 与同一套几何重算值 %.10f 不一致", frGot, frWant)
	}

	// 手算算例 y_n≈1.0，闭式 y_c=(Q²/(g b²))^(1/3)≈0.343，故必为缓流。
	if reg, _ := out["regime"].(string); reg != "subcritical" {
		t.Errorf("regime=%v，期望 subcritical", out["regime"])
	}
	if label, _ := out["regime_label"].(string); label != "缓流" {
		t.Errorf("regime_label=%v，期望 缓流", label)
	}

	// 水力半径：矩形 y=1,b=2 → R=0.5。
	if R := floatField(t, out, "hydraulic_radius"); math.Abs(R-0.5) > 1e-9 {
		t.Errorf("hydraulic_radius=%.8f，期望 0.5", R)
	}
}

func TestUniformFlowTrapezoidRoundTrip(t *testing.T) {
	h := New().Handler()
	body := `{"bottom_width":4,"side_slope":1.5,"roughness":0.022,"bed_slope":0.0008,"discharge":25}`
	status, out := postJSON(t, h, "/v1/uniform-flow", body)
	if status != http.StatusOK {
		t.Fatalf("状态码 %d, %v", status, out)
	}
	qBack := floatField(t, out, "discharge")
	if math.Abs(qBack-25)/25 > 1e-9 {
		t.Errorf("梯形流量不自洽: %.10f", qBack)
	}
	yn := floatField(t, out, "normal_depth")
	if yn <= 0 {
		t.Errorf("正常水深应 > 0: %v", yn)
	}
}

func TestUniformFlowValidationErrors(t *testing.T) {
	h := New().Handler()
	cases := []string{
		`{"bottom_width":2,"roughness":0,"bed_slope":0.0009,"discharge":1}`,     // 糙率非正
		`{"bottom_width":2,"roughness":-0.03,"bed_slope":0.0009,"discharge":1}`, // 糙率负
		`{"bottom_width":2,"roughness":0.03,"bed_slope":0,"discharge":1}`,       // 坡度非正
		`{"bottom_width":0,"roughness":0.03,"bed_slope":0.0009,"discharge":1}`,  // 底宽非正
		`{"bottom_width":2,"roughness":0.03,"bed_slope":0.0009,"discharge":-1}`, // 流量负
		`{"bottom_width":2,"roughness":0.03,"bed_slope":0.0009}`,                // 缺流量
	}
	for i, body := range cases {
		status, out := postJSON(t, h, "/v1/uniform-flow", body)
		if status != http.StatusBadRequest {
			t.Errorf("用例 %d 应返回 400，得到 %d: %v", i, status, out)
		}
		if _, ok := out["error"]; !ok {
			t.Errorf("用例 %d 的 400 响应缺少 error 字段", i)
		}
	}
}

func TestUniformFlowRejectsUnknownField(t *testing.T) {
	h := New().Handler()
	body := `{"bottom_width":2,"roughness":0.03,"bed_slope":0.0009,"discharge":1,"hacker":1}`
	if status, _ := postJSON(t, h, "/v1/uniform-flow", body); status != http.StatusBadRequest {
		t.Errorf("未知字段应 400，得到 %d", status)
	}
}

// 堰上水头与正常水深一起给出时：只出提示，且不改任何计算结果。
func TestWeirHeadIsAdvisoryOnly(t *testing.T) {
	h := New().Handler()
	base := `{"bottom_width":2,"roughness":0.030,"bed_slope":0.0009,"discharge":1.2599210498948732}`

	st1, o1 := postJSON(t, h, "/v1/uniform-flow", base)
	st2, o2 := postJSON(t, h, "/v1/uniform-flow", base[:len(base)-1]+`,"weir_head":0.3}`)
	if st1 != http.StatusOK || st2 != http.StatusOK {
		t.Fatalf("两次请求都应成功: %d %d", st1, st2)
	}
	for _, key := range []string{"normal_depth", "velocity", "froude_number", "discharge"} {
		if o1[key] != o2[key] {
			t.Errorf("给出 weir_head 后字段 %s 被改动: %v -> %v", key, o1[key], o2[key])
		}
	}
	adv, ok := o2["advisory"].(string)
	if !ok || !strings.Contains(adv, "不能直接比较") {
		t.Errorf("应返回不能直接比较的提示，得到 %v", o2["advisory"])
	}

	// 提示字段自身非法也要拦。
	bad := base[:len(base)-1] + `,"weir_head":-0.3}`
	if status, _ := postJSON(t, h, "/v1/uniform-flow", bad); status != http.StatusBadRequest {
		t.Errorf("非正堰上水头提示字段应 400，得到 %d", status)
	}
}

func TestWeirFlow(t *testing.T) {
	h := New().Handler()

	// 默认 Cd。
	status, out := postJSON(t, h, "/v1/weir-flow", `{"weir_width":2,"head":0.25}`)
	if status != http.StatusOK {
		t.Fatalf("状态码 %d: %v", status, out)
	}
	qGot := floatField(t, out, "discharge")
	qWant, err := weir.Discharge(2, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(qGot-qWant) > 1e-12 {
		t.Errorf("Q=%.10f，期望 %.10f", qGot, qWant)
	}
	if cd := floatField(t, out, "discharge_coeff"); cd != weir.DefaultDischargeCoeff {
		t.Errorf("默认 Cd=%v，期望 %v", cd, weir.DefaultDischargeCoeff)
	}

	// 显式 Cd。
	status, out = postJSON(t, h, "/v1/weir-flow",
		`{"weir_width":1,"head":0.2,"discharge_coeff":0.45}`)
	if status != http.StatusOK {
		t.Fatalf("状态码 %d: %v", status, out)
	}
	q2 := floatField(t, out, "discharge")
	qWant2, _ := weir.DischargeWithCoeff(1, 0.2, 0.45)
	if math.Abs(q2-qWant2) > 1e-12 {
		t.Errorf("自定义 Cd Q=%.10f，期望 %.10f", q2, qWant2)
	}
}

func TestWeirFlowValidationErrors(t *testing.T) {
	h := New().Handler()
	cases := []string{
		`{"weir_width":0,"head":0.2}`,
		`{"weir_width":2,"head":0}`,
		`{"weir_width":2,"head":-0.1}`,
		`{"weir_width":2}`,
		`{"weir_width":2,"head":0.2,"discharge_coeff":-1}`,
	}
	for i, body := range cases {
		status, out := postJSON(t, h, "/v1/weir-flow", body)
		if status != http.StatusBadRequest {
			t.Errorf("用例 %d 应 400，得到 %d: %v", i, status, out)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := New().Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/weir-flow", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 到 POST 端点应 405，得到 %d", rec.Code)
	}
}

func TestMalformedBody(t *testing.T) {
	h := New().Handler()
	var buf bytes.Buffer
	buf.WriteString("{not json")
	req := httptest.NewRequest(http.MethodPost, "/v1/weir-flow", &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("坏 JSON 应 400，得到 %d", rec.Code)
	}
}
